package resolve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Originals names the record a track heard on the turntable is on, by what is
// on it (ADR-0038). It answers two questions, and holds every answer:
//
//   - Has: is this track on this album? It is what keeps a side's sleeve
//     across a flip, and across a track the recognizer files under a box set,
//     without a time to wait out.
//   - Original: which original album is this track on? Of the albums a track
//     is on, the first studio album to be released: not a compilation, a box
//     set, a live album, a remix, a demo or an anniversary edition, when an
//     original exists.
//
// Deezer finds: it is asked which albums a track is on, and what is on an
// album. MusicBrainz classifies: it is asked what kind of release each album
// is, and when it first came out. Its search is good at the second and poor at
// the first - asked for Pink Floyd's "Time" it returns live recordings before
// The Dark Side of the Moon - which is why it is not asked to find.
type Originals struct {
	Deezer      string // https://api.deezer.com
	MusicBrainz string // https://musicbrainz.org
	HTTP        *http.Client
	UserAgent   string
	Logger      *slog.Logger

	// Candidates bounds how many albums are classified for one track:
	// MusicBrainz allows one call a second.
	Candidates int

	mu        sync.Mutex
	tracklist map[string][]string // album key: its tracks, as Deezer lists them
	listed    map[string]bool     // album key: asked, and nothing found
	kinds     map[string]kind     // album title: what kind of release it is
	originals map[string]string   // track key: its original album, or "" for none
	lastMB    time.Time
	gapMB     time.Duration // between MusicBrainz calls; zero means its one a second
}

// kind is what MusicBrainz says an album is.
type kind struct {
	known    bool   // MusicBrainz has it
	original bool   // a studio album, with no secondary type
	released string // its first release date, YYYY[-MM[-DD]]
	title    string // MusicBrainz's name for it
}

// Has reports whether an album's track list has this track. ErrNotFound means
// the album's tracks could not be found.
func (o *Originals) Has(ctx context.Context, artist, album, title string) (bool, error) {
	tracks, err := o.tracks(ctx, artist, album)
	if err != nil {
		return false, err
	}
	for _, t := range tracks {
		if SameTrack(t, title) {
			return true, nil
		}
	}
	return false, nil
}

// Original names the original album a track is on. hint is the album the
// recognizer named. It is asked about only when the catalog has no album for
// the track, and counts only if it is an original: an album the catalog does
// not say has the track must not outrank one it does, however old it is.
// ErrNotFound means no album could be named.
func (o *Originals) Original(ctx context.Context, artist, title, hint string) (string, error) {
	key := simplify(artist) + "\x00" + trackWords(title)
	o.mu.Lock()
	album, cached := o.originals[key]
	o.mu.Unlock()
	if cached {
		if album == "" {
			return "", ErrNotFound
		}
		return album, nil
	}

	candidates, singles, err := o.candidates(ctx, artist, title)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	if len(candidates) == 0 && strings.TrimSpace(hint) != "" {
		// The catalog has no album for the track. The source's is asked
		// about, unless its own track list says the track is not on it: that
		// is the album of the record before, carried over.
		if has, err := o.Has(ctx, artist, hint, title); err != nil || has {
			candidates = []string{hint}
		}
	}

	limit := o.Candidates
	if limit <= 0 {
		limit = 4
	}
	var originals, unknown, named []kind
	classified := 0
	for _, c := range candidates {
		if edition(c) {
			// A remix, a live album, a compilation by its name: not the
			// original, and not worth a call to ask.
			continue
		}
		if classified >= limit {
			unknown = append(unknown, kind{title: c})
			continue
		}
		classified++
		k, err := o.kind(ctx, artist, c)
		if err != nil {
			// MusicBrainz could not be asked. The album is not known to be
			// anything, which ranks it after a known original and before
			// nothing.
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			o.logger().Warn("musicbrainz could not be asked what an album is", "artist", artist, "album", c, "error", err)
			unknown = append(unknown, kind{title: c})
			continue
		}
		switch {
		case k.original:
			originals = append(originals, k)
		case !k.known:
			unknown = append(unknown, kind{title: c})
		}
	}
	// An album named after one of the tracks is a single, likely as not:
	// taken only when nothing else is left.
	plain := unknown[:0]
	for _, k := range unknown {
		if singles[trackWords(k.title)] {
			named = append(named, k)
		} else {
			plain = append(plain, k)
		}
	}
	unknown = append(plain, named...)

	switch {
	case len(originals) > 0:
		// The first released. A date MusicBrainz does not have sorts last.
		sort.SliceStable(originals, func(i, j int) bool {
			a, b := originals[i].released, originals[j].released
			if a == "" || b == "" {
				return a != "" && b == ""
			}
			return a < b
		})
		album = originals[0].title
	case len(unknown) > 0:
		album = unknown[0].title
	default:
		album = ""
	}
	o.mu.Lock()
	if o.originals == nil {
		o.originals = make(map[string]string)
	}
	o.originals[key] = album
	o.mu.Unlock()
	if album == "" {
		return "", ErrNotFound
	}
	return album, nil
}

