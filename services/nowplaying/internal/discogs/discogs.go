// Package discogs knows the records their owner has, from the owner's
// collection at Discogs.
//
// A turntable's driver names a track. Which record that track is on, of every
// record it was ever pressed on, nobody listening can tell - but the record
// on the platter is one its owner has, and the collection says which
// (ADR-0022). This package answers one question: given an artist and a
// title, which record in the collection is it, and what is on its sleeve?
//
// It knows nothing about sources, lights or MQTT.
package discogs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// API is where Discogs' API is.
const API = "https://api.discogs.com"

// Name is what a cover from here is said to be from.
const Name = "discogs"

// maxPages bounds the listing: five thousand records.
const maxPages = 50

// minPartial is the shortest title that may match as part of a longer one.
// "Intro" is part of "Introduction", and is not the same track.
const minPartial = 6

// record is one release in the collection.
type record struct {
	id          int
	title       string
	artists     []string // simplified, for comparing
	year        int
	compilation bool
	vinyl       bool
	cover       string
}

// Collection is one owner's records.
type Collection struct {
	Base      string // API, unless a test says otherwise
	User      string
	Token     string // a personal access token; empty for a public collection
	HTTP      *http.Client
	UserAgent string
	Logger    *slog.Logger

	// Refresh is how long the list of records is believed. Discogs asks
	// that its data is not shown more than six hours stale.
	Refresh time.Duration

	Now func() time.Time

	mu      sync.Mutex
	records []record
	listed  time.Time
	// tracks are the titles on a record, simplified. Asked for the first
	// time a record's artist is heard, and kept: what is pressed on a record
	// does not change.
	tracks map[int][]string
	// loading is the fetch of one artist's tracklists, so that two tracks
	// by one artist do not start two.
	loading map[string]chan struct{}
	// last is the record the previous answer was. A track that is on two
	// records the owner has is taken to be on the one already playing.
	last int
	// sleeve is the last cover fetched, and the record it is of.
	sleeve   *resolve.Cover
	sleeveOf int
}

// Find names the record in the collection that a track is on. The album it
// returns is the record's title; the cover is the record's own picture, and
// is nil when Discogs has none or it could not be fetched.
//
// album is what the source called the album, if it called it anything. It
// is a hint and no more: a record of that name is preferred when the track
// is on several, and a track is found without it.
//
// It returns resolve.ErrNotFound when the collection has no such track. Any
// other error means it could not be asked, or did not finish in time.
func (c *Collection) Find(ctx context.Context, artist, title, album string) (resolve.Result, error) {
	wantArtist, wantTitle := resolve.Simplify(artist), resolve.Simplify(title)
	if wantArtist == "" || wantTitle == "" {
		return resolve.Result{}, resolve.ErrNotFound
	}
	if err := c.list(ctx); err != nil {
		return resolve.Result{}, err
	}

	by := c.by(wantArtist)
	if len(by) == 0 {
		return resolve.Result{}, resolve.ErrNotFound
	}
	if err := c.load(ctx, wantArtist, by); err != nil {
		return resolve.Result{}, err
	}

	found, ok := c.choose(by, wantTitle, resolve.Simplify(album))
	if !ok {
		return resolve.Result{}, resolve.ErrNotFound
	}
	return resolve.Result{Album: found.title, Cover: c.cover(ctx, found)}, nil
}

// Warm lists the collection before anyone asks about it, so that the first
// record played is not the one that waits for the list.
func (c *Collection) Warm(ctx context.Context) error { return c.list(ctx) }

