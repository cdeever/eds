// Package looks knows which albums have a look of their own (ADR-0019).
//
// Almost every record is lit from its cover. A few are known by a look as
// well as by a name, and for those the room should be lit by that, not by
// whatever colors a fair reading of the sleeve finds. This is the list.
//
// It names a look and nothing else. What a look is - which colors, which
// motion - is lightd's, which knows nothing about albums; this package knows
// nothing about colors. The name is all that crosses.
//
// The list is short on purpose. It is for records whose look is as well
// known as their title.
package looks

import (
	"strings"
	"unicode"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// Spectrum is the colors of the rainbow, red to violet.
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

// Scanner is one red point going from end to end and back.
const Scanner = "scanner"

// A piece gives a look to one piece of music, whoever plays it and whatever
// record it is on: a theme, known by its title alone. The title is written
// as its bare words; a track's title is that piece when those are all the
// words it has, once the ones that only say "this is a theme" are set aside.
// anySource says the look is shown for it from any source, where every
// other look is for the sources a room is lit for (ADR-0030).
var pieces = []struct {
	title     string
	look      string
	anySource bool
}{
	{title: "knight rider", look: Scanner, anySource: true},
}

// filler is the words a title uses to say it is a theme, and nothing more.
var filler = map[string]bool{
	"theme": true, "from": true, "main": true, "title": true, "titles": true, "tv": true,
	"the": true, "original": true, "version": true, "opening": true, "music": true,
}

// bare is a title's own words, lower case and in order: without anything in
// parentheses, without punctuation, and without the filler.
func bare(title string) string {
	var words []string
	depth := 0
	word := func(r rune) bool {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		}
		return depth > 0 || !(unicode.IsLetter(r) || unicode.IsDigit(r))
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(title), word) {
		if !filler[w] {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// AnySource reports whether a look is shown whatever source its track is
// from. Nearly none is: a room is lit for the sources it is lit for.
func AnySource(look string) bool {
	for _, p := range pieces {
		if p.look == look && p.anySource {
			return true
		}
	}
	return false
}

// For returns the look of a track: its own, if it is a piece that has one,
// else that of the album it is on, or "" when it has none, which is nearly
// always.
//
// The album has to begin with the title, and the artist has to contain the
// name. "Dark Side Of The Moon (2011 Remaster)" and "The Dark Side of the
// Moon - 50th Anniversary" are both that album; a track with no album tag is
// not on any album as far as this can tell.
func For(t contract.Track) string {
	if title := bare(t.Title); title != "" {
		for _, p := range pieces {
			if title == p.title {
				return p.look
			}
		}
	}
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
// anything in parentheses and without a leading "the": what is left when
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
