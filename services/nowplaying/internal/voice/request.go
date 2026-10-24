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
	text := clean(words)
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
	if m := named.FindStringSubmatch(text); m != nil {
		return request(m[2], m[1], 0)
	}
	return Request{}, ErrNotARequest
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