// candidates are the albums Deezer says a track is on, in its order, which
// puts the obvious first, and the titles of the tracks it answered with: an
// album named like one of them is probably a single. When the title as heard
// finds nothing, it is asked again in plain words: "Pigs On the Wing, Pt. 1"
// finds nothing where "pigs on the wing 1" finds Animals.
func (o *Originals) candidates(ctx context.Context, artist, title string) ([]string, map[string]bool, error) {
	out, singles, err := o.search(ctx, artist, title, title)
	if errors.Is(err, ErrNotFound) {
		if plain := trackWords(title); plain != strings.ToLower(strings.TrimSpace(title)) {
			return o.search(ctx, artist, plain, title)
		}
	}
	return out, singles, err
}

func (o *Originals) search(ctx context.Context, artist, asked, title string) ([]string, map[string]bool, error) {
	var found struct {
		Data []struct {
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				Title string `json:"title"`
			} `json:"album"`
		} `json:"data"`
	}
	// Free text, as the cover search asks: the artist: and track: fields
	// return noise for tracks (checked 2026-10-19).
	term := quoted(artist) + " " + quoted(asked)
	if err := getJSON(ctx, o.HTTP, o.UserAgent, o.Deezer+"/search/track?limit=25&q="+url.QueryEscape(term), &found); err != nil {
		return nil, nil, err
	}
	var out []string
	fromEdition := map[int]bool{} // which of out was named by stripping an edition
	singles := map[string]bool{}
	for _, t := range found.Data {
		// The artist exactly, not a name containing it: "The Machine Performs
		// Pink Floyd" is not Pink Floyd, and its tributes are not candidates.
		if simplify(artist) != simplify(t.Artist.Name) {
			continue
		}
		singles[trackWords(t.Title)] = true
		if !SameTrack(t.Title, title) || strings.TrimSpace(t.Album.Title) == "" {
			continue
		}
		album, stripped := t.Album.Title, false
		if edition(album) {
			// An edition points at its original: "The Dark Side Of The Moon
			// (50th Anniversary) [2023 Remaster]" is a reason to ask about
			// "The Dark Side Of The Moon". One that is still an edition
			// without its brackets - a best-of - is not a candidate at all.
			if base := stripEdition(album); base != album && base != "" && !edition(base) {
				album, stripped = base, true
			} else {
				continue
			}
		}
		at := -1
		for i, a := range out {
			if exactly(a, album) {
				at = i
			}
		}
		switch {
		case at < 0:
			fromEdition[len(out)] = stripped
			out = append(out, album)
		case fromEdition[at] && !stripped:
			// The album as the catalog lists it, in place of the name an
			// edition gave it: "The Dark Side of the Moon", not "...Of The...".
			out[at], fromEdition[at] = album, false
		}
	}
	if len(out) == 0 {
		return nil, singles, ErrNotFound
	}
	return out, singles, nil
}

