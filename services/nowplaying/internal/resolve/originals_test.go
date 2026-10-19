package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The catalogs as they answered on 2026-10-19, cut down to what the tests need.
type catalogs struct {
	t        *testing.T
	mu       sync.Mutex
	asked    []string
	mbBroken bool
}

func (c *catalogs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.asked = append(c.asked, r.URL.Path+"?"+r.URL.RawQuery)
	broken := c.mbBroken
	c.mu.Unlock()
	q := r.URL.Query().Get("q")
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	type artist struct {
		Name string `json:"name"`
	}
	type album struct {
		ID    int64  `json:"id,omitempty"`
		Title string `json:"title"`
	}
	type track struct {
		Title  string `json:"title"`
		Artist artist `json:"artist"`
		Album  album  `json:"album"`
	}
	pf := artist{"Pink Floyd"}
	switch {
	case r.URL.Path == "/search/track":
		var data []track
		switch q {
		case "Pink Floyd Pigs On the Wing, Pt. 1":
			// As on 2026-10-19: nothing, for the title as the recognizer
			// writes it.
		case "Pink Floyd pigs on the wing 1":
			data = []track{
				{"Pigs on the Wing 1", pf, album{Title: "Animals"}},
				{"Pigs on the Wing 2", pf, album{Title: "Animals"}},
				{"Pigs on the Wing (8-Track Version)", pf, album{Title: "8-Tracks"}},
				{"Pigs on the Wing 1 (2018 Remix)", pf, album{Title: "Animals (2018 Remix)"}},
			}
		case "Pink Floyd Dogs":
			data = []track{
				{"Dogs", pf, album{Title: "Animals"}},
				{"Dogs (2018 Remix)", pf, album{Title: "Animals (2018 Remix)"}},
			}
		case "Pink Floyd Time":
			data = []track{
				{"Time", pf, album{Title: "The Dark Side of the Moon"}},
				{"Time (Live)", pf, album{Title: "Pulse (Live)"}},
				{"Time", pf, album{Title: "A Foot in the Door: The Best of Pink Floyd"}},
			}
		case "Pink Floyd Speak to Me/Breathe":
			data = []track{
				// Tributes by artists whose names contain the band's.
				{"Speak To Me / Breathe In The Air", artist{"The Machine Performs Pink Floyd"}, album{Title: "Symphonic Side Of The Moon"}},
				{"Speak to Me / Breathe", artist{"The Pink Floyd Story"}, album{Title: "The Great Gig in the Sky"}},
				// A single named after a track, listed first.
				{"The Great Gig in the Sky", pf, album{Title: "The Great Gig in the Sky"}},
				{"Speak to Me", pf, album{Title: "The Great Gig in the Sky"}},
				{"Speak to Me", pf, album{Title: "The Dark Side Of The Moon (50th Anniversary) [2023 Remaster]"}},
				{"Speak to Me", pf, album{Title: "The Dark Side of the Moon"}},
			}
		case "Pink Floyd See Emily Play":
			data = []track{{"See Emily Play", pf, album{Title: "Relics"}}}
		}
		reply(map[string]any{"data": data})
	case r.URL.Path == "/search/album":
		type hit struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Artist artist `json:"artist"`
		}
		var data []hit
		switch {
		case strings.Contains(q, `album:"The Dark Side of the Moon"`):
			// The edition first, as Deezer lists it.
			data = []hit{{1, "The Dark Side Of The Moon (50th Anniversary) [2023 Remaster]", pf}, {2, "The Dark Side of the Moon", pf}}
		case strings.Contains(q, `album:"Animals"`):
			data = []hit{{3, "Animals", pf}}
		}
		reply(map[string]any{"data": data})
	case r.URL.Path == "/album/2/tracks":
		reply(map[string]any{"data": []album{{Title: "Speak to Me"}, {Title: "Breathe (In the Air)"}, {Title: "On the Run"}, {Title: "Time"}, {Title: "The Great Gig in the Sky"}, {Title: "Money"}}})
	case r.URL.Path == "/album/3/tracks":
		reply(map[string]any{"data": []album{{Title: "Pigs on the Wing 1"}, {Title: "Dogs"}, {Title: "Pigs (Three Different Ones)"}, {Title: "Sheep"}, {Title: "Pigs on the Wing 2"}}})
	case r.URL.Path == "/ws/2/release-group/":
		if broken {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		type group struct {
			Title     string   `json:"title"`
			Primary   string   `json:"primary-type"`
			Secondary []string `json:"secondary-types,omitempty"`
			First     string   `json:"first-release-date"`
			Credit    []artist `json:"artist-credit"`
		}
		query := r.URL.Query().Get("query")
		var groups []group
		switch {
		case strings.Contains(query, `releasegroup:"Animals"`):
			groups = []group{
				{"Animals Test Acetate", "Album", []string{"Demo"}, "2008", []artist{pf}},
				{"Animals", "Album", nil, "1977-01-23", []artist{pf}},
			}
		case strings.Contains(query, `releasegroup:"The Dark Side of the Moon"`), strings.Contains(query, `releasegroup:"The Dark Side Of The Moon"`):
			groups = []group{{"The Dark Side of the Moon", "Album", nil, "1973-03-01", []artist{pf}}}
		case strings.Contains(query, `releasegroup:"Relics"`):
			groups = []group{{"Relics", "Album", []string{"Compilation"}, "1971-05-14", []artist{pf}}}
		}
		reply(map[string]any{"release-groups": groups})
	default:
		http.NotFound(w, r)
	}
}

