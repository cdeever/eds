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
		"Lee Jarvis, Play Van Halen's first album.":      {Artist: "Van Halen", Nth: 1},
		// As scribe writes the end of the wake word with "Hey Jarvis" in its prompt.
		"Jarvis, Play Van Halen's first album.":     {Artist: "Van Halen", Nth: 1},
		"Jarvis, played Genesis' first album.":      {Artist: "Genesis", Nth: 1},
		"And Jarvis, played Genesis's first album.": {Artist: "Genesis", Nth: 1},
		"Hey Jarvis, play Rocks by Aerosmith.":      {Artist: "Aerosmith", Album: "Rocks"},
		// "Hey DJ Ed" (ADR-0054), however "DJ" is written.
		"Hey DJ Ed, play Rocks by Aerosmith.":       {Artist: "Aerosmith", Album: "Rocks"},
		"Hey, DJ Ed, play Van Halen's first album.": {Artist: "Van Halen", Nth: 1},
		"Hey D.J. Ed, play Rush's first album.":     {Artist: "Rush", Nth: 1},
		"Hey deejay Ed. Play Genesis' first album.": {Artist: "Genesis", Nth: 1},
		"DJ Ed, could you play Rocks by Aerosmith?": {Artist: "Aerosmith", Album: "Rocks"},
		"Ed, play Kind of Blue by Miles Davis.":     {Artist: "Miles Davis", Album: "Kind of Blue"},
		"Play the Beatles' latest album.":           {Artist: "the Beatles", Nth: -1},
		// "Playing", and the comma scribe writes for a possessive after a
		// name ending in s, as they were heard on 2026-10-24.
		"The band was very generous. Playing Led Zeppelin's first album.": {Artist: "Led Zeppelin", Nth: 1},
		"Clay Genesis, first album.":                                      {Artist: "Genesis", Nth: 1},
		"Play Led Zeppelin, first album.":                                 {Artist: "Led Zeppelin", Nth: 1},
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
		"Don't pay Van Halen's first album.",       // a negation, before a misheard play
		"They said it was his first album.",        // no album by its place
		"Travis, third Led Zeppelin, first album.", // no play, nor any word for it
		"They saw it, first album.",                // a comma for a possessive, and a pronoun for a name
		"Pay them, first album.",
		"Play his first album.",
		"Play the second album by them.",
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

func TestAnAlbumByNameAfterAMisheardPlayIsOneIfOwned(t *testing.T) {
	// As scribe wrote them, 2026-10-24.
	for words, want := range map[string]Request{
		"Jarvis, Clay Revolver by the Beatles.":                {Artist: "the Beatles", Album: "Revolver"},
		"Jarvis, placed sticky fingers by the Rolling Stones.": {Artist: "the Rolling Stones", Album: "sticky fingers"},
		"Jarvis played sticky fingers by the Rolling Stones.":  {Artist: "the Rolling Stones", Album: "sticky fingers"},
	} {
		if _, err := Parse(words); err == nil {
			t.Errorf("Parse(%q) took it without the library", words)
		}
		if r, err := ParseOwned(words); err != nil || r != want {
			t.Errorf("ParseOwned(%q) = %+v, %v; want %+v", words, r, err, want)
		}
	}
	for _, words := range []string{
		"don't play Revolver by the Beatles",
		"they saw it, first album", // by place, Parse's rules
		"Revolver by the Beatles",  // no play at all
	} { // "I played it by ear" is read, as "it" by "ear", and the library has no such album

		if r, err := ParseOwned(words); err == nil {
			t.Errorf("ParseOwned(%q) = %+v; want not a request", words, r)
		}
	}
}

func TestTheWordAlbumIsNotTheAlbumsName(t *testing.T) {
	r, err := Parse("Play album The Wall by Pink Floyd.")
	if err != nil || r != (Request{Artist: "Pink Floyd", Album: "The Wall"}) {
		t.Errorf("%+v, %v", r, err)
	}
}

