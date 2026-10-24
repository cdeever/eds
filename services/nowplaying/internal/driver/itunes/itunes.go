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
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
)

// Player is the part of the protocol client the driver uses.
type Player interface {
	Login(ctx context.Context, guid string) error
	Now(ctx context.Context) (dacp.Status, error)
	Wait(ctx context.Context, revision uint32) (dacp.Status, error)
	Artwork(ctx context.Context, size int) ([]byte, error)

	// The remote's buttons (ADR-0020).
	PlayPause(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	Volume(ctx context.Context) (int, error)
	SetVolume(ctx context.Context, volume int) error

	// What the remote sends to play an album (ADR-0040).
	PlayAlbum(ctx context.Context, artist, album string) error
}

// Driver follows one player.
type Driver struct {
	Source string // the source id this driver reports as
	Kind   string // "itunes" or "music"
	GUID   string // the pairing
	Player Player
	Sink   driver.Sink
	Logger *slog.Logger

	// CoverSize is the largest cover asked for, in pixels. palette works
	// from 160; 600 leaves room for anything else that wants the picture.
	CoverSize int
	// Idle is how long the player may say nothing before the connection is
	// assumed dead. The player says nothing for the length of a track, and
	// also when it has lost power; only time tells those apart.
	Idle time.Duration

	Now func() time.Time

	// AlbumCheck is how long an album is given to start before the player
	// is asked what plays; zero is albumCheck.
	AlbumCheck time.Duration

	// DuckFor is how long a duck lasts if nothing ends it; zero is
	// DuckFor's default (ADR-0044).
	DuckFor time.Duration

	// Whether there is a session a command could be sent on. Commands arrive
	// from the bus, on a goroutine that is not the one following the player.
	mu   sync.Mutex
	live bool

	// The volume a duck took the player down from, and what ends it if
	// nothing else does. ducking is false when the music is as it was.
	ducking bool
	ducked  int
	undo    *time.Timer
}

// DuckTo is how far a duck turns the music down: to a quarter of what it was.
const DuckTo = 4

// duckFor is how long a duck lasts if nothing ends it: past any request.
const duckFor = 10 * time.Second

// VolumeStep is how far one volume command moves the player, of 100. Twenty
// presses from silence to full: fine enough to settle on a level, coarse
// enough that a press is heard.
const VolumeStep = 5

// ErrNoSession means a command arrived while the driver had no session with
// its player. It is not kept for later: a "next" that lands a minute after
// it was pressed is a fault of its own.
var ErrNoSession = errors.New("itunes: no session with the player")

// Command does what a controller asked of this player (ADR-0020). It may be
// called from any goroutine, while Run is following the player.
//
// The driver says nothing on the bus about having done it. The player
// reports the change it caused the way it reports any other, and that report
// is the acknowledgment.
func (d *Driver) Command(ctx context.Context, c contract.Command) error {
	d.mu.Lock()
	live := d.live
	d.mu.Unlock()
	if !live {
		return ErrNoSession
	}

	switch c.Command {
	case contract.PlayPause:
		return d.Player.PlayPause(ctx)
	case contract.Next:
		return d.Player.Next(ctx)
	case contract.Previous:
		return d.Player.Previous(ctx)
	case contract.VolumeUp:
		d.forgetDuck() // whoever pressed it has the volume now
		return d.nudge(ctx, VolumeStep)
	case contract.VolumeDown:
		d.forgetDuck()
		return d.nudge(ctx, -VolumeStep)
	case contract.PlayAlbum:
		// The album starts at the volume the room had, not the ducked one.
		if err := d.unduck(ctx); err != nil {
			d.logger().Warn("the volume could not be brought back before an album", "error", err)
		}
		return d.playFirstOwned(ctx, c.Artist, append([]string{c.Album}, c.Instead...))
	case contract.Duck:
		return d.duck(ctx)
	case contract.Unduck:
		return d.unduck(ctx)
	}
	return fmt.Errorf("itunes: no such command %q", c.Command)
}

// playFirstOwned plays the first of the albums the library has (ADR-0045):
// the one asked for, then each instead, in order. A miss is only found by
// trying, and each try empties the queue, so a miss on all of them leaves
// the player stopped.
func (d *Driver) playFirstOwned(ctx context.Context, artist string, albums []string) error {
	var missed []string
	for i, album := range albums {
		err := d.playAlbum(ctx, artist, album)
		if err == nil {
			if i > 0 {
				d.logger().Info("the library had not got what was asked for; played the next it has",
					"artist", artist, "asked", albums[0], "played", album, "missed", missed)
			}
			return nil
		}
		if !errors.Is(err, ErrAlbumNotFound) {
			return err
		}
		missed = append(missed, album)
	}
	return fmt.Errorf("%w: tried %q by %q", ErrAlbumNotFound, missed, artist)
}

// ErrAlbumNotFound means the player was asked for an album and is not
// playing it: its library names it differently, or does not have it.
var ErrAlbumNotFound = errors.New("itunes: the library played no album by those names")

// albumCheck is how long the player is given to start an album before it is
// asked what it is playing. iTunes for Windows had on 2026-10-23 within one
// second.
const albumCheck = 1500 * time.Millisecond

// playAlbum asks for an album by name and then asks what plays, because the
// player answers a request that matched nothing just as it answers one that
// matched (ADR-0040). The match is the player's; nothing nearer is tried.
func (d *Driver) playAlbum(ctx context.Context, artist, album string) error {
	if err := d.Player.PlayAlbum(ctx, artist, album); err != nil {
		return err
	}
	wait := d.AlbumCheck
	if wait == 0 {
		wait = albumCheck
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	now, err := d.Player.Now(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(now.Artist, artist) || !strings.EqualFold(now.Album, album) {
		return fmt.Errorf("%w: asked for %q by %q, playing %q by %q", ErrAlbumNotFound, album, artist, now.Album, now.Artist)
	}
	return nil
}

// nudge moves the volume by step. The player is asked where it is first,
// because it is the only one that knows: the slider in its own window moves
// it too.
func (d *Driver) nudge(ctx context.Context, step int) error {
	volume, err := d.Player.Volume(ctx)
	if err != nil {
		return err
	}
	to := min(max(volume+step, 0), 100)
	if to == volume {
		return nil // already as far as it goes
	}
	return d.Player.SetVolume(ctx, to)
}

// duck turns the music down while a request is heard (ADR-0044), and sets
// it to come back by itself. A duck while ducked lowers nothing further: it
// only waits again.
func (d *Driver) duck(ctx context.Context) error {
	d.mu.Lock()
	already := d.ducking
	d.mu.Unlock()
	if !already {
		volume, err := d.Player.Volume(ctx)
		if err != nil {
			return err
		}
		if err := d.Player.SetVolume(ctx, volume/DuckTo); err != nil {
			return err
		}
		d.mu.Lock()
		d.ducking, d.ducked = true, volume
		d.mu.Unlock()
	}

	wait := d.DuckFor
	if wait == 0 {
		wait = duckFor
	}
	d.mu.Lock()
	if d.undo != nil {
		d.undo.Stop()
	}
	d.undo = time.AfterFunc(wait, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.unduck(ctx); err != nil {
			d.logger().Warn("the volume could not be brought back after a duck", "error", err)
		}
	})
	d.mu.Unlock()
	return nil
}

// unduck brings back the volume a duck took down. Not ducked, it does
// nothing.
func (d *Driver) unduck(ctx context.Context) error {
	d.mu.Lock()
	if !d.ducking {
		d.mu.Unlock()
		return nil
	}
	to := d.ducked
	d.ducking = false
	if d.undo != nil {
		d.undo.Stop()
		d.undo = nil
	}
	d.mu.Unlock()
	return d.Player.SetVolume(ctx, to)
}

// forgetDuck ends a duck without bringing anything back.
func (d *Driver) forgetDuck() {
	d.mu.Lock()
	d.ducking = false
	if d.undo != nil {
		d.undo.Stop()
		d.undo = nil
	}
	d.mu.Unlock()
}

func (d *Driver) setLive(live bool) {
	d.mu.Lock()
	d.live = live
	d.mu.Unlock()
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
		d.setLive(false)
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
	d.setLive(true)
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
		update := driver.Update{}
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
