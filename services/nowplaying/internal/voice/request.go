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
package voice

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Request is an album asked for: by its name, or by its place among the
// artist's albums (ADR-0041).
type Request struct {
	Artist string
	// Album is the album's name; empty when Nth says which.
	Album string
	// Nth counts the artist's studio albums from 1, or back from the newest
	// at -1. Zero when the album is named.
	Nth int
}

// ErrNotARequest means the words are not one this grammar reads.
var ErrNotARequest = errors.New("voice: not a request for an album")

var (
	// "play the first album by X", "play the debut record from X"
	nthBy = regexp.MustCompile(`(?i)^play\s+(?:the\s+|their\s+|his\s+|her\s+)?(\S+(?:\s+recent)?)\s+(?:album|record|lp)\s+(?:by|from)\s+(.+)$`)
	// "play X's first album", "play Van Halens first album"
	possessive = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|s'|’|')?\s+(\S+(?:\s+recent)?)\s+(?:album|record|lp)$`)
	// "play Abbey Road by the Beatles"
	named = regexp.MustCompile(`(?i)^play\s+(?:the\s+album\s+)?(.+?)\s+by\s+(.+)$`)
)

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
	text, ok := fromPlay(clean(words))
	if !ok {
		// "play" misheard is taken for "play" only where the rest is an
		// album by its place: "Tae Van Halen's first album" was one
		// (2026-10-24), and a place and the word "album" are not said by
		// accident. "ALBUM by ARTIST" is too like ordinary talk, and needs
		// the word itself.
		if text, ok = misheardPlay(clean(words)); ok {
			if r, err := byPlace(text); err == nil && strictlyByPlace(text) {
				return r, nil
			}
		}
		return Request{}, ErrNotARequest
	}
	if r, err := byPlace(text); err == nil {
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
// would be one.
var marked = regexp.MustCompile(`(?i)^play\s+(.+?)(?:'s|’s|s')\s+(\S+(?:\s+recent)?)\s+(?:album|record|lp)$`)

// strictlyByPlace is whether the words are an album by its place in a form
// that is not said by accident.
func strictlyByPlace(text string) bool {
	return nthBy.MatchString(text) || marked.MatchString(text)
}

// misheard are what speech-to-text has written for "play" when its P was
// clipped or smeared, at the start of a request: "Clay", "Tae".
var misheard = regexp.MustCompile(`(?i)^((?:\S+\s+){0,2}?)(?:tae|tay|they|clay|lay|pay|pray|plate|flay|slay|blay|pla|ple|played|plays)[,.]?\s+(.+)$`)

// misheardPlay is the words with a misheard "play" made "play", allowing a
// word or two before it, never a negation.
func misheardPlay(text string) (string, bool) {
	m := misheard.FindStringSubmatch(text)
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
	// A possessive with more after it is not a name, but words misheard:
	// "ACDC's Z" was AC/DC's, said by a voice that spelled it out.
	if inner.MatchString(artist) {
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

// before is a word or two ahead of "play": the end of the wake word, which
// the device keeps a moment of so that "play" keeps its P ("…vis. Play
// Rocks by Aerosmith"), or a filler ("um, play").
var before = regexp.MustCompile(`(?i)^((?:\S+\s+){1,2}?)(play\s.*)$`)

// negation, ahead of "play", makes it not a request: "don't play Rocks by
// Aerosmith" is never a reason to play it.
var negation = regexp.MustCompile(`(?i)\b(?:don'?t|do\s+not|not|never|no|stop)\b`)

// fromPlay is the words from "play" on, allowing a word or two before it.
func fromPlay(text string) (string, bool) {
	if len(text) >= 5 && strings.EqualFold(text[:5], "play ") {
		return text, true
	}
	m := before.FindStringSubmatch(text)
	if m == nil || negation.MatchString(m[1]) {
		return "", false
	}
	return m[2], true
}

// politeness is what a person says around a request that is not it.
var politeness = regexp.MustCompile(`(?i)^(?:(?:hey|okay|ok|hi)\s+ed[,.!]?\s+|ed[,.!]\s+)?(?:(?:can|could|would|will)\s+you\s+(?:please\s+)?|please\s+)?`)

// clean is the words as one line: no surrounding punctuation, no wake word
// or "please", and "play" however it was capitalized.
func clean(words string) string {
	text := strings.Join(strings.Fields(words), " ")
	text = strings.Trim(text, ` .!?,"“”`)
	text = politeness.ReplaceAllString(text, "")
	text = strings.TrimSuffix(strings.TrimSuffix(text, " please"), ",")
	return strings.Trim(text, ` .!?,`)
}
