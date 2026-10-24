package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cdeever/eds/services/nowplaying/internal/library"
)

// inventory is an Inventory asking a stand-in for eds-inventory that
// answers contract version 1 from an index: its four routes, its nulls, its
// errors. eds-inventory's own matching is the index's, carried over, so the
// answers are the ones it gives.
func inventory(t *testing.T, ix *library.Index) *Inventory {
	t.Helper()
	srv := httptest.NewServer(fakeInventory(ix, 1))
	t.Cleanup(srv.Close)
	return &Inventory{URL: srv.URL, HTTP: srv.Client()}
}

func fakeInventory(ix *library.Index, version int) http.Handler {
	byID := func(r *http.Request) *library.Artist {
		for i := range ix.Artists {
			if "ar_"+ix.Artists[i].Name == r.PathValue("id") {
				return &ix.Artists[i]
			}
		}
		return nil
	}
	answer := func(w http.ResponseWriter, status int, v map[string]any) {
		v["v"] = version
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	album := func(a *library.Album) any {
		if a == nil {
			return nil
		}
		return map[string]any{"id": "al_" + a.Title, "title": a.Title, "tracks": a.Tracks, "sources": []string{"itunes"}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/artists/find", func(w http.ResponseWriter, r *http.Request) {
		a, score := ix.FindArtist(r.URL.Query().Get("name"))
		var artist any
		if a != nil {
			artist = map[string]any{"id": "ar_" + a.Name, "name": a.Name, "albums": []any{}}
		}
		answer(w, 200, map[string]any{"name": r.URL.Query().Get("name"), "score": score, "artist": artist})
	})
	mux.HandleFunc("GET /v1/artists/{id}/albums/find", func(w http.ResponseWriter, r *http.Request) {
		a := byID(r)
		if a == nil {
			answer(w, 404, map[string]any{"error": "no artist with that id"})
			return
		}
		titles := r.URL.Query()["title"]
		if len(titles) == 1 && r.URL.Query().Get("studio") != "true" {
			al, score := a.FindAlbum(titles[0])
			answer(w, 200, map[string]any{"title": titles[0], "score": score, "album": album(al)})
			return
		}
		answer(w, 200, map[string]any{"title": nil, "album": album(a.Owned(titles...))})
	})
	mux.HandleFunc("GET /v1/artists/{id}/albums/by-year", func(w http.ResponseWriter, r *http.Request) {
		a := byID(r)
		if a == nil {
			answer(w, 404, map[string]any{"error": "no artist with that id"})
			return
		}
		albums := []any{}
		for _, al := range a.ByYear() {
			albums = append(albums, album(&al))
		}
		answer(w, 200, map[string]any{"albums": albums})
	})
	return mux
}

// Not near enough is null in the contract, and nil here: never the nearest.
func TestTheInventorysNullIsNoArtist(t *testing.T) {
	a, score, err := inventory(t, owned()).FindArtist(context.Background(), "Ruth")
	if a != nil || err != nil || score == 0 {
		t.Errorf("%+v %.2f %v", a, score, err)
	}
}

// An answer in a version voiced does not speak is refused, not guessed at.
func TestAnotherVersionOfTheInventoryIsRefused(t *testing.T) {
	srv := httptest.NewServer(fakeInventory(owned(), 2))
	defer srv.Close()
	_, _, err := (&Inventory{URL: srv.URL}).FindArtist(context.Background(), "Genesis")
	if err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("%v", err)
	}
}

func TestAnInventoryThatCannotBeReachedIsAnErrorNotNotOwned(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, _, _, err := Owned(context.Background(), catalog{}, &Inventory{URL: srv.URL}, Request{Artist: "Genesis", Nth: 1})
	if err == nil || errors.Is(err, ErrNotOwned) {
		t.Errorf("%v", err)
	}
}

// A real eds-inventory, when NP_TEST_INVENTORY names one with the owner's
// library collected: the stand-in above is only as honest as this.
func TestARealInventory(t *testing.T) {
	url := os.Getenv("NP_TEST_INVENTORY")
	if url == "" {
		t.Skip("NP_TEST_INVENTORY names no eds-inventory")
	}
	inv := &Inventory{URL: url}
	ctx := context.Background()
	a, score, err := inv.FindArtist(ctx, "Arrowsmith")
	if err != nil || a == nil || a.Name != "Aerosmith" {
		t.Fatalf("%+v %.2f %v", a, score, err)
	}
	if titles, err := inv.ByYear(ctx, a); err != nil || len(titles) == 0 {
		t.Errorf("by year: %v %v", titles, err)
	}
	if title, err := inv.FindAlbum(ctx, a, "Toys in the Attic"); err != nil || title == "" {
		t.Errorf("Toys in the Attic: %q %v", title, err)
	}
	if title, err := inv.Owned(ctx, a, "Not An Album Anyone Made", "Aerosmith"); err != nil || title == "" {
		t.Errorf("owned: %q %v", title, err)
	}
	if a, _, err := inv.FindArtist(ctx, "Ruth"); err != nil || a != nil {
		t.Errorf("Ruth: %+v %v", a, err)
	}
}
