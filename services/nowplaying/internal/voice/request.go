// Package voice reads a spoken request as words and says which album it asks
// for (ADR-0042).
//
// The grammar is short on purpose. A request said another way is not
// understood, and nothing nearer is guessed: the wrong record is worse than
// none, and "I didn't catch that" is an answer a person can act on.
//
//	play the first album by Van Halen     an artist's Nth album
//	play Van Halen's first album          the same, said the other way
//	play Abbey Road by the Beatles        an album by its name
//	play Abbey Road from the Beatles      the same
//	play the Beatles album Abbey Road     the same, the artist first
//	play the Beatles, Abbey Road          the same, with a comma or an 's
//	play the song Come Together by the Beatles   a song (ADR-0055)
//	play some Van Halen                   an artist, shuffled (ADR-0055)
//	shuffle Van Halen                     the same
//
// "Put on", "let's hear", "I want to hear" and a few more are "play".
// "Play X" alone, an artist or an album, is ParseName's, with the library.
package voice

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Request is an album asked for: by its name, or by its place among the
// artist's albums (ADR-0041); or a song, an artist, or a name that is one
// of the two (ADR-0055), as Kind says.
type Request struct {
	Kind   Kind
	Artist string
	// Album is the album's name; empty when Nth says which.
	Album string
	// Nth counts the artist's studio albums from 1, or back from the newest
	// at -1. Zero when the album is named.
	Nth int
	// Title is the song's title, or the name said alone.
	Title string
}

// Kind is what a request asks for.
type Kind int

const (
	// AnAlbum by its name or its place. An album by its name the library
	// has not got, by an artist it has, may be a song of theirs instead.
	AnAlbum Kind = iota
	// ASong by its artist and title, said as one: "the song X by Y".
	ASong
	// AnArtist, all their songs, shuffled.
	AnArtist
	// AName said alone: the artist, if the library has one by it, else the
	// album, whoever it is by.
	AName
)

// ErrNotARequest means the words are not one this grammar reads.
var ErrNotARequest = errors.New("voice: not a request for an album")

var (
	// "play the first album by X", "play the debut record from X"
	nthBy = regexp.MustCompile(`(?i)^play\s+(?:the\s+|their\s+|his\s+|her\s+)?(\S+(?:\s+recent)?)\s+(?:album|record|lp)\s+(?:by|from)\s+(.+)$`)
	// "play X's first album", "play Van Halens first album"
	// No "s'" of its own: with the name taken as short as it can be,
	// "Genesis' first album" would ask for "Genesi". The apostrophe alone
	// covers it, and "the Beatles' latest album" with it.
	possessive = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|’|'|,)?\s+(\S+(?:\s+recent)?)\s+(?:album|record|lp)$`)
	// "play Abbey Road by the Beatles"
	// "play album The Wall by Pink Floyd" too (2026-10-24).
	named = regexp.MustCompile(`(?i)^play\s+(?:(?:the\s+)?album\s+)?(.+?)\s+(?:by|from)\s+(.+)$`)
	// "play the Beatles album Abbey Road", "play Pink Floyd's album The Wall"
	// "play Pink Floyd, the album The Wall" too.
	artistFirst = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|’|')?,?\s+(?:the\s+)?(?:album|record|lp)\s+(.+)$`)
	// "play the Beatles, Abbey Road", "play the Beatles' Abbey Road album".
	// At the last comma, so that "Crosby, Stills, Nash & Young, Déjà Vu" is
	// theirs; at the first 's, as "Van Halen's first album" is read.
	commaFirst = regexp.MustCompile(`(?i)^play\s+(.+),\s+(?:the\s+album\s+)?(.+?)(?:\s+(?:album|record|lp))?$`)
	saidFirst  = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|’|')\s+(?:album\s+)?(.+?)(?:\s+(?:album|record|lp))?$`)
)

var (
	// "play the song Come Together by the Beatles", "the track X from Y"
	songBy = regexp.MustCompile(`(?i)^play\s+(?:the\s+)?(?:song|track|tune)\s+(.+?)\s+(?:by|from)\s+(.+)$`)
	// "play the Beatles song Come Together", "the Beatles' song X"
	songOf = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|’|')?\s+(?:song|track|tune)\s+(.+)$`)
	// "play some Van Halen", "play songs by Van Halen", "anything by X"
	// "Music from" is too often an era or a place ("music from the 80s"):
	// music is an artist's only with "by".
	someOf = regexp.MustCompile(`(?i)^play\s+(?:(?:some|any)(?:thing)?\s+(?:songs?\s+|tracks?\s+)?(?:by|from)\s+|(?:some\s+)?(?:songs|tracks|stuff)\s+(?:by|from)\s+|(?:some\s+)?music\s+by\s+|some\s+)(.+)$`)
	// "shuffle Van Halen", "play Van Halen shuffled", "shuffle songs by X"
	shuffled = regexp.MustCompile(`(?i)^(?:shuffle\s+(?:(?:songs|music|tracks|everything)\s+(?:by|from)\s+)?(.+)|play\s+(.+?)\s+(?:shuffled|on\s+shuffle))$`)
	// "play X", nothing else: a name alone
	alone = regexp.MustCompile(`(?i)^play\s+(.+)$`)
	// what a name alone never has in it: the shape of another request, or
	// of a sentence
	notAlone = regexp.MustCompile(`(?i)(?:\s(?:by|from|album|record|lp|song|track)(?:\s|$)|'s\s|’s\s|,)`)
)

