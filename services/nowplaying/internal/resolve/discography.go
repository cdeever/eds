package resolve

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Discography is an artist's studio albums, oldest first, as MusicBrainz
// knows them (ADR-0041). It is what "Aerosmith's first album" is counted in.
type Discography struct {
	// Artist is the artist as MusicBrainz names them, which is the likelier
	// spelling for a library: "AC/DC" for a request heard as "acdc".
	Artist string
	Albums []StudioAlbum
}

// StudioAlbum is one album in a discography.
type StudioAlbum struct {
	Title    string
	Released string // its first release, YYYY[-MM[-DD]]
	// ID is MusicBrainz's release group: the album, whatever each release of
	// it has been called (Titles).
	ID string `json:",omitempty"`
}

// Nth is the nth studio album, counted from 1; a negative n counts from the
// newest, so -1 is the latest. ErrNotFound when there are not that many.
func (d Discography) Nth(n int) (StudioAlbum, error) {
	switch {
	case n > 0 && n <= len(d.Albums):
		return d.Albums[n-1], nil
	case n < 0 && -n <= len(d.Albums):
		return d.Albums[len(d.Albums)+n], nil
	}
	return StudioAlbum{}, fmt.Errorf("%w: %s has %d studio albums, not a %d", ErrNotFound, d.Artist, len(d.Albums), n)
}

// maxGroupPages bounds how much of a discography is read: MusicBrainz lists
// a hundred release groups a page, at one call a second, and the busiest
// rock band has a few hundred, most of them live albums and compilations.
const maxGroupPages = 5

// Discography finds an artist's studio albums: albums with no secondary
// type, so no live album, compilation, soundtrack, remix or demo, credited to
// the artist alone, in the order they were first released. An album
// MusicBrainz has no date for comes last. It is held once found, as a miss
// is.
//
// The artist must be MusicBrainz's by name, letters and digits alike: "Van
// Halen" is not "Edward Van Halen", and a tribute band is not the band.
func (o *Originals) Discography(ctx context.Context, artist string) (Discography, error) {
	key := simplify(artist)
	if key == "" {
		return Discography{}, fmt.Errorf("%w: no artist", ErrNotFound)
	}
	o.mu.Lock()
	d, cached := o.discographies[key]
	o.mu.Unlock()
	if cached {
		if d.Artist == "" {
			return Discography{}, ErrNotFound
		}
		return d, nil
	}

	id, name, err := o.artistID(ctx, artist)
	if err == nil {
		d, err = o.studioAlbums(ctx, id, name)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Discography{}, err // not asked; not remembered
	}
	o.mu.Lock()
	if o.discographies == nil {
		o.discographies = make(map[string]Discography)
	}
	o.discographies[key] = d // a miss is the zero Discography
	o.mu.Unlock()
	if err != nil {
		return Discography{}, err
	}
	return d, nil
}

// artistID finds an artist's MusicBrainz id: the best scored of those whose
// name is the one asked for.
func (o *Originals) artistID(ctx context.Context, artist string) (id, name string, err error) {
	var found struct {
		Artists []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Aliases []struct {
				Name string `json:"name"`
			} `json:"aliases"`
		} `json:"artists"`
	}
	// An alias too: "acdc", as speech is written down, is an alias of AC/DC
	// and not its name (checked 2026-10-24).
	term := fmt.Sprintf(`artist:"%s" OR alias:"%s"`, quoted(artist), quoted(artist))
	if err := o.askMB(ctx, o.MusicBrainz+"/ws/2/artist/?fmt=json&limit=10&query="+url.QueryEscape(term), &found); err != nil {
		return "", "", err
	}
	// In MusicBrainz's order, which is by score. The name heard may be an
	// alias: MusicBrainz keeps the misspellings people make, and speech
	// written down makes the same ones - "Leonard Skinner" is Lynyrd
	// Skynyrd's (2026-10-24). The name sent on is the artist's own.
	want := simplify(artist)
	for _, a := range found.Artists {
		if a.ID == "" {
			continue
		}
		named := simplify(a.Name) == want
		for _, alias := range a.Aliases {
			named = named || simplify(alias.Name) == want
		}
		if named {
			return a.ID, typographic.Replace(a.Name), nil
		}
	}
	return "", "", fmt.Errorf("%w: MusicBrainz knows no artist %q", ErrNotFound, artist)
}

