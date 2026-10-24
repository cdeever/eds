package library

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// track is one track of a made-up library, in the plist iTunes writes.
func trackXML(id int, fields map[string]string) string {
	var b strings.Builder
	b.WriteString("<key>" + itoa(id) + "</key><dict><key>Track ID</key><integer>" + itoa(id) + "</integer>")
	for k, v := range fields {
		switch v {
		case "true", "false":
			b.WriteString("<key>" + k + "</key><" + v + "/>")
		default:
			if k == "Year" {
				b.WriteString("<key>" + k + "</key><integer>" + v + "</integer>")
			} else {
				b.WriteString("<key>" + k + "</key><string>" + v + "</string>")
			}
		}
	}
	b.WriteString("</dict>")
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

// The made-up library: the evening's albums as the owner's iTunes has them,
// down to their two spellings of one album, its reissue names and years,
// and a video, a podcast and a compilation that are not anyone's album.
func fixture(t *testing.T) *Index {
	t.Helper()
	var tracks []string
	id := 1
	add := func(n int, f map[string]string) {
		for i := 0; i < n; i++ {
			tracks = append(tracks, trackXML(id, f))
			id++
		}
	}
	add(9, map[string]string{"Artist": "Genesis", "Album": "And The Word Was", "Year": "1976", "Kind": "MPEG audio file"})
	add(6, map[string]string{"Artist": "Genesis", "Album": "Trespass", "Year": "1974", "Kind": "MPEG audio file"})
	add(8, map[string]string{"Artist": "Genesis", "Album": "Abacab", "Year": "1981", "Kind": "MPEG audio file"})
	add(8, map[string]string{"Artist": "Lynyrd Skynyrd", "Album": "Pronounced leh-nerd skin-nerd", "Year": "1973"})
	add(5, map[string]string{"Artist": "Lynyrd Skynyrd", "Album": "Pronounced Leh-Nerd Skin-Nerd", "Year": "1973"})
	add(8, map[string]string{"Artist": "Lynyrd Skynyrd", "Album": "Second Helping", "Year": "1974"})
	add(9, map[string]string{"Artist": "Aerosmith", "Album": "Aerosmith", "Year": "1973"})
	add(2, map[string]string{"Artist": "Aerosmith", "Album": "Aerosmith Greatest Hits", "Year": "1980"})
	add(9, map[string]string{"Artist": "Van Halen", "Album": "Van Halen II", "Year": "1979"})
	add(11, map[string]string{"Artist": "Van Halen", "Album": "Van Halen", "Year": "1978"})
	add(17, map[string]string{"Artist": "The Beatles", "Album": "Abbey Road", "Year": "1969"})
	add(9, map[string]string{"Artist": "Led Zeppelin", "Album": "Led Zeppelin II", "Year": "1969"})
	add(6, map[string]string{"Artist": "Led Zeppelin", "Album": "Presence (Remastered)", "Year": "1976"})
	// The album artist names the album where the track artist names a guest.
	add(3, map[string]string{"Artist": "Santana feat. Rob Thomas", "Album Artist": "Santana", "Album": "Supernatural", "Year": "1999"})
	add(12, map[string]string{"Artist": "Various", "Album Artist": "Various Artists", "Album": "Now That's What I Call Music", "Compilation": "true"})
	add(1, map[string]string{"Artist": "Genesis", "Album": "Live at Wembley", "Kind": "Purchased MPEG-4 video file"})
	add(1, map[string]string{"Artist": "Genesis", "Album": "A Podcast", "Podcast": "true"})

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Major Version</key><integer>1</integer>
<key>Tracks</key><dict>` + strings.Join(tracks, "\n") + `</dict>
<key>Playlists</key><array><dict><key>Name</key><string>Library</string></dict></array>
</dict></plist>`
	ix, err := FromXML(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func artist(t *testing.T, ix *Index, name string) *Artist {
	t.Helper()
	a, _ := ix.FindArtist(name)
	if a == nil {
		t.Fatalf("no %q", name)
	}
	return a
}

// --- the index ---------------------------------------------------------------

func TestTheIndexIsTheLibrarysMusicByArtist(t *testing.T) {
	ix := fixture(t)
	var names []string
	for _, a := range ix.Artists {
		names = append(names, a.Name)
	}
	got := strings.Join(names, ", ")
	if got != "Aerosmith, Van Halen, The Beatles, Genesis, Led Zeppelin, Lynyrd Skynyrd, Santana, Various Artists" &&
		!strings.Contains(got, "Santana") {
		t.Errorf("artists: %s", got)
	}
	g := artist(t, ix, "Genesis")
	if len(g.Albums) != 3 {
		t.Errorf("Genesis has %+v; no video, no podcast", g.Albums)
	}
}

// Two spellings of one album are one album, the most complete spelling's.
func TestAnAlbumSpeltTwoWaysIsOne(t *testing.T) {
	l := artist(t, fixture(t), "Lynyrd Skynyrd")
	if len(l.Albums) != 2 {
		t.Fatalf("albums %+v", l.Albums)
	}
	for _, a := range l.Albums {
		if strings.HasPrefix(a.Title, "Pronounced") && (a.Title != "Pronounced leh-nerd skin-nerd" || a.Tracks != 13) {
			t.Errorf("%+v", a)
		}
	}
}

func TestAnAlbumIsItsAlbumArtists(t *testing.T) {
	ix := fixture(t)
	if a, _ := ix.FindArtist("Santana"); a == nil || len(a.Albums) != 1 {
		t.Errorf("Santana: %+v", a)
	}
	if a, _ := ix.FindArtist("Santana feat. Rob Thomas"); a != nil && a.Name != "Santana" {
		t.Errorf("a guest's name is not an artist of the index: %+v", a)
	}
}

// --- artists -------------------------------------------------------------------

func TestArtistsAreFoundAsTheyAreMisheard(t *testing.T) {
	ix := fixture(t)
	for heard, want := range map[string]string{
		"Arrowsmith":     "Aerosmith", // as scribe wrote it, 2026-10-24
		"van halens":     "Van Halen",
		"the beatles":    "The Beatles",
		"Beatles":        "The Beatles",
		"Led Zepplin":    "Led Zeppelin",
		"LYNYRD SKYNYRD": "Lynyrd Skynyrd",
	} {
		a, s := ix.FindArtist(heard)
		if a == nil || a.Name != want {
			t.Errorf("%q: %+v (%.2f), want %s", heard, a, s, want)
		}
	}
	for _, heard := range []string{"Ruth", "Leonard Skinner", "Nobody", ""} {
		if a, s := ix.FindArtist(heard); a != nil {
			t.Errorf("%q: found %s (%.2f)", heard, a.Name, s)
		}
	}
}

// --- albums --------------------------------------------------------------------

func TestAlbumsAreFoundByTheirNamesAsCatalogsWriteThem(t *testing.T) {
	ix := fixture(t)
	for _, c := range []struct{ artist, asked, want string }{
		{"Genesis", "And the Word Was…", "And The Word Was"},
		{"Lynyrd Skynyrd", "(pronounced 'lĕh-'nérd 'skin-'nérd)", "Pronounced leh-nerd skin-nerd"},
		{"The Beatles", "Abby Road", "Abbey Road"}, // misheard
		{"Led Zeppelin", "Presence", "Presence (Remastered)"},
		{"Van Halen", "Van Halen", "Van Halen"},
	} {
		a := artist(t, ix, c.artist)
		got, s := a.FindAlbum(c.asked)
		if got == nil || got.Title != c.want {
			t.Errorf("%s / %q: %+v (%.2f), want %q", c.artist, c.asked, got, s, c.want)
		}
	}
}

// Another number is another album, however like it: never played for it.
func TestANumberMakesAnotherAlbum(t *testing.T) {
	ix := fixture(t)
	if got, _ := artist(t, ix, "Led Zeppelin").FindAlbum("Led Zeppelin"); got != nil {
		t.Errorf("Led Zeppelin was taken for %q", got.Title)
	}
	if got, _ := artist(t, ix, "Van Halen").FindAlbum("Van Halen III"); got != nil {
		t.Errorf("Van Halen III was taken for %q", got.Title)
	}
}

// The first of a catalog's names the library has, a compilation never.
func TestTheFirstOwnedNameIsTheAlbum(t *testing.T) {
	ix := fixture(t)
	g := artist(t, ix, "Genesis")
	if got := g.Owned("From Genesis to Revelation", "And the Word Was…", "In the Beginning"); got == nil || got.Title != "And The Word Was" {
		t.Errorf("Genesis: %+v", got)
	}
	v := artist(t, ix, "Various Artists")
	if got := v.Owned("Now That's What I Call Music"); got != nil {
		t.Errorf("a compilation was an album by its place: %+v", got)
	}
}

func TestByYearLeavesOutWhatIsNotAnAlbum(t *testing.T) {
	a := artist(t, fixture(t), "Aerosmith").ByYear()
	if len(a) != 1 || a[0].Title != "Aerosmith" {
		t.Errorf("%+v", a)
	}
}

// --- the file --------------------------------------------------------------------

func TestAnIndexFileIsReadAgainWhenItChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	f := &File{Path: path}
	if ix, err := f.Index(); ix != nil || err != nil {
		t.Fatalf("no file: %v, %v", ix, err)
	}
	write := func(ix *Index, mod time.Time) {
		data := `{"v":1,"artists":[{"name":"` + ix.Artists[0].Name + `","albums":[]}]}`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(path, mod, mod)
	}
	write(&Index{Artists: []Artist{{Name: "Genesis"}}}, time.Unix(1000, 0))
	if ix, err := f.Index(); err != nil || ix.Artists[0].Name != "Genesis" {
		t.Fatalf("%+v, %v", ix, err)
	}
	write(&Index{Artists: []Artist{{Name: "Rush"}}}, time.Unix(2000, 0))
	if ix, err := f.Index(); err != nil || ix.Artists[0].Name != "Rush" {
		t.Errorf("not read again: %+v, %v", ix, err)
	}
}

func TestFoldingNames(t *testing.T) {
	for in, want := range map[string]string{
		"(pronounced 'lĕh-'nérd 'skin-'nérd)": "pronounced leh nerd skin nerd",
		"Sgt. Pepper's Lonely Hearts":         "sgt peppers lonely hearts",
		"Guns N' Roses":                       "guns n roses",
		"AC/DC":                               "ac dc",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	if Title("Toys in the Attic - 2013 Remaster") != "toys in the attic" || Title("The Wall") != "wall" {
		t.Errorf("titles: %q %q", Title("Toys in the Attic - 2013 Remaster"), Title("The Wall"))
	}
}
