package current

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

var (
	pngA = []byte("\x89PNG\r\n\x1a\n-cover-A")
	pngB = []byte("\x89PNG\r\n\x1a\n-cover-B")
	jpeg = []byte("\xff\xd8\xff\xe0-a-jpeg-from-a-catalogue")
)

type published struct {
	topic    string
	payload  []byte
	retained bool
}

type fakeBus struct {
	sent []published
	err  error
}

func (f *fakeBus) Publish(topic string, payload []byte, retained bool) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, published{topic, append([]byte(nil), payload...), retained})
	return nil
}

func (f *fakeBus) take() []published {
	out := f.sent
	f.sent = nil
	return out
}

type fakeResolver struct {
	result resolve.Result
	err    error
	asked  []contract.Track
}

func (f *fakeResolver) Resolve(_ context.Context, t contract.Track) (resolve.Result, error) {
	f.asked = append(f.asked, t)
	return f.result, f.err
}

var at = time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC)

func publisher(bus *fakeBus, r Resolver) *Publisher {
	return &Publisher{Bus: bus, Resolver: r, Prefix: "eds", Now: func() time.Time { return at }}
}

func playing(title, album string) *contract.State {
	return &contract.State{V: 1, Source: "itunes-den", Player: "itunes", State: contract.Playing,
		Track: &contract.Track{Title: title, Artist: "Artist", Album: album}, ChangedAt: at}
}

func decode(t *testing.T, p published) contract.State {
	t.Helper()
	st, err := contract.Decode(p.payload)
	if err != nil {
		t.Fatalf("published something that is not a state: %v\n%s", err, p.payload)
	}
	return st
}

// The whole promise: something is playing, so current has a title, an album
// and art - here from the player itself, with no catalogue asked.
func TestThePlayersOwnCoverIsUsedAndNothingIsLookedUp(t *testing.T) {
	bus, res := &fakeBus{}, &fakeResolver{err: resolve.ErrNotFound}
	cover := NewImage(pngA)
	st := playing("So What", "Kind of Blue")
	st.Art = &contract.Art{SHA256: cover.SHA256}

	if _, _, err := publisher(bus, res).Publish(context.Background(), st, cover); err != nil {
		t.Fatal(err)
	}

	sent := bus.take()
	if len(sent) != 2 {
		t.Fatalf("published %d messages, want the cover then the state", len(sent))
	}
	// Cover first: a subscriber that reads the state must find the image it
	// names already there.
	if sent[0].topic != "eds/nowplaying/current/art" || string(sent[0].payload) != string(pngA) || !sent[0].retained {
		t.Errorf("first message = %s (%d bytes, retained=%v)", sent[0].topic, len(sent[0].payload), sent[0].retained)
	}
	if sent[1].topic != "eds/nowplaying/current" || !sent[1].retained {
		t.Errorf("second message = %s retained=%v", sent[1].topic, sent[1].retained)
	}
	got := decode(t, sent[1])
	if got.Track.Title != "So What" || got.Track.Album != "Kind of Blue" {
		t.Errorf("track = %+v", got.Track)
	}
	if got.Art == nil || got.Art.SHA256 != cover.SHA256 || got.Art.From != "player" || got.Art.MIME != "image/png" {
		t.Errorf("art = %+v", got.Art)
	}
	if len(res.asked) != 0 {
		t.Error("a catalogue was asked although the player supplied the cover")
	}
}

func TestACoverIsLookedUpWhenTheSourceHasNone(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "Kind of Blue", Cover: &resolve.Cover{Image: jpeg, MIME: "image/jpeg", From: "deezer"}}}

	if _, _, err := publisher(bus, res).Publish(context.Background(), playing("So What", "Kind of Blue"), nil); err != nil {
		t.Fatal(err)
	}

	sent := bus.take()
	if len(sent) != 2 || string(sent[0].payload) != string(jpeg) {
		t.Fatalf("published %d messages", len(sent))
	}
	got := decode(t, sent[1])
	if got.Art == nil || got.Art.From != "deezer" || got.Art.MIME != "image/jpeg" {
		t.Errorf("art = %+v", got.Art)
	}
	if got.Art.SHA256 != NewImage(jpeg).SHA256 {
		t.Error("the hash is not the hash of the image published")
	}
}

