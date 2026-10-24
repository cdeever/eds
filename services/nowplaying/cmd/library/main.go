// Command library makes the index of the owner's iTunes library that voiced
// matches requests to, and matches by hand against it (ADR-0046).
//
//	library index "iTunes Library.xml" > library.json
//	library match library.json ARTIST [N | ALBUM]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cdeever/eds/services/nowplaying/internal/library"
)

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	switch os.Args[1] {
	case "index":
		f, err := os.Open(os.Args[2])
		check(err)
		defer f.Close()
		ix, err := library.FromXML(f)
		check(err)
		albums := 0
		for _, a := range ix.Artists {
			albums += len(a.Albums)
		}
		fmt.Fprintf(os.Stderr, "%d artists, %d albums\n", len(ix.Artists), albums)
		enc := json.NewEncoder(os.Stdout)
		check(enc.Encode(ix))
	case "match":
		if len(os.Args) < 4 {
			usage()
		}
		ix, err := library.Load(os.Args[2])
		check(err)
		artist, score := ix.FindArtist(os.Args[3])
		if artist == nil {
			fmt.Printf("no artist like %q in the library\n", os.Args[3])
			os.Exit(1)
		}
		fmt.Printf("artist %q (%.2f), %d albums\n", artist.Name, score, len(artist.Albums))
		if len(os.Args) < 5 {
			for _, a := range artist.Albums {
				fmt.Printf("  %4d  %-50s %2d tracks%s\n", a.Year, a.Title, a.Tracks, map[bool]string{true: "  (compilation)"}[a.Compilation])
			}
			return
		}
		what := strings.Join(os.Args[4:], " ")
		if n, err := strconv.Atoi(what); err == nil {
			for i, a := range artist.ByYear() {
				if i+1 == n || (n < 0 && i == len(artist.ByYear())+n) {
					fmt.Printf("by year, the %d: %q (%d)\n", n, a.Title, a.Year)
				}
			}
			return
		}
		album, s := artist.FindAlbum(what)
		if album == nil {
			fmt.Printf("no album like %q by %s\n", what, artist.Name)
			os.Exit(1)
		}
		fmt.Printf("album %q (%.2f)\n", album.Title, s)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: library index "iTunes Library.xml" > library.json
       library match library.json ARTIST [N | ALBUM]`)
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "library:", err)
		os.Exit(1)
	}
}