// kind asks MusicBrainz what an album is.
func (o *Originals) kind(ctx context.Context, artist, album string) (kind, error) {
	name := lettersOnly(album, true)
	o.mu.Lock()
	k, cached := o.kinds[simplify(artist)+"\x00"+name]
	o.mu.Unlock()
	if cached {
		return k, nil
	}

	var found struct {
		Groups []struct {
			Title     string   `json:"title"`
			Primary   string   `json:"primary-type"`
			Secondary []string `json:"secondary-types"`
			First     string   `json:"first-release-date"`
			Credit    []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
		} `json:"release-groups"`
	}
	term := fmt.Sprintf(`artist:"%s" AND releasegroup:"%s"`, quoted(artist), quoted(stripEdition(album)))
	endpoint := o.MusicBrainz + "/ws/2/release-group/?fmt=json&limit=10&query=" + url.QueryEscape(term)
	// MusicBrainz answers 503 to a caller it thinks is too quick, and asks
	// it to slow down. Once more, after a pause, is as patient as a lookup
	// that someone is waiting on can be.
	for attempt := 0; ; attempt++ {
		if err := o.waitMB(ctx); err != nil {
			return kind{}, err
		}
		err := getJSON(ctx, o.HTTP, o.UserAgent, endpoint, &found)
		if err == nil {
			break
		}
		if attempt > 0 || !strings.Contains(err.Error(), "503") {
			return kind{}, err
		}
		o.mu.Lock()
		o.lastMB = time.Now().Add(time.Second)
		o.mu.Unlock()
	}

	k = kind{title: album}
	for _, g := range found.Groups {
		names := make([]string, 0, len(g.Credit))
		for _, c := range g.Credit {
			names = append(names, c.Name)
		}
		// The album itself, not one whose name contains it: "Animals" is not
		// "Animals Test Acetate".
		if lettersOnly(g.Title, true) != name || !similar(artist, strings.Join(names, " ")) {
			continue
		}
		k.known = true
		if g.Primary == "Album" && len(g.Secondary) == 0 {
			if !k.original || (g.First != "" && (k.released == "" || g.First < k.released)) {
				k.released = g.First
			}
			k.original, k.title = true, g.Title
		}
	}
	o.mu.Lock()
	if o.kinds == nil {
		o.kinds = make(map[string]kind)
	}
	o.kinds[simplify(artist)+"\x00"+name] = k
	o.mu.Unlock()
	return k, nil
}

