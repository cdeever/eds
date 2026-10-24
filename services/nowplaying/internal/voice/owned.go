package voice

import (
	"context"
	"errors"
	"fmt"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// ErrNotOwned means the library has nothing that answers the request: no
// artist like the one heard, or no album like the one named.
var ErrNotOwned = errors.New("not in the library")

// Choose names the album a request asks for. With the owner's music to ask
// it is chosen from what the owner has, by the owner's own names (Owned);
// without, as Album guesses it.
func Choose(ctx context.Context, albums Counter, owner Owner, r Request) (artist, album string, instead []string, err error) {
	if owner != nil {
		return Owned(ctx, albums, owner, r)
	}
	return Album(ctx, albums, r)
}

// Owned chooses the album from what the owner has (ADR-0046, ADR-0047): the
// artist nearest the one heard, then the album nearest the one named, or the
// first of the artist's albums in the catalog's order that the owner has.
// What it names is the owner's, character for character, so the player is
// asked for what it has. An error that is not ErrNotOwned is the owner not
// answering.
func Owned(ctx context.Context, albums Counter, owner Owner, r Request) (artist, album string, instead []string, err error) {
	owned, counted, err := ownedArtist(ctx, albums, owner, r.Artist)
	if err != nil {
		return "", "", nil, err
	}
	if owned == nil {
		return "", "", nil, fmt.Errorf("%w: nothing by %s", ErrNotOwned, r.Artist)
	}

	if r.Album != "" {
		title, err := owner.FindAlbum(ctx, owned, r.Album)
		if err != nil {
			return "", "", nil, err
		}
		if title == "" {
			return "", "", nil, fmt.Errorf("%w: no %s by %s", ErrNotOwned, r.Album, owned.Name)
		}
		return owned.Name, title, nil, nil
	}

	// The catalog's order, as the owner has it. The album asked for may be
	// held under one of its other names (a reissue's); those are asked for
	// only if its own is not there.
	var picked []string
	if counted.Artist == "" {
		counted, _ = albums.Discography(ctx, owned.Name)
	}
	for _, mb := range inOrder(counted, r.Nth) {
		title, err := owner.Owned(ctx, owned, mb.Title)
		if err != nil {
			return "", "", nil, err
		}
		if title == "" && len(picked) == 0 {
			if title, err = owner.Owned(ctx, owned, allTitles(ctx, albums, mb)...); err != nil {
				return "", "", nil, err
			}
		}
		if title != "" && !contains(picked, title) {
			picked = append(picked, title)
		}
		if len(picked) > contract.MaxInstead {
			break
		}
	}
	// No catalog that knows the artist, or none of its albums owned: the
	// owner's own, by year.
	if len(picked) == 0 {
		byYear, err := owner.ByYear(ctx, owned)
		if err != nil {
			return "", "", nil, err
		}
		for _, title := range inOrderByYear(byYear, r.Nth) {
			picked = append(picked, title)
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

// ownedArtist is the owner's artist for the name heard: the nearest by
// spelling and sound, or else the catalog's artist the name is an alias of
// ("Leonard Skinner" is Lynyrd Skynyrd's). When the catalog was asked, its
// count is handed back too.
func ownedArtist(ctx context.Context, albums Counter, owner Owner, heard string) (*OwnedArtist, resolve.Discography, error) {
	a, score, err := owner.FindArtist(ctx, heard)
	if err != nil {
		return nil, resolve.Discography{}, err
	}
	if a != nil && score == 1 {
		return a, resolve.Discography{}, nil
	}
	if d, err := albums.Discography(ctx, heard); err == nil {
		alias, _, err := owner.FindArtist(ctx, d.Artist)
		if err != nil {
			return nil, resolve.Discography{}, err
		}
		if alias != nil {
			return alias, d, nil
		}
	}
	return a, resolve.Discography{}, nil
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

func inOrderByYear(titles []string, nth int) []string {
	var out []string
	switch {
	case nth > 0 && nth <= len(titles):
		out = append(out, titles[nth-1:]...)
	case nth < 0 && -nth <= len(titles):
		for i := len(titles) + nth; i >= 0; i-- {
			out = append(out, titles[i])
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