// Other ways of naming an album and its artist, and of asking (2026-11-11).
func TestMoreWaysOfAsking(t *testing.T) {
	for words, want := range map[string]Request{
		// As scribe wrote it on 2026-11-11, when it was not understood.
		"J. Ed. Play the Beatles, Abbey Road album.": {Artist: "the Beatles", Album: "Abbey Road"},
		"Play the Beatles, Abbey Road.":              {Artist: "the Beatles", Album: "Abbey Road"},
		"Play the Beatles' Abbey Road.":              {Artist: "the Beatles", Album: "Abbey Road"},
		"Play the Beatles' Abbey Road album":         {Artist: "the Beatles", Album: "Abbey Road"},
		"Play Van Halen's 1984.":                     {Artist: "Van Halen", Album: "1984"},
		"Play Crosby, Stills, Nash & Young, Déjà Vu": {Artist: "Crosby, Stills, Nash & Young", Album: "Déjà Vu"},
		"Play Pink Floyd, the album The Wall":        {Artist: "Pink Floyd", Album: "The Wall"},

		"Play Abbey Road from the Beatles.":                {Artist: "the Beatles", Album: "Abbey Road"},
		"Play the Beatles album Abbey Road.":               {Artist: "the Beatles", Album: "Abbey Road"},
		"Play Pink Floyd's album The Wall":                 {Artist: "Pink Floyd", Album: "The Wall"},
		"Play the Rolling Stones record Exile on Main St.": {Artist: "the Rolling Stones", Album: "Exile on Main St"},

		"Put on Abbey Road by the Beatles.":              {Artist: "the Beatles", Album: "Abbey Road"},
		"Hey DJ Ed, put on Van Halen's first album.":     {Artist: "Van Halen", Nth: 1},
		"Let's hear Rocks by Aerosmith.":                 {Artist: "Aerosmith", Album: "Rocks"},
		"Let me hear the Beatles, Revolver":              {Artist: "the Beatles", Album: "Revolver"},
		"I want to hear Genesis' first album.":           {Artist: "Genesis", Nth: 1},
		"I'd like to hear Kind of Blue by Miles Davis.":  {Artist: "Miles Davis", Album: "Kind of Blue"},
		"Can I hear the latest album by AC/DC?":          {Artist: "AC/DC", Nth: -1},
		"DJ Ed, could we hear Abbey Road by the Beatles": {Artist: "the Beatles", Album: "Abbey Road"},
		"Um, put on Rocks by Aerosmith.":                 {Artist: "Aerosmith", Album: "Rocks"},
	} {
		got, err := Parse(words)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", words, got, err, want)
		}
	}
}

// What the new ways would read wrongly, were it not for their guards.
func TestMoreWaysOfAskingGuessNothing(t *testing.T) {
	for _, words := range []string{
		"Play music from the 80s.",
		"Play the Beatles.",        // a name alone is ParseName's, with a library
		"Play the album The Wall.", // the same
		"Play some music.",
		"Shuffle.",
		"Play the song by the Beatles.",
		"Put on something good.",
		"I don't want to hear Rocks by Aerosmith.",
		"Don't put on Abbey Road by the Beatles.",
		"Listen to this.",
		"Listen to Abbey Road by the Beatles.", // "listen to" is not asking
		"Play Van Halen, eleventh album.",
		"Play the Beatles' album.",
		"Play the album by the Beatles.",
	} {
		if got, err := Parse(words); !errors.Is(err, ErrNotARequest) {
			t.Errorf("Parse(%q) = %+v, %v; want not a request", words, got, err)
		}
	}
}

// A song said as one, and an artist said as one (ADR-0055).
func TestASongOrAnArtistSaidAsOne(t *testing.T) {
	for words, want := range map[string]Request{
		"Play the song Come Together by the Beatles.":   {Kind: ASong, Artist: "the Beatles", Title: "Come Together"},
		"Hey DJ Ed, play the track Jump from Van Halen": {Kind: ASong, Artist: "Van Halen", Title: "Jump"},
		"Play the Beatles song Come Together":           {Kind: ASong, Artist: "the Beatles", Title: "Come Together"},
		"Put on Van Halen's song Panama.":               {Kind: ASong, Artist: "Van Halen", Title: "Panama"},

		"Play some Van Halen.":               {Kind: AnArtist, Artist: "Van Halen"},
		"Play something from Pink Floyd.":    {Kind: AnArtist, Artist: "Pink Floyd"},
		"Play some songs from the Beatles.":  {Kind: AnArtist, Artist: "the Beatles"},
		"Play anything by Aerosmith":         {Kind: AnArtist, Artist: "Aerosmith"},
		"Play songs by Genesis.":             {Kind: AnArtist, Artist: "Genesis"},
		"Play some music by Rush.":           {Kind: AnArtist, Artist: "Rush"},
		"Shuffle Van Halen.":                 {Kind: AnArtist, Artist: "Van Halen"},
		"Hey DJ Ed, shuffle songs by Boston": {Kind: AnArtist, Artist: "Boston"},
		"Play Van Halen shuffled.":           {Kind: AnArtist, Artist: "Van Halen"},
		"Let's hear some Led Zeppelin.":      {Kind: AnArtist, Artist: "Led Zeppelin"},
	} {
		got, err := Parse(words)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", words, got, err, want)
		}
	}
}

// A name alone is read only by ParseName, and only as a name.
func TestANameAlone(t *testing.T) {
	for words, want := range map[string]string{
		"Play the Beatles.":           "the Beatles",
		"Play Abbey Road.":            "Abbey Road",
		"Hey DJ Ed, put on Van Halen": "Van Halen",
		"I want to hear Kind of Blue": "Kind of Blue",
	} {
		got, err := ParseName(words)
		if err != nil || got != (Request{Kind: AName, Title: want}) {
			t.Errorf("ParseName(%q) = %+v, %v; want %q", words, got, err, want)
		}
	}
	for _, words := range []string{
		"Play something good.",
		"Play it.",
		"Play his first album.",
		"Play the album.",
		"Don't play Abbey Road.",
		"Play Rocks by Aerosmith and then something else entirely please now.", // a sentence
		"Thank you.",
		"Play the Beatles, Abbey Road", // another request's shape
	} {
		if got, err := ParseName(words); err == nil {
			t.Errorf("ParseName(%q) = %+v; want not a name", words, got)
		}
	}
}
