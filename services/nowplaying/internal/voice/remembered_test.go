package voice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

func TestADiscographyIsKeptAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discographies.json")
	first := &discographies{}
	r := &Remembered{Next: first, Path: path, Logger: quiet}
	if d, err := r.Discography(context.Background(), "Van Halen"); err != nil || d.Artist != "Van Halen" {
		t.Fatalf("%+v, %v", d, err)
	}

	// A restart: a new Remembered on the same file, and a counter that
	// would fail if it were asked.
	again := &discographies{}
	r2 := &Remembered{Next: again, Path: path, Logger: quiet}
	d, err := r2.Discography(context.Background(), "van halen")
	if err != nil || d.Artist != "Van Halen" || len(d.Albums) != 2 || d.Albums[1].Title != "Van Halen II" {
		t.Fatalf("after the restart: %+v, %v", d, err)
	}
	if len(again.asked) != 0 {
		t.Errorf("MusicBrainz was asked again: %q", again.asked)
	}
}

func TestAMissIsNotKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discographies.json")
	d := &discographies{}
	r := &Remembered{Next: d, Path: path, Logger: quiet}
	for range 2 {
		if _, err := r.Discography(context.Background(), "Arrowsmith"); !errors.Is(err, resolve.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}
	if len(d.asked) != 2 {
		t.Errorf("asked %d times, want each time", len(d.asked))
	}
}

func TestWhatIsKeptGrowsOldAndIsAskedAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discographies.json")
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	d := &discographies{}
	r := &Remembered{Next: d, Path: path, Logger: quiet, Now: func() time.Time { return now }}
	_, _ = r.Discography(context.Background(), "Van Halen")
	now = now.Add(31 * 24 * time.Hour)
	_, _ = r.Discography(context.Background(), "Van Halen")
	if len(d.asked) != 2 {
		t.Errorf("asked %d times, want again after a month", len(d.asked))
	}
}

// A file that cannot be read starts empty, and is written whole when the
// next discography is found.
func TestAnUnreadableFileIsStartedAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discographies.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Remembered{Next: &discographies{}, Path: path, Logger: quiet}
	if _, err := r.Discography(context.Background(), "Van Halen"); err != nil {
		t.Fatal(err)
	}
	r2 := &Remembered{Next: &discographies{}, Path: path, Logger: quiet}
	if _, err := r2.Discography(context.Background(), "Van Halen"); err != nil {
		t.Fatal(err)
	}
	if n := len(r2.Next.(*discographies).asked); n != 0 {
		t.Errorf("the rewritten file was not read: asked %d", n)
	}
}
