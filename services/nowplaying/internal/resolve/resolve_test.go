package resolve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// A real PNG signature followed by nothing much: enough for the content
// sniffer, which is all the resolver looks at.
var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR-not-a-real-image")

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fake is a catalogue that answers from a table and counts how often it was
// asked, which is what the caching promises are about.
type fake struct {
	name string
	hit  *Hit
	err  error

	mu    sync.Mutex
	asked []Query
}

func (f *fake) Name() string { return f.name }

func (f *fake) Lookup(_ context.Context, q Query) (*Hit, error) {
	f.mu.Lock()
	f.asked = append(f.asked, q)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.hit, nil
}

func (f *fake) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

// images serves cover bytes, and counts.
func images(t *testing.T, body []byte, status int) (*httptest.Server, *int) {
	t.Helper()
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func resolver(catalogues ...Catalogue) *Resolver {
	r := New(catalogues, "eds-test/0 ( test )", quiet)
	r.HTTP = &http.Client{Timeout: 2 * time.Second}
	return r
}

var kindOfBlue = contract.Track{Title: "So What", Artist: "Miles Davis", Album: "Kind of Blue"}

func TestTheFirstCatalogueThatHasItWins(t *testing.T) {
	srv, _ := images(t, png, http.StatusOK)
	first := &fake{name: "first", err: ErrNotFound}
	second := &fake{name: "second", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL + "/kob.png"}}
	third := &fake{name: "third", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL + "/other.png"}}

	got, err := resolver(first, second, third).Resolve(context.Background(), kindOfBlue)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cover.From != "second" || string(got.Cover.Image) != string(png) || got.Cover.MIME != "image/png" {
		t.Errorf("cover = from %q, %d bytes, %s", got.Cover.From, len(got.Cover.Image), got.Cover.MIME)
	}
	if third.calls() != 0 {
		t.Error("a catalogue was asked after one had already answered")
	}
}

// The promise that keeps this inside every catalogue's limits.
func TestOneLookupPerAlbumNotPerTrack(t *testing.T) {
	srv, fetched := images(t, png, http.StatusOK)
	cat := &fake{name: "cat", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL}}
	r := resolver(cat)

	for _, title := range []string{"So What", "Freddie Freeloader", "Blue in Green", "All Blues", "Flamenco Sketches"} {
		track := kindOfBlue
		track.Title = title
		if _, err := r.Resolve(context.Background(), track); err != nil {
			t.Fatal(err)
		}
	}
	if cat.calls() != 1 || *fetched != 1 {
		t.Errorf("five tracks of one album cost %d lookups and %d fetches, want 1 and 1", cat.calls(), *fetched)
	}
}

func TestNotFoundIsRememberedThenAskedAgain(t *testing.T) {
	cat := &fake{name: "cat", err: ErrNotFound}
	r := resolver(cat)
	now := time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := r.Resolve(context.Background(), kindOfBlue); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want not found", err)
		}
	}
	if cat.calls() != 1 {
		t.Errorf("a miss was looked up %d times, want once", cat.calls())
	}

	// But not forever: a release can be added to a catalogue.
	now = now.Add(r.MissTTL + time.Minute)
	_, _ = r.Resolve(context.Background(), kindOfBlue)
	if cat.calls() != 2 {
		t.Errorf("after the miss expired there were %d lookups, want 2", cat.calls())
	}
}

// Running out of time is not the same as nobody having it, and must not be
// remembered as if it were.
func TestATimeoutIsNotRememberedAsAMiss(t *testing.T) {
	cat := &fake{name: "cat", err: context.DeadlineExceeded}
	r := resolver(cat)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Resolve(ctx, kindOfBlue); errors.Is(err, ErrNotFound) {
		t.Error("a cancelled lookup was reported as not found")
	}

	cat.err = ErrNotFound
	_, _ = r.Resolve(context.Background(), kindOfBlue)
	if cat.calls() != 2 {
		t.Errorf("the cancelled lookup was cached: %d lookups, want 2", cat.calls())
	}
}

func TestACatalogueThatIsDownIsSkipped(t *testing.T) {
	srv, _ := images(t, png, http.StatusOK)
	down := &fake{name: "down", err: errors.New("connection refused")}
	up := &fake{name: "up", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL}}

	got, err := resolver(down, up).Resolve(context.Background(), kindOfBlue)
	if err != nil || got.Cover.From != "up" {
		t.Errorf("got %+v, %v", got.Cover, err)
	}
}

