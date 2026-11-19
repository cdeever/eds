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
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/library"
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

	// What the remote sends to play an album (ADR-0040), a song or an
	// artist (ADR-0055), and its shuffle button.
	PlayAlbum(ctx context.Context, artist, album string) error
	PlayTrack(ctx context.Context, artist, title string) error
	PlayArtist(ctx context.Context, artist string, index int) error
	SetShuffle(ctx context.Context, on bool) error
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

	// The artist the driver is shuffling (ADR-0055), or nil. The player's
	// own shuffle is the whole library's, so the driver jumps to a random
	// song of theirs each time one ends.
	shuffling *artistShuffle

	// Shuffle orders n songs; nil is a random order.
	Shuffle func(n int) []int
}

// artistShuffle is an artist's songs being played in an order of the
// driver's: their places in the player's queue of the artist, by album.
type artistShuffle struct {
	artist string
	deck   []int  // places still to play, in order
	at     string // the song jumped to; "" while a jump is under way
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
		d.beforeChoosing(ctx)
		return d.playFirstOwned(ctx, c.Artist, append([]string{c.Album}, c.Instead...))
	case contract.PlayTrack:
		d.beforeChoosing(ctx)
		return d.playTrack(ctx, c.Artist, c.Title)
	case contract.PlayArtist:
		d.beforeChoosing(ctx)
		return d.playArtist(ctx, c.Artist, c.Songs)
	case contract.Duck:
		return d.duck(ctx)
	case contract.Unduck:
		return d.unduck(ctx)
	}
	return fmt.Errorf("itunes: no such command %q", c.Command)
}

// beforeChoosing readies the player for what is chosen to play: at the
// volume the room had, not the ducked one (ADR-0044); with the player's own
// shuffle off, which shuffles the whole library and not what was chosen
// (ADR-0055, 2026-11-11); and with any shuffle of the driver's ended.
// Neither failing stops what was asked for.
func (d *Driver) beforeChoosing(ctx context.Context) {
	d.endShuffle(nil)
	if err := d.unduck(ctx); err != nil {
		d.logger().Warn("the volume could not be brought back before choosing what plays", "error", err)
	}
	if err := d.Player.SetShuffle(ctx, false); err != nil {
		d.logger().Warn("the player's shuffle could not be turned off", "error", err)
	}
}

// ErrTrackNotFound and ErrArtistNotFound mean the player was asked for a
// song or an artist and is not playing it (ADR-0055).
var (
	ErrTrackNotFound  = errors.New("itunes: the library played no song by that name")
	ErrArtistNotFound = errors.New("itunes: the library played nothing by that artist")
)

// playTrack asks for the artist's songs whose title begins with the one
// asked, and then whether what plays is it, with at most an edition's note
// after its title ("Come Together - Remastered 2009").
func (d *Driver) playTrack(ctx context.Context, artist, title string) error {
	if err := d.Player.PlayTrack(ctx, artist, dacp.Stem(title)); err != nil {
		return err
	}
	now, ok, err := d.until(ctx, func(now dacp.Status) bool {
		return strings.EqualFold(now.Artist, artist) && begins(now.Title, title)
	})
	if err != nil || ok {
		return err
	}
	return fmt.Errorf("%w: asked for %q by %q, playing %q by %q", ErrTrackNotFound, title, artist, now.Title, now.Artist)
}

// playArtist plays the artist's songs: shuffled by the driver among the
// songs the library says they have, or in order when it does not say.
func (d *Driver) playArtist(ctx context.Context, artist string, songs int) error {
	if songs <= 0 {
		if err := d.Player.PlayArtist(ctx, artist, 0); err != nil {
			return err
		}
		now, ok, err := d.until(ctx, func(now dacp.Status) bool { return strings.EqualFold(now.Artist, artist) })
		if err != nil || ok {
			return err
		}
		return fmt.Errorf("%w: asked for %q, playing %q", ErrArtistNotFound, artist, now.Artist)
	}
	order := d.Shuffle
	if order == nil {
		order = rand.Perm
	}
	s := &artistShuffle{artist: artist, deck: order(songs)}
	d.mu.Lock()
	d.shuffling = s
	d.mu.Unlock()
	return d.jump(ctx, s)
}

