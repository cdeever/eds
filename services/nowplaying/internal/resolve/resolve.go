// Package resolve finds what a source did not say: the album a track is on,
// and its cover.
//
// Subscribers are promised a title, an album and a cover whenever something
// is playing, and drivers cannot all supply those (ADR-0008). A cover the
// player supplied never comes here - it is the right cover, and a catalogue
// lookup is only a good guess. This is for the tracks that arrive with none.
//
// It knows nothing about lights, stands or MQTT.
package resolve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// MaxImage bounds a cover fetched from a catalogue. Covers are requested at
// 500-1000 px and are far smaller; this is the guard against a URL that turns
// out to be something else.
const MaxImage = 4 << 20

// ErrNotFound means every catalogue was asked and none had it. It is an
// answer, not a failure, and it is cached like one.
var ErrNotFound = errors.New("resolve: no catalogue has this")

// Query is what is known about a track when a catalogue is asked.
type Query struct {
	Artist string
	Album  string // may be empty: then the album is part of the question
	Title  string
}

// Hit is a catalogue's answer. Title is the track's, and is only filled in
// when the question was about a track.
type Hit struct {
	Artist   string
	Album    string
	Title    string
	ImageURL string
}

// Catalogue is one place to look. Lookup returns ErrNotFound when the
// catalogue answered and had nothing; any other error means it could not be
// asked, and the next catalogue is tried either way.
type Catalogue interface {
	Name() string
	Lookup(ctx context.Context, q Query) (*Hit, error)
}

// Cover is an image and where it came from.
type Cover struct {
	Image []byte
	MIME  string
	From  string
}

// Result is what was found. Album is the catalogue's name for the album and
// is only of interest when the question had none.
type Result struct {
	Album string
	Cover *Cover
}

type entry struct {
	result  Result
	missing bool
	at      time.Time
}

// Resolver asks catalogues in order and remembers the answers.
type Resolver struct {
	// Catalogues are tried in order; the first usable answer wins.
	Catalogues []Catalogue
	HTTP       *http.Client
	UserAgent  string
	Logger     *slog.Logger

	// MissTTL is how long "nobody has it" is believed before asking again.
	MissTTL time.Duration
	// Keep bounds the cache. Covers are held as bytes, and an evening's
	// listening is a few dozen albums.
	Keep int

	now func() time.Time

	mu    sync.Mutex
	cache map[string]*entry
	order []string
}

// New builds a Resolver with the defaults an unattended service wants.
func New(catalogues []Catalogue, userAgent string, logger *slog.Logger) *Resolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Resolver{
		Catalogues: catalogues,
		HTTP:       &http.Client{Timeout: 8 * time.Second},
		UserAgent:  userAgent,
		Logger:     logger,
		MissTTL:    6 * time.Hour,
		Keep:       48,
		now:        time.Now,
		cache:      make(map[string]*entry),
	}
}

// key is what an answer is remembered under. With an album known it is the
// album, so every track on it shares one lookup. Without one it has to be the
// track, because the album is what is being asked.
func key(t contract.Track) string {
	if strings.TrimSpace(t.Album) != "" {
		return "album\x00" + t.AlbumKey()
	}
	return "track\x00" + t.Key()
}

// Resolve finds the album and cover for a track. It returns ErrNotFound when
// there is nothing to ask with or nobody had it.
//
// One lookup is made per album, not per track and not per call: the answer,
// including "not found", is remembered. That is what keeps a service that
// follows every track change far inside catalogues' limits - the tightest is
// about twenty calls a minute.
func (r *Resolver) Resolve(ctx context.Context, t contract.Track) (Result, error) {
	q := Query{Artist: firstNonEmpty(t.AlbumArtist, t.Artist), Album: t.Album, Title: t.Title}
	if q.Album == "" {
		// The track artist is the better search term when the album is the
		// unknown: catalogues index tracks by who performs them.
		q.Artist = firstNonEmpty(t.Artist, t.AlbumArtist)
	}
	if strings.TrimSpace(q.Artist) == "" || (strings.TrimSpace(q.Album) == "" && strings.TrimSpace(q.Title) == "") {
		return Result{}, ErrNotFound
	}

	k := key(t)
	if cached, ok := r.cached(k); ok {
		if cached.missing {
			return Result{}, ErrNotFound
		}
		return cached.result, nil
	}

	for _, catalogue := range r.Catalogues {
		hit, err := catalogue.Lookup(ctx, q)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				r.Logger.Warn("catalogue could not be asked", "catalogue", catalogue.Name(), "error", err)
			}
			continue
		}
		if !matches(q, hit) {
			// A confident wrong cover is worse than none: it lights the room
			// for a record that is not playing.
			r.Logger.Info("catalogue answered with something else",
				"catalogue", catalogue.Name(), "asked_artist", q.Artist, "asked_album", q.Album,
				"got_artist", hit.Artist, "got_album", hit.Album)
			continue
		}

		image, mime, err := r.fetch(ctx, hit.ImageURL)
		if err != nil {
			r.Logger.Warn("cover could not be fetched", "catalogue", catalogue.Name(), "error", err)
			continue
		}

		result := Result{Album: hit.Album, Cover: &Cover{Image: image, MIME: mime, From: catalogue.Name()}}
		r.remember(k, &entry{result: result, at: r.now()})
		return result, nil
	}

	if ctx.Err() != nil {
		// Out of time is not "nobody has it". Do not remember it as such.
		return Result{}, ctx.Err()
	}
	r.remember(k, &entry{missing: true, at: r.now()})
	return Result{}, ErrNotFound
}

