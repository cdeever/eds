package voice

import (
	"context"
	"errors"
	"fmt"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/library"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// Library is the owner's library as an index, or nil when there is none
// (ADR-0046).
type Library interface {
	Index() (*library.Index, error)
}

// ErrNotOwned means the library has nothing that answers the request: no
// artist like the one heard, or no album like the one named.
var ErrNotOwned = errors.New("not in the library")

// Choose names the album a request asks for. With the owner's library known
// it is chosen from the library, by the library's own names (Owned); without
// one, as Album guesses it.
func Choose(ctx context.Context, albums Counter, lib Library, r Request) (artist, album string, instead []string, err error) {
	if lib != nil {
		if ix, _ := lib.Index(); ix != nil {
			return Owned(ctx, albums, ix, r)
		}
	}
	return Album(ctx, albums, r)
}

// Owned chooses the album from the owner's library (ADR-0046): the artist
// nearest the one heard, then the album nearest the one named, or the first
// of the artist's albums in the catalog's order that the library has. What it
// names is the library's, character for character, so the player is asked
// for what it has.
func Owned(ctx context.Context, albums Counter, ix *library.Index, r Request) (artist, album string, instead []string, err error) {
	owned, counted := ownedArtist(ctx, albums, ix, r.Artist)
	if owned == nil {
		return "", "", nil, fmt.Errorf("%w: nothing by %s", ErrNotOwned, r.Artist)
	}

	if r.Album != "" {
		a, _ := owned.FindAlbum(r.Album)
		if a == nil {
			return "", "", nil, fmt.Errorf("%w: no %s by %s", ErrNotOwned, r.Album, owned.Name)
		}
		return owned.Name, a.Title, nil, nil
	}

	// The catalog's order, as the library has it. The album asked for may be
	// in the library under one of its other names (a reissue's); those are
	// asked for only if its own is not there.
	var picked []string
	if counted.Artist == "" {
		counted, _ = albums.Discography(ctx, owned.Name)
	}
	for _, mb := range inOrder(counted, r.Nth) {
		a := owned.Owned(mb.Title)
		if a == nil && len(picked) == 0 {
			a = owned.Owned(allTitles(ctx, albums, mb)...)
		}
		if a != nil && !contains(picked, a.Title) {
			picked = append(picked, a.Title)
		}
		if len(picked) > contract.MaxInstead {
			break
		}
	}
	// No catalog that knows the artist, or none of its albums owned: the
	// library's own, by year.
	if len(picked) == 0 {
		for _, a := range inOrderByYear(owned.ByYear(), r.Nth) {
			picked = append(picked, a.Title)
			if len(picked) > contract.MaxInstead {
				break
			}
		}
	}
	if len(picked) == 0 {
		return "", "", nil, fmt.Errorf("%w: no album by %s to count", ErrNotOwned, owned.Name)
	}
	return owned.Name, picked[0], picked[1:], nil
}

// ownedArtist is the library's artist for the name heard: the nearest by
// spelling and sound, or else the catalog's artist the name is an alias of
// ("Leonard Skinner" is Lynyrd Skynyrd's). When the catalog was asked, its
// count is handed back too.
func ownedArtist(ctx context.Context, albums Counter, ix *library.Index, heard string) (*library.Artist, resolve.Discography) {
	if a, score := ix.FindArtist(heard); a != nil && score == 1 {
		return a, resolve.Discography{}
	}
	d, err := albums.Discography(ctx, heard)
	if err == nil {
		if a, _ := ix.FindArtist(d.Artist); a != nil {
			return a, d
		}
	}
	a, _ := ix.FindArtist(heard)
	return a, resolve.Discography{}
}

// inOrder are the counted albums from the Nth on: later ones for the first or
// the Nth, earlier ones for the latest.
func inOrder(d resolve.Discography, nth int) []resolve.StudioAlbum {
	var out []resolve.StudioAlbum
	switch {
	case nth > 0 && nth <= len(d.Albums):
		out = append(out, d.Albums[nth-1:]...)
	case nth < 0 && -nth <= len(d.Albums):
		for i := len(d.Albums) + nth; i >= 0; i-- {
			out = append(out, d.Albums[i])
		}
	}
	return out
}

func inOrderByYear(albums []library.Album, nth int) []library.Album {
	var out []library.Album
	switch {
	case nth > 0 && nth <= len(albums):
		out = append(out, albums[nth-1:]...)
	case nth < 0 && -nth <= len(albums):
		for i := len(albums) + nth; i >= 0; i-- {
			out = append(out, albums[i])
		}
	}
	return out
}

// allTitles are an album's own title and the names its releases have gone
// by, as the catalog gives them.
func allTitles(ctx context.Context, albums Counter, a resolve.StudioAlbum) []string {
	titles := []string{a.Title}
	if t, ok := albums.(Titler); ok {
		if more, err := t.Titles(ctx, a); err == nil {
			titles = append(titles, more...)
		}
	}
	return titles
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
