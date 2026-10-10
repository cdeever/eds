package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The three catalogues, in the order ADR-0008 tries them. Each needs no key
// and no account. Base URLs are fields so the tests can stand in for them.

// quoted makes a value safe inside a quoted search term. The catalogues'
// query languages all treat a double quote as syntax.
func quoted(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, `"`, " "))
}

// --- Deezer ------------------------------------------------------------------

// Deezer answers album and track searches without a key and offers a 1000 px
// cover. It is first because it is fast and its covers are large.
type Deezer struct {
	Base      string // https://api.deezer.com
	HTTP      *http.Client
	UserAgent string
}

func (Deezer) Name() string { return "deezer" }

func (d Deezer) Lookup(ctx context.Context, q Query) (*Hit, error) {
	if q.Album != "" {
		var found struct {
			Data []struct {
				Title   string `json:"title"`
				CoverXL string `json:"cover_xl"`
				Cover   string `json:"cover_big"`
				Artist  struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"data"`
		}
		term := fmt.Sprintf(`artist:"%s" album:"%s"`, quoted(q.Artist), quoted(q.Album))
		if err := getJSON(ctx, d.HTTP, d.UserAgent, d.Base+"/search/album?limit=5&q="+url.QueryEscape(term), &found); err != nil {
			return nil, err
		}
		for _, a := range found.Data {
			hit := &Hit{Artist: a.Artist.Name, Album: a.Title, ImageURL: firstNonEmpty(a.CoverXL, a.Cover)}
			if hit.ImageURL != "" && matches(q, hit) {
				return hit, nil
			}
		}
		return nil, ErrNotFound
	}

	var found struct {
		Data []struct {
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				Title   string `json:"title"`
				CoverXL string `json:"cover_xl"`
				Cover   string `json:"cover_big"`
			} `json:"album"`
		} `json:"data"`
	}
	// Plain text, not the artist: and track: fields the album search uses.
	// For tracks the fields quietly find nothing, or something unrelated,
	// while "artist title" as free text returns the right track first.
	// Checked against the live service on 2026-10-10. Free text also returns
	// near misses, which is why every result is checked before it is taken.
	term := quoted(q.Artist) + " " + quoted(q.Title)
	if err := getJSON(ctx, d.HTTP, d.UserAgent, d.Base+"/search/track?limit=5&q="+url.QueryEscape(term), &found); err != nil {
		return nil, err
	}
	for _, t := range found.Data {
		hit := &Hit{Artist: t.Artist.Name, Album: t.Album.Title, Title: t.Title, ImageURL: firstNonEmpty(t.Album.CoverXL, t.Album.Cover)}
		if hit.ImageURL != "" && matches(q, hit) {
			return hit, nil
		}
	}
	return nil, ErrNotFound
}

// --- iTunes Search -------------------------------------------------------------

// ITunes is Apple's search API: no key, about twenty calls a minute. Its
// artwork URL names a 100 px image, and the same path serves larger ones.
type ITunes struct {
	Base      string // https://itunes.apple.com
	HTTP      *http.Client
	UserAgent string
}

func (ITunes) Name() string { return "itunes" }

func (i ITunes) Lookup(ctx context.Context, q Query) (*Hit, error) {
	entity, term := "album", q.Artist+" "+q.Album
	if q.Album == "" {
		entity, term = "song", q.Artist+" "+q.Title
	}

	var found struct {
		Results []struct {
			ArtistName     string `json:"artistName"`
			CollectionName string `json:"collectionName"`
			TrackName      string `json:"trackName"`
			ArtworkURL100  string `json:"artworkUrl100"`
		} `json:"results"`
	}
	endpoint := fmt.Sprintf("%s/search?media=music&entity=%s&limit=5&term=%s", i.Base, entity, url.QueryEscape(term))
	if err := getJSON(ctx, i.HTTP, i.UserAgent, endpoint, &found); err != nil {
		return nil, err
	}
	for _, r := range found.Results {
		hit := &Hit{Artist: r.ArtistName, Album: r.CollectionName, Title: r.TrackName, ImageURL: larger(r.ArtworkURL100)}
		if hit.ImageURL != "" && matches(q, hit) {
			return hit, nil
		}
	}
	return nil, ErrNotFound
}

// larger asks for the 600 px rendition of an artwork URL that names the
// 100 px one. The size is part of the file name; this is how every client of
// the API gets a usable cover, though Apple does not document it.
func larger(artworkURL string) string {
	return strings.Replace(artworkURL, "100x100bb", "600x600bb", 1)
}

// --- MusicBrainz and the Cover Art Archive -------------------------------------

// MusicBrainz is last: it allows one request a second and needs a redirect to
// reach the image, but it is the one most likely to know an obscure or
// physical release. It answers album questions only.
type MusicBrainz struct {
	Base      string // https://musicbrainz.org
	CoverBase string // https://coverartarchive.org
	HTTP      *http.Client
	UserAgent string
}

func (MusicBrainz) Name() string { return "musicbrainz" }

func (m MusicBrainz) Lookup(ctx context.Context, q Query) (*Hit, error) {
	if q.Album == "" {
		return nil, ErrNotFound
	}

	var found struct {
		Groups []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Credit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
		} `json:"release-groups"`
	}
	term := fmt.Sprintf(`artist:"%s" AND releasegroup:"%s"`, quoted(q.Artist), quoted(q.Album))
	endpoint := m.Base + "/ws/2/release-group/?fmt=json&limit=5&query=" + url.QueryEscape(term)
	if err := getJSON(ctx, m.HTTP, m.UserAgent, endpoint, &found); err != nil {
		return nil, err
	}
	for _, g := range found.Groups {
		names := make([]string, 0, len(g.Credit))
		for _, c := range g.Credit {
			names = append(names, c.Name)
		}
		hit := &Hit{
			Artist:   strings.Join(names, " "),
			Album:    g.Title,
			ImageURL: m.CoverBase + "/release-group/" + url.PathEscape(g.ID) + "/front-500",
		}
		if g.ID != "" && matches(q, hit) {
			return hit, nil
		}
	}
	return nil, ErrNotFound
}

func getJSON(ctx context.Context, client *http.Client, userAgent, endpoint string, into any) error {
	body, err := get(ctx, client, userAgent, endpoint, 1<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("unreadable answer: %w", err)
	}
	return nil
}

// Named builds the catalogues an operator asked for by name, in that order.
func Named(names []string, client *http.Client, userAgent string) ([]Catalogue, error) {
	var out []Catalogue
	for _, name := range names {
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "":
		case "deezer":
			out = append(out, Deezer{Base: "https://api.deezer.com", HTTP: client, UserAgent: userAgent})
		case "itunes":
			out = append(out, ITunes{Base: "https://itunes.apple.com", HTTP: client, UserAgent: userAgent})
		case "musicbrainz":
			out = append(out, MusicBrainz{Base: "https://musicbrainz.org", CoverBase: "https://coverartarchive.org", HTTP: client, UserAgent: userAgent})
		default:
			return nil, fmt.Errorf("resolve: unknown catalogue %q (known: deezer, itunes, musicbrainz)", name)
		}
	}
	return out, nil
}
