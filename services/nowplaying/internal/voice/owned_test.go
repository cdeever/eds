package voice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/cdeever/eds/services/nowplaying/internal/library"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// catalog is MusicBrainz as ADR-0041 and ADR-0045 ask it, cut down to the
// albums that went wrong on 2026-10-24.
type catalog struct{}

func (catalog) Discography(_ context.Context, artist string) (resolve.Discography, error) {
	switch resolve.Simplify(artist) {
	case "genesis":
		return resolve.Discography{Artist: "Genesis", Albums: []resolve.StudioAlbum{
			{Title: "From Genesis to Revelation", ID: "g1"}, {Title: "Trespass", ID: "g2"}, {Title: "Nursery Cryme", ID: "g3"}}}, nil
	case "lynyrdskynyrd", "leonardskinner": // the second, an alias
		return resolve.Discography{Artist: "Lynyrd Skynyrd", Albums: []resolve.StudioAlbum{
			{Title: "(pronounced 'lĕh-'nérd 'skin-'nérd)", ID: "s1"}, {Title: "Second Helping", ID: "s2"}}}, nil
	case "vanhalen":
		return resolve.Discography{Artist: "Van Halen", Albums: []resolve.StudioAlbum{
			{Title: "Van Halen", ID: "vh1"}, {Title: "Van Halen II", ID: "vh2"}, {Title: "Women and Children First", ID: "vh3"}}}, nil
	}
	return resolve.Discography{}, fmt.Errorf("%w: %q", resolve.ErrNotFound, artist)
}

func (catalog) Titles(_ context.Context, a resolve.StudioAlbum) ([]string, error) {
	if a.ID == "g1" {
		return []string{"And the Word Was…", "In the Beginning"}, nil
	}
	return nil, nil
}

func owned() *library.Index {
	return &library.Index{V: library.Version, Artists: []library.Artist{
		{Name: "Genesis", Albums: []library.Album{
			{Title: "And The Word Was", Year: 1976, Tracks: 20}, {Title: "Trespass", Year: 1974, Tracks: 6},
			{Title: "Turn It On Again: The Hits", Year: 1999, Tracks: 18, Compilation: true}}},
		{Name: "Lynyrd Skynyrd", Albums: []library.Album{
			{Title: "Pronounced leh-nerd skin-nerd", Year: 1973, Tracks: 8}, {Title: "Second Helping", Year: 1974, Tracks: 8}}},
		{Name: "Van Halen", Albums: []library.Album{
			{Title: "Van Halen II", Year: 1979, Tracks: 10}, {Title: "Van Halen", Year: 1978, Tracks: 11},
			{Title: "Women And Children First", Year: 1980, Tracks: 9}}},
		{Name: "Aerosmith", Albums: []library.Album{
			{Title: "Toys In The Attic", Year: 1975, Tracks: 9}, {Title: "Aerosmith", Year: 1973, Tracks: 8},
			{Title: "Greatest Hits", Year: 1980, Tracks: 10}}},
		{Name: "The Beatles", Albums: []library.Album{{Title: "Abbey Road", Year: 1969, Tracks: 17}}},
	}}
}

func TestTheAlbumIsTheLibrarysOwn(t *testing.T) {
	for _, c := range []struct {
		r             Request
		artist, album string
		instead       []string
	}{
		// The library has the reissue, under its own name.
		{Request{Artist: "Genesis", Nth: 1}, "Genesis", "And The Word Was", []string{"Trespass"}},
		// Heard as an alias MusicBrainz keeps; spelled as the library does.
		{Request{Artist: "Leonard Skinner", Nth: 1}, "Lynyrd Skynyrd", "Pronounced leh-nerd skin-nerd", []string{"Second Helping"}},
		{Request{Artist: "Van Halen", Nth: 1}, "Van Halen", "Van Halen", []string{"Van Halen II", "Women And Children First"}},
		{Request{Artist: "Van Halen", Nth: -1}, "Van Halen", "Women And Children First", []string{"Van Halen II", "Van Halen"}},
		// Misheard, and unknown to the catalog: the library by year, with no
		// greatest hits.
		{Request{Artist: "Arrowsmith", Nth: 1}, "Aerosmith", "Aerosmith", []string{"Toys In The Attic"}},
		{Request{Artist: "the Beatles", Album: "Abby Road"}, "The Beatles", "Abbey Road", nil},
	} {
		artist, album, instead, err := Owned(context.Background(), catalog{}, owned(), c.r)
		if err != nil || artist != c.artist || album != c.album || !slices.Equal(instead, c.instead) {
			t.Errorf("%+v: %q %q %q, %v; want %q %q %q", c.r, artist, album, instead, err, c.artist, c.album, c.instead)
		}
	}
}

func TestWhatTheLibraryLacksIsNotOwned(t *testing.T) {
	for _, r := range []Request{
		{Artist: "Ruth", Nth: 1},
		{Artist: "The Beatles", Album: "Revolver"},
		{Artist: "Genesis", Nth: 4}, // the catalog has three
	} {
		if _, album, _, err := Owned(context.Background(), catalog{}, owned(), r); !errors.Is(err, ErrNotOwned) {
			t.Errorf("%+v: %q, %v; want ErrNotOwned", r, album, err)
		}
	}
}

type noIndex struct{}

func (noIndex) Index() (*library.Index, error) { return nil, nil }

func TestWithoutAnIndexAlbumsAreGuessedAsBefore(t *testing.T) {
	_, album, _, err := Choose(context.Background(), catalog{}, noIndex{}, Request{Artist: "Genesis", Nth: 2})
	if err != nil || album != "Trespass" {
		t.Errorf("%q, %v; want the catalog's Trespass", album, err)
	}
}