// A confident wrong cover lights the room for a record that is not playing.
// None is better.
func TestAnAnswerAboutSomethingElseIsRefused(t *testing.T) {
	srv, fetched := images(t, png, http.StatusOK)
	for name, hit := range map[string]*Hit{
		"a different artist": {Artist: "Miles Kane", Album: "Kind of Blue", ImageURL: srv.URL},
		"a different album":  {Artist: "Miles Davis", Album: "Bitches Brew", ImageURL: srv.URL},
		"no artist at all":   {Artist: "", Album: "Kind of Blue", ImageURL: srv.URL},
	} {
		t.Run(name, func(t *testing.T) {
			*fetched = 0
			_, err := resolver(&fake{name: "cat", hit: hit}).Resolve(context.Background(), kindOfBlue)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v, want not found", err)
			}
			if *fetched != 0 {
				t.Error("the wrong cover was fetched anyway")
			}
		})
	}
}

func TestEditionNotesAndSpellingDoNotHideTheRightAnswer(t *testing.T) {
	srv, _ := images(t, png, http.StatusOK)
	for name, hit := range map[string]*Hit{
		"an edition note":      {Artist: "Miles Davis", Album: "Kind of Blue (Legacy Edition)", ImageURL: srv.URL},
		"a bracketed note":     {Artist: "Miles Davis", Album: "Kind Of Blue [Remastered]", ImageURL: srv.URL},
		"case and punctuation": {Artist: "MILES DAVIS", Album: "kind of blue!", ImageURL: srv.URL},
		"a fuller credit":      {Artist: "Miles Davis Sextet", Album: "Kind of Blue", ImageURL: srv.URL},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolver(&fake{name: "cat", hit: hit}).Resolve(context.Background(), kindOfBlue); err != nil {
				t.Errorf("refused: %v", err)
			}
		})
	}

	for a, b := range map[string]string{"The Band": "Band", "The The": "the the", "Beyoncé": "BEYONCÉ"} {
		if !similar(a, b) {
			t.Errorf("%q and %q were not taken for the same name", a, b)
		}
	}
	for a, b := range map[string]string{"The The": "The Who", "Blue": "Red", "": "Anything", "(Live)": ""} {
		if similar(a, b) {
			t.Errorf("%q and %q were taken for the same name", a, b)
		}
	}
}

func TestWhatIsNotAnImageIsNotACover(t *testing.T) {
	for name, tc := range map[string]struct {
		body   []byte
		status int
	}{
		"an error page":   {[]byte("<html><body>Not here</body></html>"), http.StatusOK},
		"json":            {[]byte(`{"error":"quota"}`), http.StatusOK},
		"a server error":  {png, http.StatusInternalServerError},
		"gone":            {nil, http.StatusNotFound},
		"an empty answer": {nil, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := images(t, tc.body, tc.status)
			cat := &fake{name: "cat", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL}}
			if _, err := resolver(cat).Resolve(context.Background(), kindOfBlue); !errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v, want not found", err)
			}
		})
	}
}