// tracks is an album's track list, from Deezer, held once fetched.
func (o *Originals) tracks(ctx context.Context, artist, album string) ([]string, error) {
	key := simplify(artist) + "\x00" + simplify(album)
	o.mu.Lock()
	tracks, cached := o.tracklist[key]
	missing := o.listed[key]
	o.mu.Unlock()
	if cached {
		return tracks, nil
	}
	if missing {
		return nil, ErrNotFound
	}

	var albums struct {
		Data []struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"data"`
	}
	term := fmt.Sprintf(`artist:"%s" album:"%s"`, quoted(artist), quoted(album))
	if err := getJSON(ctx, o.HTTP, o.UserAgent, o.Deezer+"/search/album?limit=5&q="+url.QueryEscape(term), &albums); err != nil {
		return nil, o.miss(key, err)
	}
	var id int64
	for _, pass := range []func(string) bool{
		func(t string) bool { return exactly(t, album) },
		func(t string) bool { return similar(t, album) },
	} {
		for _, a := range albums.Data {
			if id == 0 && a.ID != 0 && similar(artist, a.Artist.Name) && pass(a.Title) {
				id = a.ID
			}
		}
	}
	if id == 0 {
		return nil, o.miss(key, ErrNotFound)
	}

	var listing struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := getJSON(ctx, o.HTTP, o.UserAgent, fmt.Sprintf("%s/album/%d/tracks?limit=100", o.Deezer, id), &listing); err != nil {
		return nil, o.miss(key, err)
	}
	for _, t := range listing.Data {
		tracks = append(tracks, t.Title)
	}
	if len(tracks) == 0 {
		return nil, o.miss(key, ErrNotFound)
	}
	o.mu.Lock()
	if o.tracklist == nil {
		o.tracklist = make(map[string][]string)
	}
	o.tracklist[key] = tracks
	o.mu.Unlock()
	return tracks, nil
}

// miss remembers that an album's tracks are not to be had, when that is the
// answer and not a failure to ask.
func (o *Originals) miss(key string, err error) error {
	if errors.Is(err, ErrNotFound) {
		o.mu.Lock()
		if o.listed == nil {
			o.listed = make(map[string]bool)
		}
		o.listed[key] = true
		o.mu.Unlock()
	}
	return err
}

// waitMB keeps to MusicBrainz's one call a second.
func (o *Originals) waitMB(ctx context.Context) error {
	gap := o.gapMB
	if gap == 0 {
		gap = 1500 * time.Millisecond
	}
	o.mu.Lock()
	wait := time.Until(o.lastMB.Add(gap))
	o.mu.Unlock()
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	o.mu.Lock()
	o.lastMB = time.Now()
	o.mu.Unlock()
	return nil
}

func (o *Originals) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

// editionRE finds what says an album is not an original: a remix, a remaster,
// a live album, an anniversary or deluxe edition, a compilation or a box.
var editionRE = regexp.MustCompile(`(?i)\b(remix(ed)?|remaster(ed)?|live|anniversary|deluxe|edition|expanded|reissue|demos?|best of|greatest hits|hits|collection|anthology|essentials?|box|compilation|singles|8-tracks?|in concert|unplugged|version)\b`)

// edition reports whether an album's name says it is not an original.
func edition(album string) bool { return editionRE.MatchString(album) }

// stripEdition is a name without what is in brackets, for asking MusicBrainz
// for the album an edition is of.
func stripEdition(album string) string {
	var out strings.Builder
	depth := 0
	for _, r := range album {
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
		if depth == 0 {
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}

func containsAlbum(albums []string, album string) bool {
	for _, a := range albums {
		if exactly(a, album) {
			return true
		}
	}
	return false
}

// exactly compares two names letter for letter, brackets and all, ignoring
// only case, punctuation and a leading article: "The Dark Side of the Moon" is
// not "The Dark Side Of The Moon (50th Anniversary)".
func exactly(a, b string) bool {
	x, y := strings.TrimPrefix(lettersOnly(a, false), "the"), strings.TrimPrefix(lettersOnly(b, false), "the")
	return x != "" && x == y
}

// SameTrack reports whether two track titles are one track, the way a track
// list and a recognizer write them: "Pigs on the Wing 1" and "Pigs On the
// Wing, Pt. 1" are one; "Time (2011 Remaster)" and "Time - 2011 Remaster"
// are "Time"; and "Speak to Me/Breathe", which the recognizer names as one,
// is the track list's "Speak to Me".
func SameTrack(a, b string) bool {
	x, y := strings.Fields(trackWords(a)), strings.Fields(trackWords(b))
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	if len(x) > len(y) {
		x, y = y, x
	}
	if len(x) == len(y) {
		return strings.Join(x, " ") == strings.Join(y, " ")
	}
	// The shorter is the start of the longer, and long enough to mean it.
	if len(x) < 2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

var numberWords = map[string]string{"one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "i": "1", "ii": "2", "iii": "3"}

// trackWords is a title as words: lowercased, without what is in brackets or
// after " - ", with "Pt." and "Part" dropped and small numbers written as
// digits.
func trackWords(title string) string {
	if i := strings.Index(title, " - "); i > 0 {
		title = title[:i]
	}
	title = stripEdition(title)
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	}) {
		w = strings.Trim(w, "'")
		switch w {
		case "", "pt", "part":
			continue
		}
		if d, ok := numberWords[w]; ok {
			w = d
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}
