package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
)

// --- playing an album (ADR-0040) ---------------------------------------------
//
// By hand, what a play_album command will do. It logs in with the stored
// pairing beside the running agent, as a second phone remote would.

func playAlbum(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return errors.New(`usage: npagent play-album ARTIST ALBUM   (e.g. "Van Halen" "Van Halen")`)
	}
	artist, album := args[0], args[1]

	p, err := player()
	if err != nil {
		return err
	}
	client := dacp.NewClient(p.Host, p.Port)
	ask, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Login(ask, p.GUID); err != nil {
		return err
	}
	if err := client.PlayAlbum(ask, artist, album); err != nil {
		return err
	}

	// What the player says it is playing is the proof, not the answer to the
	// request: it answers a query that matched nothing just as politely.
	time.Sleep(2 * time.Second)
	now, err := client.Now(ask)
	if err != nil {
		return err
	}
	fmt.Println("The player says:", describe(now))
	if !strings.EqualFold(now.Album, album) || !strings.EqualFold(now.Artist, artist) {
		return fmt.Errorf("the player is not playing %q by %q: the library may name it differently", album, artist)
	}
	return nil
}