// jump plays the next of a shuffle's songs. A place past the end of the
// artist's songs - the library counted more than the player has under
// their name - is not theirs, and nor is any after it: those are dropped
// and the next tried. iTunes for Windows answers such a place with a 500
// (2026-11-11, 73 counted for Van Halen).
//
// It has landed on a song of theirs that is not the one playing before the
// jump: for a moment after it is asked, the player still says that one,
// and taking it for the landing made each song that followed look like
// the next (2026-11-11, a song a second through two albums).
func (d *Driver) jump(ctx context.Context, s *artistShuffle) error {
	// What was playing, asked briefly: a player slow to say is no reason
	// not to jump, only to take the first song of theirs heard for the
	// landing.
	ask, cancel := context.WithTimeout(ctx, time.Second)
	before, _ := d.Player.Now(ask)
	cancel()
	from := songKey(before)
	for {
		d.mu.Lock()
		if d.shuffling != s {
			d.mu.Unlock()
			return nil // something else was chosen meanwhile
		}
		if len(s.deck) == 0 {
			d.shuffling = nil // every song played: the player goes on in order
			d.mu.Unlock()
			return nil
		}
		i := s.deck[0]
		s.deck, s.at = s.deck[1:], ""
		d.mu.Unlock()

		err := d.Player.PlayArtist(ctx, s.artist, i)
		var refused *dacp.StatusError
		past := errors.As(err, &refused) && refused.Code >= 500
		if err != nil && !past {
			d.endShuffle(s)
			return err
		}
		var now dacp.Status
		ok := false
		if !past {
			theirs := func(now dacp.Status) bool { return strings.EqualFold(now.Artist, s.artist) }
			now, ok, err = d.until(ctx, func(now dacp.Status) bool { return theirs(now) && songKey(now) != from })
			if err != nil {
				d.endShuffle(s)
				return err
			}
			// The place was the very song playing before: it is still theirs.
			ok = ok || (theirs(now) && songKey(now) == from)
		}
		if ok {
			d.mu.Lock()
			s.at = songKey(now)
			d.mu.Unlock()
			return nil
		}
		if i == 0 {
			d.endShuffle(s)
			return fmt.Errorf("%w: asked for %q, playing %q", ErrArtistNotFound, s.artist, now.Artist)
		}
		d.mu.Lock()
		kept := s.deck[:0]
		for _, j := range s.deck {
			if j < i {
				kept = append(kept, j)
			}
		}
		s.deck = kept
		d.mu.Unlock()
		d.logger().Info("fewer songs by the artist than the library counted", "artist", s.artist, "past", i)
	}
}

// endShuffle ends the driver's shuffle: s, or whichever is going for nil.
func (d *Driver) endShuffle(s *artistShuffle) {
	d.mu.Lock()
	if s == nil || d.shuffling == s {
		d.shuffling = nil
	}
	d.mu.Unlock()
}

// onSong is told each status the player reports. While the driver
// shuffles, another song of the artist's - the next in the player's own
// order, or Next pressed - is the cue for the next of the shuffle's; a song
// of anyone else's, or nothing playing, ends it.
func (d *Driver) onSong(ctx context.Context, now dacp.Status) {
	d.mu.Lock()
	s := d.shuffling
	if s == nil || s.at == "" {
		d.mu.Unlock()
		return // none, or a jump under way
	}
	key := songKey(now)
	switch {
	case key == s.at:
		d.mu.Unlock()
		return // the same song: paused, resumed, or said again
	case now.Title == "" || !strings.EqualFold(now.Artist, s.artist):
		d.shuffling = nil
		d.mu.Unlock()
		d.logger().Info("the shuffle ended: something else plays", "artist", s.artist)
		return
	}
	s.at = "" // jumping
	d.mu.Unlock()
	go func() {
		if err := d.jump(ctx, s); err != nil && ctx.Err() == nil {
			d.logger().Warn("the shuffle could not go on", "artist", s.artist, "error", err)
		}
	}()
}

func songKey(s dacp.Status) string {
	return s.Artist + "\x00" + s.Album + "\x00" + s.Title
}

