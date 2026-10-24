// Package library knows the owner's iTunes library as its XML lists it, and
// finds in it what a request names (ADR-0046).
//
// iTunes will not list its library to EdS over DACP (ADR-0040), so every
// request used to be a guess at how the library spells an album - and the
// library spells them as it pleases: a reissue's name, its own capitals, no
// accents. With the library's own list, a request is matched to the nearest
// of the owner's albums, and the player is asked for that one exactly.
package library

import (
	"strings"
	"unicode"
)

// Fold is a name as names are compared: lowercase, without accents, letters
// and digits only, words single-spaced. "(pronounced 'lĕh-'nérd
// 'skin-'nérd)" folds to "pronounced leh nerd skin nerd".
func Fold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if f := FoldRune(r); f != 0 {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(f)
			continue
		}
		// An apostrophe joins: "Pepper's" is "peppers", "Guns N' Roses" is
		// "guns n roses". Anything else separates.
		if r != '\'' && r != '’' {
			space = true
		}
	}
	return b.String()
}

// FoldRune is a letter or digit as it is compared, or 0 for anything else.
func FoldRune(r rune) rune {
	r = unicode.ToLower(r)
	if f, ok := unaccented[r]; ok {
		return f
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return r
	}
	return 0
}

// unaccented are the accented letters names use, as their plain ones.
var unaccented = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, accented := range map[rune]string{
		'a': "áàâäãåāăą", 'c': "çćč", 'e': "éèêëēĕėęě", 'i': "íìîïīĭį", 'n': "ñńň",
		'o': "óòôöõøōŏő", 'u': "úùûüūŭůű", 'y': "ýÿ", 's': "śšş", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = plain
		}
	}
	return m
}()

// edition is what a library or a catalog adds to an album's name for one
// release of it: what is in brackets, and what follows " - " when it says
// remaster, edition, version or deluxe.
func withoutEdition(title string) string {
	var b strings.Builder
	depth := 0
	for _, r := range title {
		switch r {
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if i := strings.Index(out, " - "); i > 0 {
		tail := strings.ToLower(out[i:])
		for _, w := range []string{"remaster", "edition", "version", "deluxe", "expanded", "anniversary", "mono", "stereo"} {
			if strings.Contains(tail, w) {
				out = out[:i]
				break
			}
		}
	}
	if strings.TrimSpace(out) == "" {
		return title // a title that is all brackets: "(pronounced ...)"
	}
	return out
}

// Title is an album's name as albums are compared: folded, without its
// edition, without a leading "the".
func Title(title string) string {
	t := Fold(withoutEdition(title))
	if rest, ok := strings.CutPrefix(t, "the "); ok && rest != "" {
		return rest
	}
	return t
}

// Name is an artist's name as artists are compared: folded, without a
// leading "the", and "and" for "&".
func Name(name string) string {
	n := Fold(strings.ReplaceAll(name, "&", " and "))
	if rest, ok := strings.CutPrefix(n, "the "); ok && rest != "" {
		return rest
	}
	return n
}
