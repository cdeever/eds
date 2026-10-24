// Command discography lists an artist's studio albums as EdS counts them, and
// names one (ADR-0041). It is how "Aerosmith's first album" is checked by
// hand, before anything is played.
//
//	discography ARTIST        every studio album, oldest first
//	discography ARTIST N      the Nth; -1 is the latest
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/calls"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: discography ARTIST [N]")
		os.Exit(2)
	}
	n := 0
	if len(os.Args) == 3 {
		var err error
		if n, err = strconv.Atoi(os.Args[2]); err != nil || n == 0 {
			fmt.Fprintf(os.Stderr, "discography: %q is not a count from 1, or back from -1\n", os.Args[2])
			os.Exit(2)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o := &resolve.Originals{
		MusicBrainz: "https://musicbrainz.org",
		HTTP:        calls.New().Client(10 * time.Second),
		UserAgent:   "eds-discography/dev ( https://github.com/cdeever/eds )",
	}
	d, err := o.Discography(ctx, os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "discography:", err)
		os.Exit(1)
	}

	if n != 0 {
		a, err := d.Nth(n)
		if err != nil {
			fmt.Fprintln(os.Stderr, "discography:", err)
			os.Exit(1)
		}
		// What npagent play-album takes, in its order.
		fmt.Printf("%q %q\t(%s)\n", d.Artist, a.Title, a.Released)
		return
	}
	fmt.Println(d.Artist)
	for i, a := range d.Albums {
		fmt.Printf("%3d  %-10s  %s\n", i+1, a.Released, a.Title)
	}
}