// until asks the player what it plays until it is what was asked for or
// the wait is out, and says what it last heard: as soon as it might have
// started, and again until the wait is out. What is there is known in a few
// tenths of a second, what is not only when the wait is.
func (d *Driver) until(ctx context.Context, asked func(dacp.Status) bool) (dacp.Status, bool, error) {
	wait := d.AlbumCheck
	if wait == 0 {
		wait = albumCheck
	}
	deadline := time.Now().Add(wait)
	var now dacp.Status
	for {
		step := min(wait/6, time.Until(deadline))
		select {
		case <-ctx.Done():
			return now, false, ctx.Err()
		case <-time.After(max(step, 0)):
		}
		var err error
		if now, err = d.Player.Now(ctx); err != nil {
			return now, false, err
		}
		if asked(now) {
			return now, true, nil
		}
		if !time.Now().Before(deadline) {
			return now, false, nil
		}
	}
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
	// The whole title first: with the owner's library known, it is the
	// library's own (ADR-0046). Then its plain beginning, for a title a
	// catalog spelt and the library may have spelt otherwise.
	starts := []string{dacp.Stem(album)}
	if q := dacp.QueryStem(album); q != starts[0] {
		starts = append(starts, q)
	}
	var err error
	for _, start := range starts {
		if err = d.tryAlbum(ctx, artist, album, start); !errors.Is(err, ErrAlbumNotFound) {
			return err
		}
	}
	return err
}

// tryAlbum asks for the albums whose title begins with start, and says
// whether what then plays is album.
func (d *Driver) tryAlbum(ctx context.Context, artist, album, start string) error {
	if err := d.Player.PlayAlbum(ctx, artist, start); err != nil {
		return err
	}
	now, ok, err := d.until(ctx, func(now dacp.Status) bool {
		return strings.EqualFold(now.Artist, artist) && begins(now.Album, album)
	})
	if err != nil || ok {
		return err
	}
	return fmt.Errorf("%w: asked for %q by %q, playing %q by %q", ErrAlbumNotFound, album, artist, now.Album, now.Artist)
}

// begins is whether a library's title is the one asked for, alone or with
// an edition's note after it: "Led Zeppelin II (Remastered)" is "Led
// Zeppelin II", and "And the Word Was..." is "And the Word Was…". More of a
// name is not a note: "Van Halen II" is not "Van Halen", which the player's
// match by the start of a title would otherwise play for it.
//
// Letters and digits are compared, without case or accents, so that
// "Pronounced leh-nerd skin-nerd" is "(pronounced 'lĕh-'nérd 'skin-'nérd)".
func begins(title, asked string) bool {
	want := fold(dacp.Stem(asked))
	if want == "" {
		return false
	}
	// Walk the title until as many of its letters and digits as the asked
	// title has are matched, and see what follows the last of them.
	matched, end := 0, -1
	for i, r := range title {
		f := library.FoldRune(r)
		if f == 0 {
			continue
		}
		if f != rune(want[matched]) {
			return false
		}
		matched++
		if matched == len(want) {
			end = i + utf8.RuneLen(r)
			break
		}
	}
	if end < 0 {
		return false
	}
	rest := strings.TrimSpace(title[end:])
	if rest == "" {
		return true
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return strings.ContainsRune("([{-–—:…./,'\"", first) && !continues(rest)
}

// continues is whether what follows a matched title is more of a name after
// all, behind a quote or a stop: "Sgt. Pepper's" has more letters after its
// apostrophe, and an asked "Sgt" is not it. A note is spaced or bracketed.
func continues(rest string) bool {
	if len(rest) < 2 {
		return false
	}
	first, size := utf8.DecodeRuneInString(rest)
	next, _ := utf8.DecodeRuneInString(rest[size:])
	return strings.ContainsRune("'\".", first) && library.FoldRune(next) != 0
}

// fold is a title's letters and digits, lowercased and without accents.
func fold(s string) string {
	var b strings.Builder
	for _, r := range s {
		if f := library.FoldRune(r); f != 0 {
			b.WriteRune(f)
		}
	}
	return b.String()
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
		d.onSong(ctx, status)
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
