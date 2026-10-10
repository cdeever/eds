package itunes

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// script is a player that answers from a list. Now returns the first status;
// each Wait returns the next, and when the list runs out, the given error.
type script struct {
	mu       sync.Mutex
	statuses []dacp.Status
	next     int
	end      error
	loginErr error

	covers     map[uint64][]byte // by album id
	coverErr   error
	coverCalls int
	logins     int
	playing    uint64 // album id of the status last handed out
}

func (s *script) Login(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logins++
	return s.loginErr
}

func (s *script) hand(ctx context.Context) (dacp.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.statuses) {
		if s.end != nil {
			return dacp.Status{}, s.end
		}
		s.mu.Unlock()
		<-ctx.Done()
		s.mu.Lock()
		return dacp.Status{}, ctx.Err()
	}
	st := s.statuses[s.next]
	s.next++
	s.playing = st.AlbumID
	return st, nil
}

func (s *script) Now(ctx context.Context) (dacp.Status, error)            { return s.hand(ctx) }
func (s *script) Wait(ctx context.Context, _ uint32) (dacp.Status, error) { return s.hand(ctx) }

func (s *script) Artwork(context.Context, int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coverCalls++
	if s.coverErr != nil {
		return nil, s.coverErr
	}
	return s.covers[s.playing], nil
}

type sink struct {
	mu      sync.Mutex
	updates []Update
	online  []bool
}

func (s *sink) Online(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = append(s.online, v)
}

func (s *sink) Publish(u Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, u)
}

var (
	abbaCover      = []byte("\xff\xd8\xff\xe0-abba-greatest-hits")
	jailbreakCover = []byte("\xff\xd8\xff\xe0-74-jailbreak")
	voltageCover   = []byte("\x89PNG\r\n\x1a\n-high-voltage")
)

const (
	abba      = uint64(101)
	jailbreak = uint64(202)
	voltage   = uint64(303)
)

func st(rev uint32, playing, paused bool, artist, title, album string, albumID uint64, remaining uint32) dacp.Status {
	return dacp.Status{Revision: rev, Playing: playing, Paused: paused, Artist: artist, Title: title,
		Album: album, AlbumID: albumID, DurationMS: 231000, RemainingMS: remaining}
}

// run plays a script to its end and returns what the driver published.
func run(t *testing.T, p *script) *sink {
	t.Helper()
	out := &sink{}
	p.end = &dacp.StatusError{Code: http.StatusForbidden, Status: "403 Forbidden"} // ends Run cleanly
	at := time.Date(2026, 10, 10, 17, 21, 0, 0, time.UTC)
	d := &Driver{Source: "itunes-den", GUID: "X", Player: p, Sink: out, Logger: quiet,
		Now: func() time.Time { at = at.Add(time.Second); return at }}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Run(ctx); !errors.Is(err, ErrUnpaired) {
		t.Fatalf("Run ended with %v", err)
	}
	return out
}