func (r *Resolver) cached(k string) (*entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[k]
	if !ok {
		return nil, false
	}
	if e.missing && r.now().Sub(e.at) > r.MissTTL {
		return nil, false
	}
	return e, true
}

func (r *Resolver) remember(k string, e *entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.cache[k]; !exists {
		r.order = append(r.order, k)
	}
	r.cache[k] = e
	for len(r.order) > r.Keep {
		delete(r.cache, r.order[0])
		r.order = r.order[1:]
	}
}

func (r *Resolver) fetch(ctx context.Context, url string) ([]byte, string, error) {
	if url == "" {
		return nil, "", errors.New("no image url")
	}
	body, err := get(ctx, r.HTTP, r.UserAgent, url, MaxImage)
	if err != nil {
		return nil, "", err
	}
	mime := http.DetectContentType(body)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("%s is %s, not an image", url, mime)
	}
	return body, mime, nil
}

// get is one bounded GET. A catalogue that answers 404 has answered.
func get(ctx context.Context, client *http.Client, userAgent, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Every catalogue asks to know who is calling, and one refuses callers
	// that do not say.
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, image/*;q=0.9, */*;q=0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s sent more than %d bytes", req.URL.Host, limit)
	}
	return body, nil
}

// matches reports whether a hit is the thing that was asked for. Catalogues
// return their best guess for any search, and the best guess for an obscure
// record is often a famous one.
func matches(q Query, hit *Hit) bool {
	if hit == nil || !similar(q.Artist, hit.Artist) {
		return false
	}
	if strings.TrimSpace(q.Album) == "" {
		// The album was the question, so it cannot be checked. The track can:
		// a free-text search for an artist and a title returns that artist's
		// other tracks too, and each is on a different album.
		return strings.TrimSpace(hit.Album) != "" && similar(q.Title, hit.Title)
	}
	return similar(q.Album, hit.Album)
}

// similar compares two names the way a person would: ignoring case,
// punctuation, and the edition notes catalogues append - "(Remastered 2011)",
// "[Deluxe Edition]". One containing the other is enough, because "The Band"
// and "Band" are the same band and a catalogue's "Kind of Blue (Legacy
// Edition)" is the album asked for as "Kind of Blue".
func similar(a, b string) bool {
	x, y := simplify(a), simplify(b)
	if x == "" || y == "" {
		return false
	}
	return x == y || strings.Contains(x, y) || strings.Contains(y, x)
}

func simplify(s string) string {
	plain := lettersOnly(s, true)
	if plain == "" {
		// A name that was all brackets: compare what there is.
		plain = lettersOnly(s, false)
	}
	// A leading article is dropped, unless it is the whole name.
	if trimmed := strings.TrimPrefix(plain, "the"); trimmed != "" {
		return trimmed
	}
	return plain
}

// lettersOnly keeps letters and digits, lowercased, optionally skipping
// anything inside brackets.
func lettersOnly(s string, skipBracketed bool) string {
	var out strings.Builder
	depth := 0
	for _, r := range strings.ToLower(s) {
		switch r {
		case '(', '[':
			depth++
			continue
		case ')', ']':
			if depth > 0 {
				depth--
			}
			continue
		}
		if (depth == 0 || !skipBracketed) && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