// TIDAL through Last.fm arrives with no album at all.
func TestAMissingAlbumIsFilledIn(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "Kind of Blue", Cover: &resolve.Cover{Image: jpeg, From: "itunes"}}}

	if _, _, err := publisher(bus, res).Publish(context.Background(), playing("So What", ""), nil); err != nil {
		t.Fatal(err)
	}
	got := decode(t, bus.take()[1])
	if got.Track.Album != "Kind of Blue" {
		t.Errorf("album = %q", got.Track.Album)
	}
}

// A source that knows its album keeps its own name for it. The catalogue's
// "Kind Of Blue (Legacy Edition)" is not an improvement.
func TestAKnownAlbumIsNotRenamedByACatalogue(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "Kind Of Blue (Legacy Edition)", Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}

	_, _, _ = publisher(bus, res).Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
	if got := decode(t, bus.take()[1]); got.Track.Album != "Kind of Blue" {
		t.Errorf("album = %q", got.Track.Album)
	}
}

// A source may supply the cover but not the album - a locally read TIDAL
// does. The album is looked up; the player's cover still wins.
func TestThePlayersCoverWinsEvenWhenTheAlbumIsLookedUp(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "Kind of Blue", Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}
	cover := NewImage(pngA)
	st := playing("So What", "")
	st.Art = &contract.Art{SHA256: cover.SHA256}

	_, _, _ = publisher(bus, res).Publish(context.Background(), st, cover)
	sent := bus.take()
	got := decode(t, sent[1])
	if got.Track.Album != "Kind of Blue" || got.Art.From != "player" || string(sent[0].payload) != string(pngA) {
		t.Errorf("album %q, art from %q", got.Track.Album, got.Art.From)
	}
}

// A track change and its cover are two messages. For a moment the image on
// hand is the previous track's, and it must not be published as this one's.
func TestACoverThatBelongsToAnotherTrackIsNotUsed(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{err: resolve.ErrNotFound}
	stale := NewImage(pngA)
	st := playing("So What", "Kind of Blue")
	st.Art = &contract.Art{SHA256: NewImage(pngB).SHA256} // names B; only A has arrived

	_, _, _ = publisher(bus, res).Publish(context.Background(), st, stale)

	sent := bus.take()
	if len(sent) != 1 || sent[0].topic != "eds/nowplaying/current" {
		t.Fatalf("published %d messages; the stale cover went out", len(sent))
	}
	if got := decode(t, sent[0]); got.Art != nil {
		t.Errorf("the state names art that was not published: %+v", got.Art)
	}
}

