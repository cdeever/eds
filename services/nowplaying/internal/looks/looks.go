// Package looks knows which albums have a look of their own (ADR-0019).
//
// Almost every record is lit from its cover. A few are known by a look as
// well as by a name, and for those the room should be lit by that, not by
// whatever colours a fair reading of the sleeve finds. This is the list.
//
// It names a look and nothing else. What a look is - which colours, which
// motion - is lightd's, which knows nothing about albums; this package knows
// nothing about colours. The name is all that crosses.
//
// The list is short on purpose. It is for records whose look is as well
// known as their title.
package looks

import (
	"strings"
	"unicode"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// Spectrum is the colours of the rainbow, red to violet.
const Spectrum = "spectrum"

// A rule gives a look to an album by an artist. Both are written as a person
// would write them; they are compared as plain letters.
type rule struct {
	artist, album, look string
}

var rules = []rule{
	// A prism, a beam of white light, and the spectrum it becomes.
	{artist: "Pink Floyd", album: "The Dark Side of the Moon", look: Spectrum},
}

// For returns the look of the album a track is on, or "" when it has none,
// which is nearly always.
//
// The album has to begin with the title, and the artist has to contain the
// name. "Dark Side Of The Moon (2011 Remaster)" and "The Dark Side of the
// Moon - 50th Anniversary" are both that album; a track with no album tag is
// not on any album as far as this can tell.
func For(t contract.Track) string {
	album := plain(t.Album)
	if album == "" {
		return ""
	}
	artists := plain(t.AlbumArtist) + "\x00" + plain(t.Artist)
	for _, r := range rules {
		if strings.HasPrefix(album, plain(r.album)) && strings.Contains(artists, plain(r.artist)) {
			return r.look
		}
	}
	return ""
}

// plain reduces a name to its letters and digits, lower case, without
// anything in brackets and without a leading "the": what is left when
// taggers' habits are taken away.
func plain(s string) string {
	var out strings.Builder
	depth := 0
	for _, r := range strings.ToLower(s) {
		switch r {
		case '(', '[':
			depth++
			continue
		case ')', ']':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			out.WriteRune(r)
		}
	}
	return strings.TrimPrefix(out.String(), "the")
}
