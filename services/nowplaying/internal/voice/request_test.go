package voice

import (
	"errors"
	"testing"
)

func TestRequestsAreRead(t *testing.T) {
	for words, want := range map[string]Request{
		// As scribe wrote them on 2026-10-24, from requests said by `say`.
		"Play Van Halen's first album.":          {Artist: "Van Halen", Nth: 1},
		"Play the first album by Guns N' Roses.": {Artist: "Guns N' Roses", Nth: 1},
		"Play Abbey Road by the Beatles.":        {Artist: "the Beatles", Album: "Abbey Road"},
		"Play Led Zeppelin's fourth album.":      {Artist: "Led Zeppelin", Nth: 4},

		// Said other ways that mean the same.
		"play aerosmith's debut album":                       {Artist: "aerosmith", Nth: 1},
		"Play Van Halens first album":                        {Artist: "Van Halens", Nth: 1}, // the s stays: Talking Heads ends in one too
		"Play Talking Heads first album":                     {Artist: "Talking Heads", Nth: 1},
		"Play Aerosmith’s 3rd record":                        {Artist: "Aerosmith", Nth: 3},
		"Play the latest album by AC/DC":                     {Artist: "AC/DC", Nth: -1},
		"Play the most recent album from Rush.":              {Artist: "Rush", Nth: -1},
		"Play their second album by Boston":                  {Artist: "Boston", Nth: 2},
		"Hey Ed, play Rush's first album.":                   {Artist: "Rush", Nth: 1},
		"Could you please play Kind of Blue by Miles Davis?": {Artist: "Miles Davis", Album: "Kind of Blue"},
		"Play the album Back in Black by AC/DC, please.":     {Artist: "AC/DC", Album: "Back in Black"},
		"  PLAY   Rocks   by   Aerosmith  ":                  {Artist: "Aerosmith", Album: "Rocks"},

		// The end of the wake word, kept with the request, or a filler.
		"...vis. Play Aerosmith's first album.": {Artist: "Aerosmith", Nth: 1},
		"Jarvis, play Rocks by Aerosmith.":      {Artist: "Aerosmith", Album: "Rocks"},
		"Um, play Van Halen's first album":      {Artist: "Van Halen", Nth: 1},

		// "play" misheard, where the rest is an album by its place, as
		// scribe wrote it on 2026-10-24 and before.
		"Tae Van Halen's first album.":           {Artist: "Van Halen", Nth: 1},
		"Clay Aerosmith's first album.":          {Artist: "Aerosmith", Nth: 1},
		"...vis. Pray the third album by Boston": {Artist: "Boston", Nth: 3},
		// The end of "Hey Jarvis", as scribe wrote it, and "play" as "played".
		"And each of us played Van Halen's first album.": {Artist: "Van Halen", Nth: 1},
	} {
		got, err := Parse(words)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", words, got, err, want)
		}
	}
}

// Not a request, or not one this grammar reads: nothing is guessed.
func TestWhatIsNotARequestIsNotRead(t *testing.T) {
	for _, words := range []string{
		"",
		"Thank you.",
		"Play something good.",
		"Play ACDC's Z latest album.", // as scribe misheard AC/DC
		"Play Van Halen's eleventh album.",
		"Stop.",
		"Play by Van Halen",
		"Play the first album by",
		"Clay Rocks by Aerosmith.", // a misheard play, without a place: too like talk
		"They went by the store.",
		"Don't pay Van Halen's first album.",     // a negation, before a misheard play
		"They said it was his first album.",      // no album by its place
		"I told him to play Rocks by Aerosmith.", // ALBUM by ARTIST, too far in
		"I don't think you should play Van Halen's first album.",
		"One two three four five six play Van Halen's first album.", // six words in
		"Don't play Van Halen's first album.",
		"Do not play Rocks by Aerosmith.",
		"No, play Rocks by Aerosmith.",
		"Never play Rocks by Aerosmith.",
		"One two three play Rocks by Aerosmith.", // ALBUM by ARTIST, too far from the start
	} {
		if got, err := Parse(words); !errors.Is(err, ErrNotARequest) {
			t.Errorf("Parse(%q) = %+v, %v; want not a request", words, got, err)
		}
	}
}
