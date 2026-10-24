package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// MusicBrainz as it answered for Van Halen on 2026-10-24, cut down: the band
// beside its members, and its release groups over two pages, in no order.
type musicbrainz struct {
	mu    sync.Mutex
	asked []string
}

func (m *musicbrainz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.asked = append(m.asked, r.URL.Path+"?"+r.URL.RawQuery)
	m.mu.Unlock()
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	type credit struct {
		Name string `json:"name"`
	}
	type group struct {
		Title     string   `json:"title"`
		Primary   string   `json:"primary-type"`
		Secondary []string `json:"secondary-types"`
		First     string   `json:"first-release-date"`
		Credit    []credit `json:"artist-credit"`
	}
	vh := []credit{{"Van Halen"}}

	switch r.URL.Path {
	case "/ws/2/artist/":
		type alias struct {
			Name string `json:"name"`
		}
		type artist struct {
			ID      string  `json:"id"`
			Name    string  `json:"name"`
			Aliases []alias `json:"aliases,omitempty"`
		}
		var found []artist
		switch r.URL.Query().Get("query") {
		case `artist:"Van Halen" OR alias:"Van Halen"`, `artist:"van halen" OR alias:"van halen"`:
			found = []artist{{ID: "b665b768", Name: "Van Halen"}, {ID: "2a66baa5", Name: "Edward Van Halen"}, {ID: "e90678bc", Name: "Alex Van Halen"}}
		case `artist:"Aerosmith" OR alias:"Aerosmith"`:
			found = []artist{{ID: "e7ec8c9c", Name: "Aerosmith Rocks!"}} // only a tribute
		case `artist:"Leonard Skinner" OR alias:"Leonard Skinner"`:
			// As MusicBrainz answered on 2026-10-24: Leonard Bernstein
			// before the band, which has the name as an alias.
			found = []artist{
				{ID: "b1", Name: "Leonard Bernstein"},
				{ID: "ae2b1b7c", Name: "Lynyrd Skynyrd", Aliases: []alias{{"Leonard Skinner"}, {"Lynyrd Skynard"}}},
			}
		case `artist:"acdc" OR alias:"acdc"`:
			found = []artist{{ID: "66c662b6", Name: "AC/DC"}, {ID: "0d6d8a2e", Name: "DJ AC-DC"}}
		}
		reply(map[string]any{"artists": found})

	case "/ws/2/release-group":
		// Asked for without the bootlegs, which this stand-in does not list.
		if r.URL.Query().Get("release-group-status") != "website-default" {
			http.Error(w, "bootlegs asked for", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("artist") != "b665b768" {
			reply(map[string]any{"release-group-count": 0, "release-groups": []group{}})
			return
		}
		pages := [][]group{{
			{"1984", "Album", nil, "1984-01-09", vh},
			{"Live: Right Here, Right Now", "Album", []string{"Live"}, "1993-02-23", vh},
			{"Van Halen II", "Album", nil, "1979-03-26", vh},
			{"The Best of Both Worlds", "Album", []string{"Compilation"}, "2004-07-20", vh},
		}, {
			{"Van Halen", "Album", nil, "1978-02-10", vh},
			{"A Different Kind of Truth", "Album", nil, "2012-02-07", vh},
			{"An Album Without a Date", "Album", nil, "", vh},
			{"A Collaboration", "Album", nil, "1990-01-01", []credit{{"Van Halen"}, {"Someone Else"}}},
		}}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page := map[int]int{0: 0, 4: 1}[offset]
		reply(map[string]any{"release-group-count": 8, "release-groups": pages[page]})

	default:
		http.NotFound(w, r)
	}
}

func discographer(t *testing.T) (*Originals, *musicbrainz) {
	t.Helper()
	m := &musicbrainz{}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return &Originals{MusicBrainz: srv.URL, HTTP: srv.Client(), UserAgent: "test", gapMB: time.Millisecond}, m
}

func TestAnArtistsStudioAlbumsOldestFirst(t *testing.T) {
	o, _ := discographer(t)
	d, err := o.Discography(context.Background(), "Van Halen")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range d.Albums {
		got = append(got, a.Title)
	}
	want := []string{"Van Halen", "Van Halen II", "1984", "A Different Kind of Truth", "An Album Without a Date"}
	if d.Artist != "Van Halen" || len(got) != len(want) {
		t.Fatalf("%s: %q", d.Artist, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("album %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestTheNthAlbum(t *testing.T) {
	o, _ := discographer(t)
	d, err := o.Discography(context.Background(), "Van Halen")
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int]string{1: "Van Halen", 2: "Van Halen II", -1: "An Album Without a Date"} {
		if a, err := d.Nth(n); err != nil || a.Title != want {
			t.Errorf("Nth(%d) = %q, %v; want %q", n, a.Title, err, want)
		}
	}
	for _, n := range []int{0, 6, -6} {
		if _, err := d.Nth(n); !errors.Is(err, ErrNotFound) {
			t.Errorf("Nth(%d): err = %v", n, err)
		}
	}
}

// The name is MusicBrainz's, whatever case it was heard in, and it is read
// once: a second request for the band asks nothing.
func TestADiscographyIsHeld(t *testing.T) {
	o, m := discographer(t)
	if _, err := o.Discography(context.Background(), "van halen"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	asked := len(m.asked)
	m.mu.Unlock()
	if asked != 3 {
		t.Errorf("asked %d times, want the artist and two pages", asked)
	}
	d, err := o.Discography(context.Background(), "Van Halen")
	if err != nil || d.Artist != "Van Halen" {
		t.Fatalf("%+v, %v", d, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.asked) != asked {
		t.Errorf("asked again: %q", m.asked[asked:])
	}
}

// Heard as "acdc", the band is AC/DC, by its alias and by its letters.
func TestAnArtistFoundByAliasIsNamedAsMusicBrainzNamesThem(t *testing.T) {
	o, _ := discographer(t)
	_, err := o.Discography(context.Background(), "acdc")
	// It has no albums here; what matters is that it was found by name.
	if !errors.Is(err, ErrNotFound) || err.Error() != "resolve: no catalog has this: MusicBrainz lists no studio album by AC/DC" {
		t.Errorf("err = %v", err)
	}
}

func TestNamesAreWrittenAsALibraryWritesThem(t *testing.T) {
	for in, want := range map[string]string{
		"[Led Zeppelin IV]":          "Led Zeppelin IV",
		"Guns N’ Roses":              "Guns N' Roses",
		"[untitled] and [more]":      "[untitled] and [more]",
		"“Heroes”":                   `"Heroes"`,
		" Appetite for Destruction ": "Appetite for Destruction",
	} {
		if got := libraryName(in); got != want {
			t.Errorf("libraryName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A name speech made of the band, kept by MusicBrainz as its alias, is the
// band - named as the band names itself, which is what a library has. The
// stand-in lists no albums for it; that it was found by its alias, and not
// taken for Leonard Bernstein, is what is checked.
func TestAnArtistHeardAsAnAliasIsTheArtist(t *testing.T) {
	o, _ := discographer(t)
	_, err := o.Discography(context.Background(), "Leonard Skinner")
	if !errors.Is(err, ErrNotFound) || err.Error() != "resolve: no catalog has this: MusicBrainz lists no studio album by Lynyrd Skynyrd" {
		t.Errorf("err = %v", err)
	}
}

// A tribute band with the name in its own is not the band, and an artist
// with no studio album has no first one.
func TestNoArtistByThatNameIsNotFound(t *testing.T) {
	o, _ := discographer(t)
	for _, artist := range []string{"Aerosmith", "Nobody", "  "} {
		if _, err := o.Discography(context.Background(), artist); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: err = %v", artist, err)
		}
	}
}
