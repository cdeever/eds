package voice

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// Remembered keeps the discographies its Counter finds on disk, so that a
// restart does not forget them: counting an artist's albums is MusicBrainz's
// one call a second, two or three times over, and was most of the wait for
// "Aerosmith's first album" the first time it was asked after a deploy
// (CHG-0037).
//
// Only what was found is kept. A miss may be a name misheard or MusicBrainz
// busy, and is asked again. What is kept is believed for a month: a band's
// first album does not change, and its latest rarely.
type Remembered struct {
	Next   Counter
	Path   string // a JSON file; its directory must exist
	MaxAge time.Duration
	Logger *slog.Logger
	Now    func() time.Time

	mu     sync.Mutex
	loaded bool
	kept   map[string]rememberedDiscography
}

type rememberedDiscography struct {
	Discography resolve.Discography `json:"discography"`
	Found       time.Time           `json:"found"`
}

// Discography answers from what is kept, or asks and keeps the answer.
func (r *Remembered) Discography(ctx context.Context, artist string) (resolve.Discography, error) {
	key := resolve.Simplify(artist)
	r.mu.Lock()
	r.load()
	kept, ok := r.kept[key]
	r.mu.Unlock()
	// Kept before albums had their MusicBrainz ids (ADR-0045), it is asked
	// for again: without them, an album's other names cannot be.
	if ok && r.now().Sub(kept.Found) < r.maxAge() && hasIDs(kept.Discography) {
		return kept.Discography, nil
	}

	d, err := r.Next.Discography(ctx, artist)
	if err != nil {
		return d, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kept[key] = rememberedDiscography{Discography: d, Found: r.now()}
	if err := r.save(); err != nil {
		r.logger().Warn("discographies could not be kept; they will be asked for again after a restart", "path", r.Path, "error", err)
	}
	return d, nil
}

// load reads what was kept, once. A file that is missing or unreadable is an
// empty one: nothing is lost but time.
func (r *Remembered) load() {
	if r.loaded {
		return
	}
	r.loaded = true
	r.kept = map[string]rememberedDiscography{}
	data, err := os.ReadFile(r.Path)
	if err != nil {
		if !os.IsNotExist(err) {
			r.logger().Warn("kept discographies could not be read; starting without them", "path", r.Path, "error", err)
		}
		return
	}
	if err := json.Unmarshal(data, &r.kept); err != nil {
		r.logger().Warn("kept discographies are not readable; starting without them", "path", r.Path, "error", err)
		r.kept = map[string]rememberedDiscography{}
		return
	}
	r.logger().Info("kept discographies read", "artists", len(r.kept), "path", r.Path)
}

// save writes everything kept, whole, by renaming over the old file: a
// crash mid-write leaves the old one, not half of a new one.
func (r *Remembered) save() error {
	data, err := json.Marshal(r.kept)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.Path), ".discographies-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), r.Path)
}

func (r *Remembered) maxAge() time.Duration {
	if r.MaxAge > 0 {
		return r.MaxAge
	}
	return 30 * 24 * time.Hour
}

func (r *Remembered) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Remembered) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func hasIDs(d resolve.Discography) bool {
	for _, a := range d.Albums {
		if a.ID == "" {
			return false
		}
	}
	return len(d.Albums) > 0
}

// Titles asks the counter behind, which keeps nothing of them: they are
// asked for only when an album is, once a request.
func (r *Remembered) Titles(ctx context.Context, album resolve.StudioAlbum) ([]string, error) {
	if t, ok := r.Next.(Titler); ok {
		return t.Titles(ctx, album)
	}
	return nil, nil
}
