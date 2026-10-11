package looks

import (
	"testing"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

func TestDarkSideOfTheMoonHasTheSpectrum(t *testing.T) {
	for _, track := range []contract.Track{
		{Artist: "Pink Floyd", Album: "The Dark Side of the Moon", Title: "Time"},
		{Artist: "Pink Floyd", Album: "Dark Side Of The Moon", Title: "Money"},
		{Artist: "Pink Floyd", Album: "The Dark Side of the Moon (2011 Remastered Version)", Title: "Us and Them"},
		{Artist: "Pink Floyd", Album: "The Dark Side Of The Moon [50th Anniversary]", Title: "Breathe (In the Air)"},
		{Artist: "Pink Floyd", Album: "The Dark Side of the Moon - 50th Anniversary", Title: "Eclipse"},
		{Artist: "PINK FLOYD", Album: "the dark side of the moon", Title: "On the Run"},
		// As iTunes tags a compilation-style rip: the album artist is the band.
		{Artist: "Pink Floyd, Clare Torry", AlbumArtist: "Pink Floyd", Album: "The Dark Side of the Moon", Title: "The Great Gig in the Sky"},
		{Artist: "Clare Torry", AlbumArtist: "Pink Floyd", Album: "The Dark Side of the Moon", Title: "The Great Gig in the Sky"},
	} {
		if got := For(track); got != Spectrum {
			t.Errorf("%q by %q / %q: look %q", track.Album, track.Artist, track.AlbumArtist, got)
		}
	}
}

func TestNearlyEverythingHasNoLook(t *testing.T) {
	for _, track := range []contract.Track{
		{},
		{Artist: "Pink Floyd", Title: "Money"}, // no album: not on one, as far as this knows
		{Artist: "Pink Floyd", Album: "Wish You Were Here", Title: "Have a Cigar"},
		{Artist: "Pink Floyd", Album: "The Wall", Title: "Mother"},
		// The title on someone else's record is someone else's record.
		{Artist: "The Flaming Lips", Album: "The Dark Side of the Moon", Title: "Money"},
		{Artist: "Easy Star All-Stars", Album: "Dub Side of the Moon", Title: "Time"},
		// An album that merely mentions it.
		{Artist: "Pink Floyd", Album: "A Tribute to The Dark Side of the Moon", Title: "Time"},
		{Artist: "The J. Geils Band", Album: "Bloodshot", Title: "House Party"},
	} {
		if got := For(track); got != "" {
			t.Errorf("%q by %q: look %q, want none", track.Album, track.Artist, got)
		}
	}
}