// vague is what stands where an album's name would, but is not one: "play
// something from Pink Floyd", "play music from the 80s". Read as names,
// they would be asked for, missed, and the music stopped (ADR-0045).
var vague = regexp.MustCompile(`(?i)^(?:something|anything|some(?:thing)?\s+.*|music|songs?|stuff|tracks?|hits|the\s+hits|(?:a|the)\s+(?:song|track|tune|record)|an?\s+album|the\s+album|albums?|records?|it|them|that|this)$`)

// place is a word that says an album's place, known or not: "Van Halen's
// eleventh album" is a place this grammar does not count, never an album
// called "eleventh", and "the first album by" is no artist "the first".
var place = regexp.MustCompile(`(?i)(?:^|\s)(?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|eleventh|twelfth|\S+teenth|\S+tieth|debut|latest|newest|last|recent|\d+(?:st|nd|rd|th))$`)

// joining starts what is not an album's name but the rest of a sentence.
var joining = regexp.MustCompile(`(?i)^(?:by|from)(?:\s|$)`)

// article alone is not an artist: "play the album The Wall" names none.
var article = regexp.MustCompile(`(?i)^(?:the|a|an|their|his|her|my|our|your)$`)

// pronoun ends a phrase that names nobody.
var pronoun = regexp.MustCompile(`(?i)(?:^|\s)(?:it|them|him|her|us|me|you|this|that|his|its|their|our|my|your|those|these)$`)

// inner is a possessive inside a name, with words after it.
var inner = regexp.MustCompile(`(?i)(?:'s|’s)\s`)

// ordinals are the places a request can name. The tenth is as far as a
// person counts aloud.
var ordinals = map[string]int{
	"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5,
	"sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10,
	"debut":  1,
	"latest": -1, "newest": -1, "last": -1, "most recent": -1,
}

// Parse reads the words of a request. They are as speech-to-text wrote them:
// capitals, punctuation, perhaps the wake word or a "please".
//
// A possessive said without its apostrophe keeps its s ("Van Halens first
// album" asks for "Van Halens"): the grammar cannot tell it from a name that
// ends in one, such as Talking Heads, and whoever looks the artist up can.
func Parse(words string) (Request, error) {
	if r, err := parseShuffle(clean(words)); err == nil {
		return r, nil
	}
	text, ok := fromPlay(clean(words))
	if !ok {
		// "play" misheard is taken for "play" only where the rest is an
		// album by its place: "Tae Van Halen's first album" was one
		// (2026-10-24), and a place and the word "album" are not said by
		// accident. "ALBUM by ARTIST" is too like ordinary talk, and needs
		// the word itself.
		if text, ok = farPlay(clean(words)); ok {
			if r, err := byPlace(text); err == nil && strictlyByPlace(text) {
				return r, nil
			}
		}
		return Request{}, ErrNotARequest
	}
	if r, err := byPlace(text); err == nil {
		return r, nil
	}
	if r, err := songOrArtist(text); err == nil {
		return r, nil
	}
	return byName(text)
}

// songOrArtist reads a song said as one, or an artist said as one.
func songOrArtist(text string) (Request, error) {
	if m := songBy.FindStringSubmatch(text); m != nil {
		return song(m[2], m[1])
	}
	if m := songOf.FindStringSubmatch(text); m != nil {
		return song(m[1], m[2])
	}
	if m := someOf.FindStringSubmatch(text); m != nil {
		return artistAlone(m[1])
	}
	return Request{}, ErrNotARequest
}

// parseShuffle reads "shuffle X", which does not begin with a way of
// asking for something to play.
func parseShuffle(text string) (Request, error) {
	if m := shuffled.FindStringSubmatch(text); m != nil {
		return artistAlone(m[1] + m[2])
	}
	return Request{}, ErrNotARequest
}

