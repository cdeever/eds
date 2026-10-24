package voice

import (
	"context"

	"github.com/cdeever/eds/services/nowplaying/internal/library"
)

// Owner is the owner's music, asked what a request needs of it: the artist
// nearest a name heard, and that artist's albums as the owner has them,
// spelled as the player will match them. It is eds-inventory (Inventory,
// ADR-0047), or the index of the library copied to voiced (ADR-0046).
type Owner interface {
	// FindArtist is the artist nearest the name heard, and how near, or nil
	// when none is near enough.
	FindArtist(ctx context.Context, heard string) (*OwnedArtist, float64, error)
	// FindAlbum is the title of the artist's album nearest the one named,
	// or "" when none is near enough.
	FindAlbum(ctx context.Context, a *OwnedArtist, title string) (string, error)
	// Owned is the title of the first of titles the artist has an album
	// for, never a compilation, or "".
	Owned(ctx context.Context, a *OwnedArtist, titles ...string) (string, error)
	// ByYear are the titles of the artist's own albums, earliest first,
	// without compilations or what is named as one.
	ByYear(ctx context.Context, a *OwnedArtist) ([]string, error)
}

// OwnedArtist is an artist the owner has albums by.
type OwnedArtist struct {
	// ID is what the Owner knows the artist by.
	ID   string
	Name string
}

// Library is where the Owner is found: nil, and no error, when there is
// none yet, and requests are guessed at as they were (ADR-0045).
type Library interface {
	Owner() (Owner, error)
}

// IndexFile is the library index copied to voiced (ADR-0046), as a Library.
type IndexFile struct {
	File *library.File
}

// Owner is the index as it is now, or nil when there is none.
func (f IndexFile) Owner() (Owner, error) {
	ix, err := f.File.Index()
	if ix == nil {
		return nil, err
	}
	return IndexOwner(ix), err
}

// IndexOwner is an index as an Owner.
func IndexOwner(ix *library.Index) Owner { return indexOwner{ix} }

type indexOwner struct{ ix *library.Index }

func (o indexOwner) FindArtist(_ context.Context, heard string) (*OwnedArtist, float64, error) {
	a, score := o.ix.FindArtist(heard)
	if a == nil {
		return nil, score, nil
	}
	return &OwnedArtist{ID: a.Name, Name: a.Name}, score, nil
}

func (o indexOwner) artist(a *OwnedArtist) *library.Artist {
	for i := range o.ix.Artists {
		if o.ix.Artists[i].Name == a.ID {
			return &o.ix.Artists[i]
		}
	}
	return &library.Artist{}
}

func (o indexOwner) FindAlbum(_ context.Context, a *OwnedArtist, title string) (string, error) {
	if al, _ := o.artist(a).FindAlbum(title); al != nil {
		return al.Title, nil
	}
	return "", nil
}

func (o indexOwner) Owned(_ context.Context, a *OwnedArtist, titles ...string) (string, error) {
	if al := o.artist(a).Owned(titles...); al != nil {
		return al.Title, nil
	}
	return "", nil
}

func (o indexOwner) ByYear(_ context.Context, a *OwnedArtist) ([]string, error) {
	var out []string
	for _, al := range o.artist(a).ByYear() {
		out = append(out, al.Title)
	}
	return out, nil
}