// list fetches the collection if it has not been, or not lately. A list that
// cannot be refreshed is kept: the records have not left the shelf.
func (c *Collection) list(ctx context.Context) error {
	c.mu.Lock()
	fresh := !c.listed.IsZero() && c.now().Sub(c.listed) < c.refresh()
	have := !c.listed.IsZero()
	c.mu.Unlock()
	if fresh {
		return nil
	}

	var records []record
	for page := 1; page <= maxPages; page++ {
		var listing struct {
			Pagination struct {
				Pages int `json:"pages"`
			} `json:"pagination"`
			Releases []struct {
				ID    int `json:"id"`
				Basic struct {
					Title   string `json:"title"`
					Year    int    `json:"year"`
					Cover   string `json:"cover_image"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Formats []struct {
						Name         string   `json:"name"`
						Descriptions []string `json:"descriptions"`
					} `json:"formats"`
				} `json:"basic_information"`
			} `json:"releases"`
		}
		endpoint := fmt.Sprintf("%s/users/%s/collection/folders/0/releases?per_page=100&page=%d",
			c.base(), url.PathEscape(c.User), page)
		if err := c.getJSON(ctx, endpoint, &listing); err != nil {
			if have {
				c.logger().Warn("the collection could not be listed again; the last list is kept", "error", err)
				return nil
			}
			return fmt.Errorf("discogs: the collection could not be listed: %w", err)
		}
		for _, r := range listing.Releases {
			rec := record{id: r.ID, title: r.Basic.Title, year: r.Basic.Year, cover: r.Basic.Cover}
			for _, a := range r.Basic.Artists {
				if name := resolve.Simplify(artistName(a.Name)); name != "" {
					rec.artists = append(rec.artists, name)
				}
			}
			for _, f := range r.Basic.Formats {
				if strings.EqualFold(f.Name, "Vinyl") {
					rec.vinyl = true
				}
				for _, d := range f.Descriptions {
					if strings.EqualFold(d, "Compilation") {
						rec.compilation = true
					}
				}
			}
			if rec.id != 0 && rec.title != "" {
				records = append(records, rec)
			}
		}
		if page >= listing.Pagination.Pages {
			break
		}
	}

	c.mu.Lock()
	c.records, c.listed = records, c.now()
	c.mu.Unlock()
	c.logger().Info("the collection was listed", "records", len(records))
	return nil
}

// by is the records credited to an artist.
func (c *Collection) by(artist string) []record {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []record
	for _, r := range c.records {
		for _, name := range r.artists {
			if name == artist {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// load fetches the tracklists of an artist's records that are not held yet.
//
// The fetching outlives the caller. Whoever asked has a few seconds, and an
// artist with thirty records takes longer than that the first time: the
// caller is told it is not ready, and the next track by that artist finds it
// done.
func (c *Collection) load(ctx context.Context, artist string, records []record) error {
	c.mu.Lock()
	var missing []int
	for _, r := range records {
		if _, ok := c.tracks[r.id]; !ok {
			missing = append(missing, r.id)
		}
	}
	done, running := c.loading[artist]
	if len(missing) > 0 && !running {
		done = make(chan struct{})
		if c.loading == nil {
			c.loading = map[string]chan struct{}{}
		}
		c.loading[artist] = done
		go c.fetch(artist, missing, done)
		running = true
	}
	c.mu.Unlock()

	if !running {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("discogs: still reading the tracklists of %d records: %w", len(missing), ctx.Err())
	}
}

func (c *Collection) fetch(artist string, ids []int, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	defer func() {
		c.mu.Lock()
		delete(c.loading, artist)
		c.mu.Unlock()
		close(done)
	}()

	// A few at a time. One by one, an artist with a dozen records is not
	// read before whoever asked has stopped waiting; all at once is more
	// than Discogs allows in a minute.
	queue := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range queue {
				c.read(ctx, id)
			}
		}()
	}
	for _, id := range ids {
		queue <- id
	}
	close(queue)
	workers.Wait()
}

// read fetches one record's tracklist and keeps it.
func (c *Collection) read(ctx context.Context, id int) {
	var release struct {
		Tracklist []track `json:"tracklist"`
	}
	endpoint := c.base() + "/releases/" + strconv.Itoa(id)
	err := c.getJSON(ctx, endpoint, &release)
	var limited errRateLimited
	if errors.As(err, &limited) {
		// Asked to wait, and there is time to: nobody is waiting on this
		// any more who cannot wait a little longer.
		select {
		case <-ctx.Done():
			return
		case <-time.After(limited.wait):
		}
		err = c.getJSON(ctx, endpoint, &release)
	}
	if err != nil {
		// Not remembered as a record with no tracks: it is asked for again
		// the next time its artist is heard.
		c.logger().Warn("a record's tracklist could not be read", "release", id, "error", err)
		return
	}
	var titles []string
	for _, t := range release.Tracklist {
		titles = t.titles(titles)
	}
	c.mu.Lock()
	if c.tracks == nil {
		c.tracks = map[int][]string{}
	}
	c.tracks[id] = titles
	c.mu.Unlock()
}

// track is one line of a tracklist. A medley or a suite is a line with lines
// of its own, and any of them may be what a recogniser calls the track.
type track struct {
	Type  string  `json:"type_"`
	Title string  `json:"title"`
	Sub   []track `json:"sub_tracks"`
}

func (t track) titles(into []string) []string {
	if t.Type != "heading" {
		if title := resolve.Simplify(t.Title); title != "" {
			into = append(into, title)
		}
	}
	for _, sub := range t.Sub {
		into = sub.titles(into)
	}
	return into
}

// choose picks the record a title is on. A title that is on the record, to
// the letter, beats one that is part of a longer title. Among records that
// have it: an album before a compilation, always - the owner's choice, so
// that a greatest hits played through shows each song's own album; then one
// named as the source named the album; then the one last answered with,
// because a side is played through; then vinyl before the same album on a
// disc, because what asks here is listening to a record; then the earliest,
// as the likeliest to be the one meant.
func (c *Collection) choose(records []record, title, album string) (record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var exact, partial []record
	for _, r := range records {
		best := 0
		for _, have := range c.tracks[r.id] {
			switch {
			case have == title:
				best = 2
			case best == 0 && len(have) >= minPartial && len(title) >= minPartial &&
				(strings.Contains(have, title) || strings.Contains(title, have)):
				best = 1
			}
		}
		switch best {
		case 2:
			exact = append(exact, r)
		case 1:
			partial = append(partial, r)
		}
	}
	candidates := exact
	if len(candidates) == 0 {
		candidates = partial
	}
	if len(candidates) == 0 {
		return record{}, false
	}

	named := func(r record) bool {
		have := resolve.Simplify(r.title)
		return album != "" && have != "" && (have == album || strings.Contains(have, album) || strings.Contains(album, have))
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		switch {
		case a.compilation != b.compilation:
			return !a.compilation
		case named(a) != named(b):
			return named(a)
		case (a.id == c.last) != (b.id == c.last):
			return a.id == c.last
		case a.vinyl != b.vinyl:
			return a.vinyl
		case (a.year > 0) != (b.year > 0):
			return a.year > 0
		case a.year != b.year:
			return a.year < b.year
		}
		return a.id < b.id
	})
	c.last = candidates[0].id
	return candidates[0], true
}

// cover is a record's picture. Only the last one is kept: a side is one
// record, and Discogs asks that its pictures are not held longer than they
// are needed.
func (c *Collection) cover(ctx context.Context, r record) *resolve.Cover {
	c.mu.Lock()
	if c.sleeve != nil && c.sleeveOf == r.id {
		kept := c.sleeve
		c.mu.Unlock()
		return kept
	}
	c.mu.Unlock()

	if r.cover == "" {
		return nil
	}
	body, err := c.get(ctx, r.cover, resolve.MaxImage)
	if err != nil {
		c.logger().Warn("a record's cover could not be fetched", "release", r.id, "error", err)
		return nil
	}
	mime := http.DetectContentType(body)
	if !strings.HasPrefix(mime, "image/") {
		c.logger().Warn("a record's cover is not a picture", "release", r.id, "mime", mime)
		return nil
	}
	cover := &resolve.Cover{Image: body, MIME: mime, From: Name}
	c.mu.Lock()
	c.sleeve, c.sleeveOf = cover, r.id
	c.mu.Unlock()
	return cover
}

// errRateLimited carries how long Discogs asked to be left alone.
type errRateLimited struct{ wait time.Duration }

func (e errRateLimited) Error() string { return fmt.Sprintf("discogs: rate limited for %s", e.wait) }

func (c *Collection) getJSON(ctx context.Context, endpoint string, into any) error {
	body, err := c.get(ctx, endpoint, 4<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("discogs: unreadable answer: %w", err)
	}
	return nil
}

func (c *Collection) get(ctx context.Context, endpoint string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// Discogs refuses a caller that does not say who it is.
	req.Header.Set("User-Agent", c.UserAgent)
	// The token goes to the API and nowhere else: a cover is fetched from
	// another host, by an address that needs no telling who is asking.
	if c.Token != "" && strings.HasPrefix(endpoint, c.base()+"/") {
		req.Header.Set("Authorization", "Discogs token="+c.Token)
	}

	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if seconds < 1 || seconds > 120 {
			seconds = 60
		}
		return nil, errRateLimited{time.Duration(seconds) * time.Second}
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("discogs answered %s: the collection is private and the token is missing or wrong", resp.Status)
	default:
		return nil, fmt.Errorf("discogs answered %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("discogs sent more than %d bytes", limit)
	}
	return body, nil
}

var numbered = regexp.MustCompile(`\s*\(\d+\)$`)

// artistName is an artist as a person would write it. Discogs tells artists
// of one name apart with a number, "Chicago (2)", files some under their
// second word, "Beatles, The", and marks a name spelled another way on the
// sleeve with a star.
func artistName(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), "*")
	name = numbered.ReplaceAllString(name, "")
	if rest, ok := strings.CutSuffix(name, ", The"); ok {
		name = "The " + rest
	}
	return name
}

func (c *Collection) base() string {
	if c.Base != "" {
		return c.Base
	}
	return API
}

func (c *Collection) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Collection) refresh() time.Duration {
	if c.Refresh > 0 {
		return c.Refresh
	}
	return 6 * time.Hour
}

func (c *Collection) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Collection) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