// ParseName reads "play X" said alone, which only the owner's library can
// tell an artist from an album in (ADR-0055). The caller asks it only when
// Parse has not read the words, and only with a library: without one,
// nothing is guessed.
func ParseName(words string) (Request, error) {
	text, ok := fromPlay(clean(words))
	if !ok {
		return Request{}, ErrNotARequest
	}
	m := alone.FindStringSubmatch(text)
	if m == nil {
		return Request{}, ErrNotARequest
	}
	name := strings.TrimSpace(m[1])
	if notAlone.MatchString(" "+name) || len(strings.Fields(name)) > 8 || unnamed(name) {
		return Request{}, ErrNotARequest
	}
	return Request{Kind: AName, Title: name}, nil
}

func song(artist, title string) (Request, error) {
	artist, title = strings.TrimSpace(artist), strings.TrimSpace(title)
	if artist == "" || title == "" || unnamed(artist) || vague.MatchString(title) || joining.MatchString(title) {
		return Request{}, ErrNotARequest
	}
	return Request{Kind: ASong, Artist: artist, Title: title}, nil
}

func artistAlone(artist string) (Request, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" || unnamed(artist) || vague.MatchString(artist) {
		return Request{}, ErrNotARequest
	}
	return Request{Kind: AnArtist, Artist: artist}, nil
}

// unnamed is whether words name nobody: an article, a pronoun, or a place.
func unnamed(name string) bool {
	return article.MatchString(name) || pronoun.MatchString(name) || place.MatchString(name) || inner.MatchString(name) || vague.MatchString(name)
}

// byName reads an album by its name, with its artist said after it or
// before it.
func byName(text string) (Request, error) {
	for _, form := range []struct {
		re            *regexp.Regexp
		artist, album int
	}{
		{named, 2, 1},
		{artistFirst, 1, 2},
		{commaFirst, 1, 2},
		{saidFirst, 1, 2},
	} {
		m := form.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		if r, err := request(m[form.artist], m[form.album], 0); err == nil {
			return r, nil
		}
	}
	return Request{}, ErrNotARequest
}

// ParseOwned reads what Parse does not take for a request, but would be
// one if its album is in the owner's library: "ALBUM by ARTIST" after a
// misheard "play" ("Clay Revolver by the Beatles", "played Sticky Fingers by
// the Rolling Stones", 2026-10-24). Ordinary talk names an album the owner
// has by its artist seldom, and only after the wake word; the caller must
// play it only if the library has it (ADR-0046).
func ParseOwned(words string) (Request, error) {
	text, ok := farPlay(clean(words))
	if !ok {
		return Request{}, ErrNotARequest
	}
	if _, err := byPlace(text); err == nil {
		return Request{}, ErrNotARequest // by place, Parse's rules hold
	}
	// "Clay some Van Halen" (2026-11-11): an artist, or a song by one, the
	// caller plays only if the library has the artist.
	if r, err := songOrArtist(text); err == nil {
		return r, nil
	}
	if m := named.FindStringSubmatch(text); m != nil {
		return request(m[2], m[1], 0)
	}
	return Request{}, ErrNotARequest
}

// byPlace reads "play the Nth album by X" and "play X's Nth album".
func byPlace(text string) (Request, error) {
	if m := nthBy.FindStringSubmatch(text); m != nil {
		if n, ok := ordinal(m[1]); ok {
			return request(m[2], "", n)
		}
	}
	if m := possessive.FindStringSubmatch(text); m != nil {
		if n, ok := ordinal(m[2]); ok {
			return request(m[1], "", n)
		}
	}
	return Request{}, ErrNotARequest
}

// marked is "play X's Nth album" with the possessive said: without it,
// after a "play" that was misheard, any sentence ending "his first album"
// would be one. For a name ending in s, scribe writes the possessive as a
// comma: "Genesis, first album", "Led Zeppelin, first album" (2026-10-24).
var marked = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|’|'|,)\s+(\S+(?:\s+recent)?)\s+(?:album|record|lp)$`)

// strictlyByPlace is whether the words are an album by its place in a form
// that is not said by accident.
func strictlyByPlace(text string) bool {
	return nthBy.MatchString(text) || marked.MatchString(text)
}

// far finds "play", or what speech-to-text has written for it when its P
// was clipped or smeared ("Clay", "Tae", "played"), up to five words in: the
// device keeps 600 ms from before the wake word was decided on, and scribe
// has written the end of "Hey Jarvis" in it as "And each of us" (2026-10-24).
// Only an album by its place, strictly said, is read this far in.
var far = regexp.MustCompile(`(?i)^((?:\S+\s+){0,5}?)(?:play|playing|tae|tay|they|clay|lay|pay|pray|plate|flay|slay|blay|pla|ple|played|plays|placed|place)[,.]?\s+(.+)$`)