// Published incomplete rather than withheld: a track with no cover is still
// the track that is playing.
func TestWhenNothingHasTheCoverTheTrackIsStillPublished(t *testing.T) {
	for name, err := range map[string]error{
		"not found":      resolve.ErrNotFound,
		"catalogue down": errors.New("connection refused"),
	} {
		t.Run(name, func(t *testing.T) {
			bus := &fakeBus{}
			_, _, _ = publisher(bus, &fakeResolver{err: err}).Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
			sent := bus.take()
			if len(sent) != 1 {
				t.Fatalf("published %d messages, want the state alone", len(sent))
			}
			if got := decode(t, sent[0]); got.Art != nil || got.Track.Title != "So What" {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestWithNoResolverNothingIsLookedUp(t *testing.T) {
	bus := &fakeBus{}
	if _, _, err := publisher(bus, nil).Publish(context.Background(), playing("So What", ""), nil); err != nil {
		t.Fatal(err)
	}
	if sent := bus.take(); len(sent) != 1 {
		t.Errorf("published %d messages", len(sent))
	}
}

// The next track on the same album has the same cover. Publishing it again
// would make lightd repaint a room that is already the right colour.
func TestTheSameCoverIsNotPublishedTwice(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}
	p := publisher(bus, res)

	_, _, _ = p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
	bus.take()

	_, _, _ = p.Publish(context.Background(), playing("Freddie Freeloader", "Kind of Blue"), nil)
	sent := bus.take()
	if len(sent) != 1 || sent[0].topic != "eds/nowplaying/current" {
		t.Fatalf("the next track on the album published %d messages, want the state alone", len(sent))
	}
	if got := decode(t, sent[0]); got.Art == nil || got.Track.Title != "Freddie Freeloader" {
		t.Errorf("got %+v", got)
	}
}

func TestNothingIsPublishedWhenNothingChanged(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, &fakeResolver{err: resolve.ErrNotFound})

	st := playing("So What", "Kind of Blue")
	if _, sent, _ := p.Publish(context.Background(), st, nil); !sent {
		t.Error("the first publish reported that nothing went out")
	}
	bus.take()

	again := *st
	pos := int64(30000)
	again.PositionMS = &pos
	_, sent, _ := p.Publish(context.Background(), &again, nil)
	if sent || len(bus.take()) != 0 {
		t.Error("a position update was published")
	}
}

// What is returned is what went out, so a log line says what subscribers saw.
func TestPublishReturnsTheStateAsPublished(t *testing.T) {
	res := &fakeResolver{result: resolve.Result{Album: "Kind of Blue", Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}
	out, sent, err := publisher(&fakeBus{}, res).Publish(context.Background(), playing("So What", ""), nil)
	if err != nil || !sent {
		t.Fatalf("sent=%v err=%v", sent, err)
	}
	if out.Track.Album != "Kind of Blue" || out.Art == nil || out.Art.From != "deezer" {
		t.Errorf("returned %+v with art %+v", out.Track, out.Art)
	}
}

func TestPausingKeepsTheTrackAndItsCover(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, &fakeResolver{result: resolve.Result{Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}})

	_, _, _ = p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
	bus.take()

	paused := playing("So What", "Kind of Blue")
	paused.State = contract.Paused
	_, _, _ = p.Publish(context.Background(), paused, nil)

	sent := bus.take()
	if len(sent) != 1 {
		t.Fatalf("pausing published %d messages", len(sent))
	}
	if got := decode(t, sent[0]); got.State != contract.Paused || got.Art == nil || got.Track.Title != "So What" {
		t.Errorf("got %+v", got)
	}
}

// With no source at all, say "stopped" rather than leave the last track
// standing on a retained topic as if it were still playing.
func TestWithNothingReportingCurrentSaysStopped(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	_, _, _ = p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
	bus.take()

	if _, _, err := p.Publish(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	sent := bus.take()
	if len(sent) != 1 {
		t.Fatalf("published %d messages", len(sent))
	}
	got := decode(t, sent[0])
	if got.State != contract.Stopped || got.Track != nil || got.Source != "" || got.Art != nil {
		t.Errorf("got %+v", got)
	}
	// The cover topic is left alone: the stand keeps the last scene.
	if sent[0].topic != "eds/nowplaying/current" {
		t.Errorf("topic = %s", sent[0].topic)
	}
}

func TestAStoppedSourceWithNoTrackIsPassedOn(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{}
	st := &contract.State{V: 1, Source: "itunes-den", Player: "itunes", State: contract.Stopped, ChangedAt: at}
	_, _, _ = publisher(bus, res).Publish(context.Background(), st, nil)

	if got := decode(t, bus.take()[0]); got.State != contract.Stopped || got.Source != "itunes-den" {
		t.Errorf("got %+v", got)
	}
	if len(res.asked) != 0 {
		t.Error("a lookup was made with no track to look up")
	}
}

// If the bus refuses the cover, the state that names it must not go out, and
// the next attempt must try the cover again.
func TestAFailedPublishIsRetriedInFull(t *testing.T) {
	bus := &fakeBus{err: errors.New("bus: timed out")}
	p := publisher(bus, &fakeResolver{result: resolve.Result{Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}})

	if _, _, err := p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil); err == nil {
		t.Fatal("a failed publish was not reported")
	}

	bus.err = nil
	if _, _, err := p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil); err != nil {
		t.Fatal(err)
	}
	if sent := bus.take(); len(sent) != 2 {
		t.Errorf("the retry published %d messages, want cover and state", len(sent))
	}
}

func TestNewImage(t *testing.T) {
	if NewImage(nil) != nil || NewImage([]byte("online")) != nil || NewImage([]byte(`{"v":1}`)) != nil {
		t.Error("something that is not an image was taken for a cover")
	}
	a, b := NewImage(pngA), NewImage(pngB)
	if a == nil || a.MIME != "image/png" || len(a.SHA256) != 64 || a.SHA256 == b.SHA256 {
		t.Errorf("image = %+v", a)
	}
	if j := NewImage(jpeg); j == nil || j.MIME != "image/jpeg" {
		t.Errorf("jpeg = %+v", j)
	}
}
