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

func TestAmericanIdiotHasTheGrenade(t *testing.T) {
	for _, track := range []contract.Track{
		{Artist: "Green Day", Album: "American Idiot", Title: "Jesus of Suburbia"},
		{Artist: "Green Day", Album: "American Idiot (20th Anniversary Deluxe Edition)", Title: "Holiday"},
		{Artist: "GREEN DAY", Album: "american idiot", Title: "St. Jimmy"},
	} {
		if got := For(track); got != Grenade {
			t.Errorf("%q by %q: look %q", track.Album, track.Artist, got)
		}
	}
	// The song on another record is that record's.
	for _, track := range []contract.Track{
		{Artist: "Green Day", Album: "Greatest Hits: God's Favorite Band", Title: "American Idiot"},
		{Artist: "Green Day", Album: "Dookie", Title: "Basket Case"},
	} {
		if got := For(track); got != "" {
			t.Errorf("%q by %q: look %q, want none", track.Album, track.Artist, got)
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

// ADR-0030: a piece of music can have a look, whoever plays it.
func TestTheKnightRiderThemeHasTheScanner(t *testing.T) {
	for _, track := range []contract.Track{
		{Artist: "Stu Phillips", Title: "Knight Rider"},
		{Artist: "Stu Phillips", Title: "Knight Rider Theme"},
		{Artist: "Stu Phillips & Glen A. Larson", Album: "Knight Rider (Original Television Soundtrack)", Title: "Knight Rider (Main Title)"},
		{Artist: "TV Themes", Album: "Television's Greatest Hits", Title: "Theme from Knight Rider"},
		{Artist: "Stu Phillips", Title: "Knight Rider - Main Theme"},
		{Artist: "Hollywood Studio Orchestra", Title: "KNIGHT RIDER: TV THEME"},
		{Artist: "Stu Phillips", Title: "Knight Rider Theme (From \"Knight Rider\") [Remastered]"},
		// On a record that has a look of its own, the piece's comes first.
		{Artist: "Pink Floyd", Album: "The Dark Side of the Moon", Title: "Knight Rider Theme"},
	} {
		if got := For(track); got != Scanner {
			t.Errorf("%q by %q: look %q", track.Title, track.Artist, got)
		}
	}
	if !AnySource(Scanner) {
		t.Error("the scanner is not shown from any source")
	}
	if AnySource(Spectrum) || AnySource("") {
		t.Error("a look that is an album's is shown from any source")
	}
}

func TestWhatOnlyMentionsKnightRiderHasNoLook(t *testing.T) {
	for _, track := range []contract.Track{
		{Artist: "Stu Phillips", Album: "Knight Rider", Title: "KITT Jumps"}, // the rest of the soundtrack
		{Artist: "Someone", Title: "Knight Rider's Lament"},
		{Artist: "Someone", Title: "My Knight Rider"},
		{Artist: "Someone", Title: "Knight"},
		{Artist: "Someone", Title: "Theme"},
		{Artist: "Someone", Title: "(Knight Rider)"},
	} {
		if got := For(track); got != "" {
			t.Errorf("%q: look %q, want none", track.Title, got)
		}
	}
}
