package library

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Index is the owner's albums, by artist: what a request is matched to.
// Only names, years and counts are kept - no paths, plays or ratings.
type Index struct {
	V       int       `json:"v"`
	Built   time.Time `json:"built"`
	Artists []Artist  `json:"artists"`
}

// Artist is an artist with albums in the library: the album artist where the
// library has one, else the track artist.
type Artist struct {
	Name   string  `json:"name"`
	Albums []Album `json:"albums"`
}

// Album is one album, as the library has it: Title exactly as the player
// will match it.
type Album struct {
	Title       string `json:"title"`
	Year        int    `json:"year,omitempty"`
	Tracks      int    `json:"tracks"`
	Compilation bool   `json:"compilation,omitempty"`
}

// Version is the index file's format.
const Version = 1

// track is what is read of one track.
type track struct {
	artist, albumArtist, album, kind string
	year                             int
	compilation, notMusic            bool
}

// FromXML reads the library's XML ("iTunes Library.xml") as it streams by,
// and makes the index of its music. The file is tens of megabytes; nothing
// but the tracks' names is held.
func FromXML(r io.Reader) (*Index, error) {
	type key struct{ artist, album string }
	type found struct {
		artist, title string
		years         map[int]int
		tracks        int
		compilation   bool
	}
	albums := map[key]*found{}
	// Several spellings of one album: the one with the most tracks is the
	// album, and is what is played.
	spellings := map[key]map[string]int{}

	err := eachTrack(r, func(t track) {
		if t.notMusic || strings.TrimSpace(t.album) == "" {
			return
		}
		artist := strings.TrimSpace(t.albumArtist)
		if artist == "" {
			artist = strings.TrimSpace(t.artist)
		}
		if artist == "" {
			return
		}
		k := key{Name(artist), Fold(t.album)}
		f := albums[k]
		if f == nil {
			f = &found{artist: artist, years: map[int]int{}}
			albums[k] = f
			spellings[k] = map[string]int{}
		}
		f.tracks++
		f.compilation = f.compilation || t.compilation
		if t.year > 0 {
			f.years[t.year]++
		}
		spellings[k][t.album]++
	})
	if err != nil {
		return nil, err
	}

	byArtist := map[string]*Artist{}
	var names []string
	for k, f := range albums {
		title, most := "", -1
		for s, n := range spellings[k] {
			if n > most || (n == most && s < title) {
				title, most = s, n
			}
		}
		year, seen := 0, 0
		for y, n := range f.years {
			if n > seen || (n == seen && y < year) {
				year, seen = y, n
			}
		}
		a := byArtist[k.artist]
		if a == nil {
			a = &Artist{Name: f.artist}
			byArtist[k.artist] = a
			names = append(names, k.artist)
		}
		a.Albums = append(a.Albums, Album{Title: title, Year: year, Tracks: f.tracks, Compilation: f.compilation})
	}
	sort.Strings(names)
	ix := &Index{V: Version, Built: time.Now().UTC()}
	for _, n := range names {
		a := byArtist[n]
		sort.Slice(a.Albums, func(i, j int) bool { return a.Albums[i].Title < a.Albums[j].Title })
		ix.Artists = append(ix.Artists, *a)
	}
	return ix, nil
}

// eachTrack walks the plist's Tracks dictionary, handing each track over.
func eachTrack(r io.Reader, each func(track)) error {
	d := xml.NewDecoder(r)
	d.Strict = false
	depth := 0
	inTracks := false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("library: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict":
				depth++
				// plist > dict(1) > key Tracks > dict(2) > key id > dict(3)
				if inTracks && depth == 3 {
					tr, err := readTrack(d)
					if err != nil {
						return err
					}
					depth-- // readTrack consumed the closing </dict>
					each(tr)
				}
			case "key":
				var k string
				if err := d.DecodeElement(&k, &t); err != nil {
					return fmt.Errorf("library: %w", err)
				}
				if depth == 1 {
					inTracks = k == "Tracks"
				}
			}
		case xml.EndElement:
			if t.Name.Local == "dict" {
				depth--
				if depth == 1 {
					inTracks = false // the Tracks dictionary has ended
				}
			}
		}
	}
}

// readTrack reads one track's dictionary, up to and including its </dict>.
func readTrack(d *xml.Decoder) (track, error) {
	var t track
	var key string
	for {
		tok, err := d.Token()
		if err != nil {
			return t, fmt.Errorf("library: %w", err)
		}
		switch e := tok.(type) {
		case xml.EndElement:
			if e.Name.Local == "dict" {
				return t, nil
			}
		case xml.StartElement:
			if e.Name.Local == "key" {
				if err := d.DecodeElement(&key, &e); err != nil {
					return t, fmt.Errorf("library: %w", err)
				}
				continue
			}
			var v string
			switch e.Name.Local {
			case "true", "false":
				if err := d.Skip(); err != nil {
					return t, err
				}
				v = e.Name.Local
			default:
				if err := d.DecodeElement(&v, &e); err != nil {
					return t, fmt.Errorf("library: %w", err)
				}
			}
			switch key {
			case "Artist":
				t.artist = v
			case "Album Artist":
				t.albumArtist = v
			case "Album":
				t.album = v
			case "Kind":
				t.kind = v
				t.notMusic = t.notMusic || strings.Contains(strings.ToLower(v), "video")
			case "Year":
				t.year, _ = strconv.Atoi(v)
			case "Compilation":
				t.compilation = v == "true"
			case "Podcast", "Movie", "TV Show", "Music Video", "Has Video", "iTunesU", "Audiobook":
				t.notMusic = t.notMusic || v == "true"
			}
		}
	}
}

// Load reads an index written by Save.
func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ix Index
	if err := json.Unmarshal(data, &ix); err != nil {
		return nil, fmt.Errorf("library: %s: %w", path, err)
	}
	if ix.V != Version {
		return nil, fmt.Errorf("library: %s is version %d, this program reads %d", path, ix.V, Version)
	}
	return &ix, nil
}

// File is an index on disk, read again when it changes: a new one is
// copied over it by hand (make -C deploy library), and the next request
// sees it.
type File struct {
	Path string

	mu  sync.Mutex
	mod time.Time
	ix  *Index
	err error
}

// Index is the file's index as it is now, or nil when there is none: no
// file, or one that cannot be read, which is said once.
func (f *File) Index() (*Index, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := os.Stat(f.Path)
	if err != nil {
		f.ix, f.err = nil, nil
		if !os.IsNotExist(err) {
			f.err = err
		}
		return nil, f.err
	}
	if st.ModTime().Equal(f.mod) && (f.ix != nil || f.err != nil) {
		return f.ix, f.err
	}
	f.mod = st.ModTime()
	f.ix, f.err = Load(f.Path)
	return f.ix, f.err
}