// studioAlbums reads an artist's release groups, every page of them, since
// MusicBrainz lists them in no useful order.
func (o *Originals) studioAlbums(ctx context.Context, id, name string) (Discography, error) {
	d := Discography{Artist: name}
	for page, offset := 0, 0; page < maxGroupPages; page++ {
		var found struct {
			Count  int `json:"release-group-count"`
			Groups []struct {
				ID        string   `json:"id"`
				Title     string   `json:"title"`
				Primary   string   `json:"primary-type"`
				Secondary []string `json:"secondary-types"`
				First     string   `json:"first-release-date"`
				Credit    []struct {
					Name string `json:"name"`
				} `json:"artist-credit"`
			} `json:"release-groups"`
		}
		// website-default is MusicBrainz's own filter for release groups with
		// an official release: without it, a band's later years are bootlegs
		// ("Studio Sessions", undated, was the Beatles' latest album).
		endpoint := fmt.Sprintf("%s/ws/2/release-group?artist=%s&type=album&release-group-status=website-default&inc=artist-credits&fmt=json&limit=100&offset=%d",
			o.MusicBrainz, url.QueryEscape(id), offset)
		if err := o.askMB(ctx, endpoint, &found); err != nil {
			return Discography{}, err
		}
		for _, g := range found.Groups {
			names := make([]string, 0, len(g.Credit))
			for _, c := range g.Credit {
				names = append(names, c.Name)
			}
			// The artist alone: a collaboration is someone else's album too,
			// and nobody counts it among a band's own.
			if g.Primary != "Album" || len(g.Secondary) > 0 || simplify(strings.Join(names, " ")) != simplify(name) {
				continue
			}
			d.Albums = append(d.Albums, StudioAlbum{Title: libraryName(g.Title), Released: g.First, ID: g.ID})
		}
		offset += len(found.Groups)
		if len(found.Groups) == 0 || offset >= found.Count {
			break
		}
	}
	sort.SliceStable(d.Albums, func(i, j int) bool {
		a, b := d.Albums[i].Released, d.Albums[j].Released
		if a == "" || b == "" {
			return a != "" && b == ""
		}
		return a < b
	})
	if len(d.Albums) == 0 {
		return Discography{}, fmt.Errorf("%w: MusicBrainz lists no studio album by %s", ErrNotFound, name)
	}
	return d, nil
}

// askMB is one MusicBrainz request, kept to its one call a second, and once
// more after a pause when it answers 503, which is how it says slow down.
func (o *Originals) askMB(ctx context.Context, endpoint string, into any) error {
	for attempt := 0; ; attempt++ {
		if err := o.waitMB(ctx); err != nil {
			return err
		}
		err := getJSON(ctx, o.HTTP, o.UserAgent, endpoint, into)
		if err == nil || attempt > 0 || !strings.Contains(err.Error(), "503") {
			return err
		}
		o.mu.Lock()
		o.lastMB = time.Now().Add(time.Second)
		o.mu.Unlock()
	}
}

// typographic are the quotation marks MusicBrainz writes and a library
// mostly does not: "Guns N’ Roses" is "Guns N' Roses" in iTunes, and the
// player matches names character for character.
var typographic = strings.NewReplacer("’", "'", "‘", "'", "“", `"`, "”", `"`)

// libraryName is a MusicBrainz title as a library is likelier to have it:
// straight quotes, and an untitled album's name without the brackets
// MusicBrainz puts around it ("[Led Zeppelin IV]").
func libraryName(title string) string {
	title = typographic.Replace(strings.TrimSpace(title))
	if len(title) > 2 && strings.HasPrefix(title, "[") && strings.HasSuffix(title, "]") && strings.Count(title, "[") == 1 {
		title = strings.TrimSpace(title[1 : len(title)-1])
	}
	return title
}

// Titles are the other names an album's releases have gone by, most used
// first, as a library might have it instead (ADR-0045): Genesis' first,
// *From Genesis to Revelation*, is in one owner's library as its reissue,
// *And the Word Was…* (2026-10-24). The album's own title is not among them.
func (o *Originals) Titles(ctx context.Context, album StudioAlbum) ([]string, error) {
	if album.ID == "" {
		return nil, nil
	}
	var found struct {
		Releases []struct {
			Title string `json:"title"`
		} `json:"releases"`
	}
	endpoint := fmt.Sprintf("%s/ws/2/release?release-group=%s&fmt=json&limit=100", o.MusicBrainz, url.QueryEscape(album.ID))
	if err := o.askMB(ctx, endpoint, &found); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	var order []string
	for _, r := range found.Releases {
		t := libraryName(r.Title)
		if t == "" || simplify(t) == simplify(album.Title) {
			continue
		}
		if counts[t] == 0 {
			order = append(order, t)
		}
		counts[t]++
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	return order, nil
}