// The session recorded against iTunes 12.13.10.3 on 2026-10-10, as it came:
// several answers per action, with the revision jumping.
func TestTheRecordedSession(t *testing.T) {
	p := &script{
		covers: map[uint64][]byte{abba: abbaCover, jailbreak: jailbreakCover, voltage: voltageCover},
		statuses: []dacp.Status{
			{Revision: 9},  // stopped, nothing loaded
			{Revision: 10}, // again
			st(17, true, false, "Abba", "Dancing Queen", "Abba Greatest Hits Vol. 2", abba, 231000),
			st(18, true, false, "Abba", "Does Your Mother Know", "Abba Greatest Hits Vol. 2", abba, 231000),
			st(22, true, false, "Abba", "Does Your Mother Know", "Abba Greatest Hits Vol. 2", abba, 231000),
			st(23, false, true, "Abba", "Does Your Mother Know", "Abba Greatest Hits Vol. 2", abba, 229000),
			st(24, false, true, "Abba", "Does Your Mother Know", "Abba Greatest Hits Vol. 2", abba, 229000),
			st(27, true, false, "AC/DC", "Baby, Please Don't Go", "'74 Jailbreak", jailbreak, 231000),
			st(30, true, false, "AC/DC", "Baby, Please Don't Go", "'74 Jailbreak", jailbreak, 231000),
			st(39, true, false, "AC/DC", "It's A Long Way To The Top", "High Voltage", voltage, 231000),
			st(40, false, true, "AC/DC", "It's A Long Way To The Top", "High Voltage", voltage, 226000),
			st(41, false, true, "AC/DC", "It's A Long Way To The Top", "High Voltage", voltage, 226000),
		},
	}
	out := run(t, p)

	type want struct {
		state contract.PlayState
		title string
		cover []byte // the cover published with this update, or nil
	}
	wants := []want{
		{contract.Stopped, "", nil},
		{contract.Playing, "Dancing Queen", abbaCover},
		{contract.Playing, "Does Your Mother Know", nil}, // same album: no new cover
		{contract.Paused, "Does Your Mother Know", nil},
		{contract.Playing, "Baby, Please Don't Go", jailbreakCover},
		{contract.Playing, "It's A Long Way To The Top", voltageCover},
		{contract.Paused, "It's A Long Way To The Top", nil},
	}

	// Twelve answers, seven things that happened.
	if len(out.updates) != len(wants) {
		for i, u := range out.updates {
			t.Logf("update %d: %s %v", i, u.State.State, u.State.Track)
		}
		t.Fatalf("published %d updates, want %d", len(out.updates), len(wants))
	}
	for i, w := range wants {
		u := out.updates[i]
		title := ""
		if u.State.Track != nil {
			title = u.State.Track.Title
		}
		if u.State.State != w.state || title != w.title {
			t.Errorf("update %d: %s %q, want %s %q", i, u.State.State, title, w.state, w.title)
		}
		if string(u.Cover) != string(w.cover) {
			t.Errorf("update %d (%q): cover of %d bytes, want %d", i, title, len(u.Cover), len(w.cover))
		}
		if err := u.State.Validate(); err != nil {
			t.Errorf("update %d would be refused on the bus: %v", i, err)
		}
		if u.State.Source != "itunes-den" || u.State.Player != "itunes" {
			t.Errorf("update %d is from %q/%q", i, u.State.Source, u.State.Player)
		}
	}

	if p.coverCalls != 3 {
		t.Errorf("the cover was fetched %d times for three albums", p.coverCalls)
	}

	// Every state that has a track names its cover, including the ones that
	// did not carry the image: the next track on the album, and the pause.
	for i, u := range out.updates[1:] {
		if u.State.Art == nil || len(u.State.Art.SHA256) != 64 {
			t.Errorf("update %d has a track but names no cover", i+1)
		}
	}
	if out.updates[1].State.Art.SHA256 != out.updates[2].State.Art.SHA256 {
		t.Error("two tracks on one album name different covers")
	}
	if out.updates[2].State.Art.SHA256 == out.updates[4].State.Art.SHA256 {
		t.Error("two albums name the same cover")
	}
	if out.updates[1].State.Art.MIME != "image/jpeg" || out.updates[5].State.Art.MIME != "image/png" {
		t.Errorf("mime types: %q, %q", out.updates[1].State.Art.MIME, out.updates[5].State.Art.MIME)
	}
	if out.updates[0].State.Track != nil || out.updates[0].State.Art != nil {
		t.Error("a stopped player with nothing loaded was given a track or a cover")
	}
}

// changed_at is when something a listener would notice happened. A repeat,
// or progress within a track, must not move it - the arbiter believes it at
// start-up.
func TestChangedAtMovesOnlyWhenSomethingHappens(t *testing.T) {
	p := &script{statuses: []dacp.Status{
		st(1, true, false, "A", "One", "Album", abba, 231000),
		st(2, true, false, "A", "One", "Album", abba, 200000), // progress
		st(3, false, true, "A", "One", "Album", abba, 200000), // paused
		st(4, true, false, "A", "Two", "Album", abba, 231000), // next track
	}}
	out := run(t, p)

	if len(out.updates) != 3 {
		t.Fatalf("published %d updates, want 3: progress alone is not one", len(out.updates))
	}
	a, b, c := out.updates[0].State.ChangedAt, out.updates[1].State.ChangedAt, out.updates[2].State.ChangedAt
	if !b.After(a) || !c.After(b) {
		t.Errorf("changed_at did not advance with each change: %s %s %s", a, b, c)
	}
}

func TestPositionIsReported(t *testing.T) {
	p := &script{statuses: []dacp.Status{st(1, true, false, "A", "One", "Album", abba, 201000)}}
	out := run(t, p)
	u := out.updates[0].State
	if u.PositionMS == nil || *u.PositionMS != 30000 || u.Track.DurationMS != 231000 {
		t.Errorf("position %v of %d", u.PositionMS, u.Track.DurationMS)
	}
}