func TestAnOversizedImageIsRefused(t *testing.T) {
	big := append(append([]byte{}, png...), make([]byte, MaxImage)...)
	srv, _ := images(t, big, http.StatusOK)
	cat := &fake{name: "cat", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", ImageURL: srv.URL}}
	if _, err := resolver(cat).Resolve(context.Background(), kindOfBlue); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
}

// A source with no album - TIDAL through Last.fm, say - asks a different
// question: which album is this track on?
func TestWithNoAlbumTheAlbumIsPartOfTheAnswer(t *testing.T) {
	srv, _ := images(t, png, http.StatusOK)
	cat := &fake{name: "cat", hit: &Hit{Artist: "Miles Davis", Album: "Kind of Blue", Title: "So What", ImageURL: srv.URL}}
	r := resolver(cat)

	got, err := r.Resolve(context.Background(), contract.Track{Title: "So What", Artist: "Miles Davis"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Album != "Kind of Blue" || got.Cover == nil {
		t.Errorf("got album %q, cover %v", got.Album, got.Cover != nil)
	}
	if q := cat.asked[0]; q.Album != "" || q.Title != "So What" || q.Artist != "Miles Davis" {
		t.Errorf("asked %+v", q)
	}

	// With no album to key on, each track is its own question.
	_, err = r.Resolve(context.Background(), contract.Track{Title: "All Blues", Artist: "Miles Davis"})
	if cat.calls() != 2 {
		t.Errorf("%d lookups for two albumless tracks, want 2", cat.calls())
	}
	// And the catalogue's answer here is about "So What", a different track:
	// its album is not this track's album, and must not be taken for it.
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("an answer about another track was accepted: %v", err)
	}
}

func TestACompilationIsLookedUpByItsAlbumArtist(t *testing.T) {
	cat := &fake{name: "cat", err: ErrNotFound}
	_, _ = resolver(cat).Resolve(context.Background(), contract.Track{
		Title: "Song", Artist: "Someone", AlbumArtist: "Various Artists", Album: "Hits"})
	if cat.asked[0].Artist != "Various Artists" {
		t.Errorf("asked for artist %q", cat.asked[0].Artist)
	}
}

func TestWithNothingToAskNothingIsAsked(t *testing.T) {
	cat := &fake{name: "cat", err: ErrNotFound}
	r := resolver(cat)
	for name, track := range map[string]contract.Track{
		"nothing":       {},
		"only a title":  {Title: "So What"},
		"only an album": {Album: "Kind of Blue"},
		"only artist":   {Artist: "Miles Davis"},
	} {
		if _, err := r.Resolve(context.Background(), track); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if cat.calls() != 0 {
		t.Errorf("a catalogue was asked %d times with nothing to ask", cat.calls())
	}
}

func TestTheCacheIsBounded(t *testing.T) {
	srv, _ := images(t, png, http.StatusOK)
	r := resolver()
	r.Keep = 3
	for i := 0; i < 10; i++ {
		album := fmt.Sprintf("Album %d", i)
		r.Catalogues = []Catalogue{&fake{name: "cat", hit: &Hit{Artist: "Artist", Album: album, ImageURL: srv.URL}}}
		if _, err := r.Resolve(context.Background(), contract.Track{Title: "T", Artist: "Artist", Album: album}); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cache) != 3 || len(r.order) != 3 {
		t.Errorf("cache holds %d entries, want 3", len(r.cache))
	}
}

// --- the real catalogues, against stand-ins that speak their shapes ----------

type route struct {
	path  string // prefix
	check func(r *http.Request) error
	body  string
}

func standIn(t *testing.T, routes ...route) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "eds-test/0 ( test )" {
			t.Errorf("request to %s carried User-Agent %q", r.URL.Path, r.Header.Get("User-Agent"))
		}
		for _, route := range routes {
			if strings.HasPrefix(r.URL.Path, route.path) {
				if route.check != nil {
					if err := route.check(r); err != nil {
						t.Errorf("%s: %v", r.URL.Path, err)
					}
				}
				_, _ = w.Write([]byte(route.body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wantQuery(key string, parts ...string) func(*http.Request) error {
	return func(r *http.Request) error {
		got := r.URL.Query().Get(key)
		for _, part := range parts {
			if !strings.Contains(got, part) {
				return fmt.Errorf("%s = %q, missing %q", key, got, part)
			}
		}
		return nil
	}
}

func TestDeezer(t *testing.T) {
	srv := standIn(t,
		route{"/search/album", wantQuery("q", `artist:"Miles Davis"`, `album:"Kind of Blue"`),
			`{"data":[
			  {"title":"Bitches Brew","cover_xl":"http://img/wrong.jpg","artist":{"name":"Miles Davis"}},
			  {"title":"Kind of Blue","cover_xl":"http://img/kob-xl.jpg","cover_big":"http://img/kob-big.jpg","artist":{"name":"Miles Davis"}}]}`},
		// Free text for tracks: the live service finds nothing when a track
		// search uses the field syntax. The answer leads with a near miss -
		// the right artist, a different track on a different album - as free
		// text does, and that one must not be taken.
		route{"/search/track", func(r *http.Request) error {
			if q := r.URL.Query().Get("q"); q != "Miles Davis So What" {
				return fmt.Errorf("q = %q, want plain text", q)
			}
			return nil
		}, `{"data":[
			  {"title":"Spanish Key","artist":{"name":"Miles Davis"},"album":{"title":"Bitches Brew","cover_xl":"http://img/wrong.jpg"}},
			  {"title":"So What","artist":{"name":"Miles Davis"},"album":{"title":"Kind of Blue","cover_xl":"","cover_big":"http://img/kob-big.jpg"}}]}`},
	)
	d := Deezer{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}

	// The first result is the wrong album; the catalogue must not take it.
	hit, err := d.Lookup(context.Background(), Query{Artist: "Miles Davis", Album: "Kind of Blue"})
	if err != nil || hit.ImageURL != "http://img/kob-xl.jpg" {
		t.Errorf("album lookup = %+v, %v", hit, err)
	}

	hit, err = d.Lookup(context.Background(), Query{Artist: "Miles Davis", Title: "So What"})
	if err != nil || hit.Album != "Kind of Blue" || hit.ImageURL != "http://img/kob-big.jpg" {
		t.Errorf("track lookup = %+v, %v", hit, err)
	}
}

func TestDeezerWithNothing(t *testing.T) {
	srv := standIn(t, route{"/search/album", nil, `{"data":[]}`})
	d := Deezer{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}
	if _, err := d.Lookup(context.Background(), Query{Artist: "Nobody", Album: "Nothing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

// A quote in a title is syntax to the catalogue. It must not end the term.
func TestAQuoteInANameDoesNotBreakTheSearch(t *testing.T) {
	srv := standIn(t, route{"/search/album", func(r *http.Request) error {
		q := r.URL.Query().Get("q")
		if strings.Count(q, `"`) != 4 {
			return fmt.Errorf("q = %q: the quotes are unbalanced", q)
		}
		return nil
	}, `{"data":[]}`})
	d := Deezer{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}
	_, _ = d.Lookup(context.Background(), Query{Artist: `The "Artist"`, Album: `An "Album"`})
}

func TestITunes(t *testing.T) {
	srv := standIn(t, route{"/search", func(r *http.Request) error {
		if r.URL.Query().Get("entity") != "album" {
			return fmt.Errorf("entity = %q", r.URL.Query().Get("entity"))
		}
		return wantQuery("term", "Miles Davis", "Kind of Blue")(r)
	}, `{"results":[{"artistName":"Miles Davis","collectionName":"Kind of Blue (Legacy Edition)",
	      "artworkUrl100":"https://is1-ssl.mzstatic.com/image/thumb/x/y/100x100bb.jpg"}]}`})
	i := ITunes{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}

	hit, err := i.Lookup(context.Background(), Query{Artist: "Miles Davis", Album: "Kind of Blue"})
	if err != nil {
		t.Fatal(err)
	}
	// The 100 px URL is no use on a strip; the larger one is asked for.
	if hit.ImageURL != "https://is1-ssl.mzstatic.com/image/thumb/x/y/600x600bb.jpg" {
		t.Errorf("image url = %q", hit.ImageURL)
	}
}

func TestITunesAsksForASongWhenTheAlbumIsUnknown(t *testing.T) {
	srv := standIn(t, route{"/search", func(r *http.Request) error {
		if r.URL.Query().Get("entity") != "song" {
			return fmt.Errorf("entity = %q", r.URL.Query().Get("entity"))
		}
		return nil
	}, `{"results":[{"artistName":"Miles Davis","trackName":"So What","collectionName":"Kind of Blue","artworkUrl100":"http://a/100x100bb.jpg"}]}`})
	i := ITunes{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}
	hit, err := i.Lookup(context.Background(), Query{Artist: "Miles Davis", Title: "So What"})
	if err != nil || hit.Album != "Kind of Blue" {
		t.Errorf("got %+v, %v", hit, err)
	}
}

func TestMusicBrainz(t *testing.T) {
	srv := standIn(t, route{"/ws/2/release-group/", wantQuery("query", `artist:"Miles Davis"`, `releasegroup:"Kind of Blue"`),
		`{"release-groups":[{"id":"8e8a594f-2175-38c7-a871-abb68ec363e7","title":"Kind of Blue",
		  "artist-credit":[{"name":"Miles Davis"}]}]}`})
	m := MusicBrainz{Base: srv.URL, CoverBase: "https://caa.example", HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}

	hit, err := m.Lookup(context.Background(), Query{Artist: "Miles Davis", Album: "Kind of Blue"})
	if err != nil {
		t.Fatal(err)
	}
	if hit.ImageURL != "https://caa.example/release-group/8e8a594f-2175-38c7-a871-abb68ec363e7/front-500" {
		t.Errorf("image url = %q", hit.ImageURL)
	}

	// It answers album questions only, and must not be asked a track one.
	if _, err := m.Lookup(context.Background(), Query{Artist: "Miles Davis", Title: "So What"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a track question got %v", err)
	}
}

func TestACatalogueErrorIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	d := Deezer{Base: srv.URL, HTTP: srv.Client(), UserAgent: "eds-test/0 ( test )"}
	_, err := d.Lookup(context.Background(), Query{Artist: "A", Album: "B"})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("a rate limit was reported as %v", err)
	}
}

func TestNamed(t *testing.T) {
	got, err := Named([]string{"deezer", " iTunes ", "", "musicbrainz"}, http.DefaultClient, "ua")
	if err != nil || len(got) != 3 || got[0].Name() != "deezer" || got[1].Name() != "itunes" || got[2].Name() != "musicbrainz" {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := Named([]string{"discogs"}, http.DefaultClient, "ua"); err == nil {
		t.Error("an unknown catalogue was accepted")
	}
}