// farPlay is the words from "play", or its mishearing, made "play", never
// after a negation.
func farPlay(text string) (string, bool) {
	m := far.FindStringSubmatch(text)
	if m == nil || negation.MatchString(m[1]) {
		return "", false
	}
	return "play " + m[2], true
}

func request(artist, album string, nth int) (Request, error) {
	artist, album = strings.TrimSpace(artist), strings.TrimSpace(album)
	if artist == "" || (album == "" && nth == 0) {
		return Request{}, ErrNotARequest
	}
	if article.MatchString(artist) || place.MatchString(artist) {
		return Request{}, ErrNotARequest
	}
	if nth == 0 && (vague.MatchString(album) || place.MatchString(album) || joining.MatchString(album)) {
		return Request{}, ErrNotARequest
	}
	// A possessive with more after it is not a name, but words misheard:
	// "ACDC's Z" was AC/DC's, said by a voice that spelled it out.
	if inner.MatchString(artist) {
		return Request{}, ErrNotARequest
	}
	// Nor is a pronoun, or words ending in one: "play his first album"
	// names nobody, and "they saw it, first album" read with a misheard
	// "play" and a comma for a possessive would ask for an artist "saw it".
	if pronoun.MatchString(artist) {
		return Request{}, ErrNotARequest
	}
	return Request{Artist: artist, Album: album, Nth: nth}, nil
}

func ordinal(word string) (int, bool) {
	word = strings.ToLower(strings.Join(strings.Fields(word), " "))
	if n, ok := ordinals[word]; ok {
		return n, true
	}
	// "1st" to "10th", as speech-to-text sometimes writes them.
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if digits, ok := strings.CutSuffix(word, suffix); ok {
			if n, err := strconv.Atoi(digits); err == nil && n >= 1 && n <= 10 {
				return n, true
			}
		}
	}
	return 0, false
}

// asking is the ways of saying "play" that are read as it. "Listen to" is
// not one: "listen to this" is said to a person about anything.
const asking = `(?:play|put\s+on|let'?s\s+(?:hear|play)|let\s+me\s+hear|i\s+(?:want|wanna|would\s+like)\s+to\s+hear|i'?d\s+like\s+to\s+hear|can\s+(?:i|we)\s+hear|could\s+(?:i|we)\s+hear)`

// asked is a request that starts with one of them.
var asked = regexp.MustCompile(`(?i)^` + asking + `\s+(.*)$`)

// before is a word or two ahead of the asking: the end of the wake word,
// which the device keeps a moment of so that "play" keeps its P ("…vis.
// Play Rocks by Aerosmith"), or a filler ("um, play").
var before = regexp.MustCompile(`(?i)^((?:\S+\s+){1,2}?)(` + asking + `\s.*)$`)

// negation, ahead of "play", makes it not a request: "don't play Rocks by
// Aerosmith" is never a reason to play it.
var negation = regexp.MustCompile(`(?i)\b(?:don'?t|do\s+not|not|never|no|stop)\b`)

// fromPlay is the words from "play", or another way of asking, on, made
// "play", allowing a word or two before it.
func fromPlay(text string) (string, bool) {
	if m := asked.FindStringSubmatch(text); m != nil {
		return "play " + m[1], true
	}
	m := before.FindStringSubmatch(text)
	if m == nil || negation.MatchString(m[1]) {
		return "", false
	}
	if m := asked.FindStringSubmatch(m[2]); m != nil {
		return "play " + m[1], true
	}
	return "", false
}

// politeness is what a person says around a request that is not it: the
// wake word, "Hey DJ Ed" (ADR-0054), however "DJ" is written, or "Hey
// Jarvis", which the ears woke to before; and "please".
var politeness = regexp.MustCompile(`(?i)^(?:(?:hey|okay|ok|hi)[,]?\s+(?:` + dj + `[,.]?\s+)?(?:ed|jarvis)[,.!]?\s+|(?:` + dj + `[,.]?\s+)?(?:ed|jarvis)[,.!]\s+)?(?:(?:can|could|would|will)\s+you\s+(?:please\s+)?|please\s+)?`)

// dj is "DJ" as speech-to-text may write it.
const dj = `(?:dj|d\.\s?j\.|dee\s?jay|deejay)`

// clean is the words as one line: no surrounding punctuation, no wake word
// or "please", and "play" however it was capitalized.
func clean(words string) string {
	text := strings.Join(strings.Fields(words), " ")
	text = strings.Trim(text, ` .!?,"“”`)
	text = politeness.ReplaceAllString(text, "")
	text = strings.TrimSuffix(strings.TrimSuffix(text, " please"), ",")
	return strings.Trim(text, ` .!?,`)
}