// A player with no cover for a track, or one that fails to hand it over, just
// has none. The state still goes out, and nowplayd looks one up.
func TestAMissingCoverDoesNotHoldTheStateBack(t *testing.T) {
	for name, p := range map[string]*script{
		"none":            {covers: map[uint64][]byte{}},
		"fetch failed":    {coverErr: errors.New("connection reset")},
		"not an image":    {covers: map[uint64][]byte{abba: []byte("<html>no</html>")}},
		"an empty answer": {covers: map[uint64][]byte{abba: {}}},
	} {
		t.Run(name, func(t *testing.T) {
			p.statuses = []dacp.Status{st(1, true, false, "A", "One", "Album", abba, 231000)}
			out := run(t, p)
			if len(out.updates) != 1 {
				t.Fatalf("published %d updates", len(out.updates))
			}
			if u := out.updates[0]; u.Cover != nil || u.State.Art != nil || u.State.Track.Title != "One" {
				t.Errorf("update = %+v", u.State)
			}
		})
	}
}

// A stream has no album id, so the track stands in for the album.
func TestWithNoAlbumIDEachTrackGetsItsOwnCover(t *testing.T) {
	p := &script{covers: map[uint64][]byte{0: abbaCover}, statuses: []dacp.Status{
		st(1, true, false, "Station", "Song One", "", 0, 0),
		st(2, true, false, "Station", "Song Two", "", 0, 0),
		st(3, true, false, "Station", "Song Two", "", 0, 0),
	}}
	run(t, p)
	if p.coverCalls != 2 {
		t.Errorf("the cover was fetched %d times for two songs", p.coverCalls)
	}
}

func TestOnlineMeansASessionWithThePlayer(t *testing.T) {
	out := run(t, &script{statuses: []dacp.Status{st(1, true, false, "A", "One", "Album", abba, 231000)}})
	if len(out.online) != 2 || !out.online[0] || out.online[1] {
		t.Errorf("online = %v, want true then false", out.online)
	}
}

// A player that refuses the pairing is not coming back by being asked again.
func TestAnUnknownRemoteStopsTheDriver(t *testing.T) {
	p := &script{loginErr: &dacp.StatusError{Code: http.StatusForbidden, Status: "403 Forbidden"}}
	out := &sink{}
	d := &Driver{Source: "itunes", GUID: "X", Player: p, Sink: out, Logger: quiet}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Run(ctx); !errors.Is(err, ErrUnpaired) {
		t.Errorf("Run ended with %v", err)
	}
	if p.logins != 1 {
		t.Errorf("logged in %d times against a player that refused", p.logins)
	}
	for _, v := range out.online {
		if v {
			t.Error("reported online without ever having a session")
		}
	}
}

// Anything else - the PC asleep, iTunes closed - is worth trying again.
func TestALostPlayerIsRetried(t *testing.T) {
	p := &script{loginErr: errors.New("connection refused")}
	d := &Driver{Source: "itunes", GUID: "X", Player: p, Sink: &sink{}, Logger: quiet}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := d.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run ended with %v, want it to keep trying until told to stop", err)
	}
	if p.logins < 2 {
		t.Errorf("tried %d times in 1.5 s, want a retry", p.logins)
	}
}

// A quiet evening and a dead PC look the same on a hanging request. After
// long enough the driver asks outright, and carries on if it is answered.
func TestAfterALongSilenceThePlayerIsAskedOutright(t *testing.T) {
	p := &quietPlayer{status: st(1, true, false, "A", "One", "Album", abba, 231000)}
	out := &sink{}
	d := &Driver{Source: "itunes", GUID: "X", Player: p, Sink: out, Logger: quiet, Idle: 100 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	_ = d.Run(ctx)

	if p.nows < 3 {
		t.Errorf("asked outright %d times in 450 ms of silence with a 100 ms limit", p.nows)
	}
	if len(out.updates) != 1 {
		t.Errorf("published %d updates: asking again is not a change", len(out.updates))
	}
}

type quietPlayer struct {
	status dacp.Status
	mu     sync.Mutex
	nows   int
}

func (q *quietPlayer) Login(context.Context, string) error { return nil }
func (q *quietPlayer) Now(context.Context) (dacp.Status, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nows++
	// A new revision each time, as a live player gives, so the driver does
	// not take the answer for a stuck one.
	q.status.Revision++
	return q.status, nil
}
func (q *quietPlayer) Wait(ctx context.Context, _ uint32) (dacp.Status, error) {
	<-ctx.Done()
	return dacp.Status{}, ctx.Err()
}
func (q *quietPlayer) Artwork(context.Context, int) ([]byte, error) { return nil, nil }
