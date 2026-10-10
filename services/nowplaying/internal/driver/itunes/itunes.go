// Package itunes is the driver for iTunes and the Music app.
//
// It holds a request open against the player and reports each change the
// player pushes (ADR-0011). It reports what the player says and looks nothing
// up: the album and the cover come from the player's own library.
package itunes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
)

// Update is one thing for the agent to publish.
type Update struct {
	State contract.State
	// Cover is set when the cover changed with this state, and is the image
	// the state's Art names. It is published before the state.
	Cover []byte
}

// Sink receives what the driver sees.
type Sink interface {
	// Online reports whether the player can be reached. A driver is online
	// when it has a session with its player, not merely when it is running.
	Online(online bool)
	Publish(Update)
}

// Player is the part of the protocol client the driver uses.
type Player interface {
	Login(ctx context.Context, guid string) error
	Now(ctx context.Context) (dacp.Status, error)
	Wait(ctx context.Context, revision uint32) (dacp.Status, error)
	Artwork(ctx context.Context, size int) ([]byte, error)
}

// Driver follows one player.
type Driver struct {
	Source string // the source id this driver reports as
	Kind   string // "itunes" or "music"
	GUID   string // the pairing
	Player Player
	Sink   Sink
	Logger *slog.Logger

	// CoverSize is the largest cover asked for, in pixels. palette works
	// from 160; 600 leaves room for anything else that wants the picture.
	CoverSize int
	// Idle is how long the player may say nothing before the connection is
	// assumed dead. The player says nothing for the length of a track, and
	// also when it has lost power; only time tells those apart.
	Idle time.Duration

	Now func() time.Time
}

// ErrUnpaired means the player does not know this remote. Retrying cannot
// help: someone has to pair again.
var ErrUnpaired = errors.New("itunes: the player does not know this remote; pair again")

// Run follows the player until ctx ends, reconnecting whenever the connection
// is lost. It returns only for ctx or for ErrUnpaired.
func (d *Driver) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		started := d.now()
		err := d.follow(ctx)
		d.Sink.Online(false)

		if ctx.Err() != nil {
			return ctx.Err()
		}
		var refused *dacp.StatusError
		if errors.As(err, &refused) && refused.Code == http.StatusForbidden {
			return ErrUnpaired
		}

		// A session that lasted a while was a real one; start over quickly.
		if d.now().Sub(started) > time.Minute {
			backoff = time.Second
		}
		d.logger().Warn("lost the player", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

// follow is one session: log in, report, and wait for changes until an error.
func (d *Driver) follow(ctx context.Context) error {
	short, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := d.Player.Login(short, d.GUID)
	if err == nil {
		var status dacp.Status
		status, err = d.Player.Now(short)
		if err == nil {
			cancel()
			return d.report(ctx, status)
		}
	}
	cancel()
	return err
}

func (d *Driver) report(ctx context.Context, status dacp.Status) error {
	d.Sink.Online(true)

	var (
		last      *contract.State
		lastAlbum string // what the held cover belongs to
		art       *contract.Art
		changedAt time.Time
	)

	for {
		state := d.toState(status)

		// The moment something a listener would notice happened: a different
		// track, or a different play state. A repeat keeps the earlier time.
		if last == nil || last.State != state.State || trackKey(last) != trackKey(&state) {
			changedAt = d.now()
		}
		state.ChangedAt = changedAt

		// One cover per album, fetched when the album changes. The player's
		// album id says so when it has one; a stream has none, and then the
		// track has to stand in.
		update := Update{}
		if state.Track != nil {
			album := albumKey(status, state)
			if album != lastAlbum {
				art, update.Cover = d.cover(ctx)
				lastAlbum = album
			}
			state.Art = art
		} else {
			art, lastAlbum = nil, ""
		}

		// The player answers several times for one action, each with a new
		// revision and the same content. Only a difference is worth saying.
		if !contract.Same(last, &state) || update.Cover != nil {
			update.State = state
			d.Sink.Publish(update)
			kept := state
			last = &kept
		}

		wait, cancel := context.WithTimeout(ctx, d.idle())
		next, err := d.Player.Wait(wait, status.Revision)
		cancel()
		if err != nil {
			if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
				// Nothing for a long while. Ask outright: a live player
				// answers at once and the wait resumes, a dead one does not.
				check, cancel := context.WithTimeout(ctx, 15*time.Second)
				next, err = d.Player.Now(check)
				cancel()
			}
			if err != nil {
				return err
			}
		}
		if next.Revision == status.Revision {
			// Answered with nothing new. Not expected, and must not spin.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
		status = next
	}
}

// cover fetches the current cover. A player with none, or one that fails to
// hand it over, simply has no cover: nowplayd looks one up instead.
func (d *Driver) cover(ctx context.Context) (*contract.Art, []byte) {
	image, err := d.Player.Artwork(ctx, d.coverSize())
	if err != nil {
		d.logger().Warn("the cover could not be fetched", "error", err)
		return nil, nil
	}
	if len(image) == 0 {
		return nil, nil
	}
	mime := http.DetectContentType(image)
	if len(mime) < 6 || mime[:6] != "image/" {
		d.logger().Warn("the player sent something that is not an image", "type", mime)
		return nil, nil
	}
	sum := sha256.Sum256(image)
	return &contract.Art{SHA256: hex.EncodeToString(sum[:]), MIME: mime}, image
}

func (d *Driver) toState(s dacp.Status) contract.State {
	state := contract.State{V: contract.Version, Source: d.Source, Player: d.kind(), State: contract.Stopped}
	switch {
	case s.Playing:
		state.State = contract.Playing
	case s.Paused:
		state.State = contract.Paused
	}

	if s.Title == "" && s.Artist == "" && s.Album == "" {
		// Nothing loaded.
		return state
	}
	state.Track = &contract.Track{Title: s.Title, Artist: s.Artist, Album: s.Album, DurationMS: int64(s.DurationMS)}
	if s.DurationMS > 0 {
		position := int64(s.PositionMS())
		state.PositionMS = &position
	}
	return state
}

func trackKey(s *contract.State) string {
	if s == nil || s.Track == nil {
		return ""
	}
	return s.Track.Key()
}

func albumKey(s dacp.Status, state contract.State) string {
	if s.AlbumID != 0 {
		return "id:" + hex.EncodeToString([]byte{
			byte(s.AlbumID >> 56), byte(s.AlbumID >> 48), byte(s.AlbumID >> 40), byte(s.AlbumID >> 32),
			byte(s.AlbumID >> 24), byte(s.AlbumID >> 16), byte(s.AlbumID >> 8), byte(s.AlbumID)})
	}
	return "track:" + state.Track.Key()
}

func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Driver) idle() time.Duration {
	if d.Idle > 0 {
		return d.Idle
	}
	return 5 * time.Minute
}

func (d *Driver) coverSize() int {
	if d.CoverSize > 0 {
		return d.CoverSize
	}
	return 600
}

func (d *Driver) kind() string {
	if d.Kind != "" {
		return d.Kind
	}
	return "itunes"
}

func (d *Driver) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}