func originals(t *testing.T) (*Originals, *catalogs) {
	t.Helper()
	c := &catalogs{t: t}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	return &Originals{Deezer: srv.URL, MusicBrainz: srv.URL, HTTP: srv.Client(), UserAgent: "test", gapMB: time.Millisecond}, c
}

func TestTheOriginalAlbumIsTheStudioAlbum(t *testing.T) {
	o, _ := originals(t)
	for _, c := range []struct{ title, hint, want string }{
		// The evening of 2026-10-18: Animals, put on after Dark Side.
		{"Pigs On the Wing, Pt. 1", "", "Animals"},
		{"Dogs", "The Dark Side of the Moon", "Animals"},
		// Not the live album, not the best-of.
		{"Time", "", "The Dark Side of the Moon"},
		// No album from the recognizer, and the anniversary edition listed
		// first: the original.
		{"Speak to Me/Breathe", "", "The Dark Side of the Moon"},
	} {
		got, err := o.Original(context.Background(), "Pink Floyd", c.title, c.hint)
		if err != nil || got != c.want {
			t.Errorf("%q: %q, %v; want %q", c.title, got, err, c.want)
		}
	}
}

// A track that only ever came out on a compilation has no original: the
// caller keeps what the recognizer said.
func TestATrackOnlyOnACompilationHasNoOriginal(t *testing.T) {
	o, _ := originals(t)
	if got, err := o.Original(context.Background(), "Pink Floyd", "See Emily Play", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %q, %v; want ErrNotFound", got, err)
	}
}

func TestAnAlbumHasWhatIsOnIt(t *testing.T) {
	o, _ := originals(t)
	for _, c := range []struct {
		album, title string
		want         bool
	}{
		{"The Dark Side of the Moon", "Time", true},                     // the other side
		{"The Dark Side of the Moon", "Breathe (In the Air)", true},     // as listed
		{"The Dark Side of the Moon", "Pigs On the Wing, Pt. 1", false}, // another record
		{"Animals", "Pigs On the Wing, Pt. 1", true},
		{"Animals", "Dogs", true},
	} {
		got, err := o.Has(context.Background(), "Pink Floyd", c.album, c.title)
		if err != nil || got != c.want {
			t.Errorf("%s has %q: %v, %v; want %v", c.album, c.title, got, err, c.want)
		}
	}
	if _, err := o.Has(context.Background(), "Pink Floyd", "Wish You Were Here", "Have a Cigar"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an album Deezer does not have: %v, want ErrNotFound", err)
	}
}

func TestAnswersAreHeld(t *testing.T) {
	o, c := originals(t)
	ctx := context.Background()
	o.Original(ctx, "Pink Floyd", "Pigs On the Wing, Pt. 1", "")
	o.Has(ctx, "Pink Floyd", "Animals", "Dogs")
	before := len(c.asked)
	o.Original(ctx, "Pink Floyd", "Pigs On the Wing, Pt. 1", "")
	o.Has(ctx, "Pink Floyd", "Animals", "Sheep")
	o.Has(ctx, "Pink Floyd", "Wish You Were Here", "Have a Cigar")
	o.Has(ctx, "Pink Floyd", "Wish You Were Here", "Welcome to the Machine")
	// One more: the album Deezer does not have, asked once and remembered.
	if after := len(c.asked); after != before+1 {
		t.Errorf("asked %d more times, want 1: %v", after-before, c.asked[before:])
	}
}

// MusicBrainz down: nothing is known to be an original, and the first album
// Deezer names that does not say it is an edition is taken.
func TestWithoutMusicBrainzTheFirstPlainAlbumIsTaken(t *testing.T) {
	o, c := originals(t)
	c.mbBroken = true
	got, err := o.Original(context.Background(), "Pink Floyd", "Speak to Me/Breathe", "")
	if err != nil || got != "The Dark Side of the Moon" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestSameTrack(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Pigs on the Wing 1", "Pigs On the Wing, Pt. 1", true},
		{"Pigs on the Wing (Part One)", "Pigs on the Wing 1", true},
		{"Pigs on the Wing 1", "Pigs on the Wing 2", false},
		{"Time", "Time - 2011 Remaster", true},
		{"Time (Live)", "Time", true},
		{"Speak to Me", "Speak to Me/Breathe", true},
		{"Time", "Timeless", false},
		{"Money", "Time", false},
		{"Us", "Us and Them", false}, // one word is not enough to be the start of another
		{"", "Time", false},
	} {
		if got := SameTrack(c.a, c.b); got != c.want {
			t.Errorf("SameTrack(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// The album the recognizer named, carried over from the record before, is
// not taken when its own track list says the track is not on it.
func TestACarriedOverAlbumIsNotTaken(t *testing.T) {
	o, _ := originals(t)
	if got, err := o.Original(context.Background(), "Pink Floyd", "Have a Cigar", "The Dark Side of the Moon"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %q, %v; want ErrNotFound", got, err)
	}
	// Nor is another whose track list lacks it.
	if got, err := o.Original(context.Background(), "Pink Floyd", "Have a Cigar", "Animals"); err == nil {
		t.Errorf("Animals does not have it either, got %q", got)
	}
}
