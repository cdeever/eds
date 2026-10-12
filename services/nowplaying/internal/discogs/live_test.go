package discogs

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive asks the real Discogs, and is skipped unless told whom to ask
// about: NP_TEST_DISCOGS="Artist|Title|Album" with NP_DISCOGS_USER set, and
// NP_DISCOGS_TOKEN for a private collection. It prints what it finds; what a
// collection holds is its owner's business and not something to assert.
func TestLive(t *testing.T) {
	ask, user := os.Getenv("NP_TEST_DISCOGS"), os.Getenv("NP_DISCOGS_USER")
	if ask == "" || user == "" {
		t.Skip("NP_TEST_DISCOGS and NP_DISCOGS_USER are not set")
	}
	c := &Collection{
		User:      user,
		Token:     os.Getenv("NP_DISCOGS_TOKEN"),
		HTTP:      &http.Client{Timeout: 20 * time.Second},
		UserAgent: "eds-nowplaying/test ( https://github.com/cdeever/eds )",
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	for _, one := range strings.Split(ask, ";") {
		artist, rest, _ := strings.Cut(one, "|")
		title, album, _ := strings.Cut(rest, "|")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		started := time.Now()
		got, err := c.Find(ctx, artist, title, album)
		cancel()
		if err != nil {
			t.Logf("%s - %s: %v (%s)", artist, title, err, time.Since(started).Round(time.Millisecond))
			continue
		}
		cover := "no cover"
		if got.Cover != nil {
			cover = fmt.Sprintf("%s, %d bytes", got.Cover.MIME, len(got.Cover.Image))
		}
		t.Logf("%s - %s: on %q (record %d), %s (%s)", artist, title, got.Album, c.last, cover, time.Since(started).Round(time.Millisecond))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	compilations := 0
	for _, r := range c.records {
		if r.compilation {
			compilations++
		}
	}
	t.Logf("%d records in the collection, %d of them compilations, %d tracklists read", len(c.records), compilations, len(c.tracks))
}
