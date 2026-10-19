package current

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

var (
	pngA = []byte("\x89PNG\r\n\x1a\n-cover-A")
	pngB = []byte("\x89PNG\r\n\x1a\n-cover-B")
	jpeg = []byte("\xff\xd8\xff\xe0-a-jpeg-from-a-catalog")
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

// take returns what was published since the last take, without the looks:
// every cover is told whether it has one (ADR-0019), and the tests that are
// about covers and states are not about that. takeAll is for those that are.
func (f *fakeBus) take() []published {
	var out []published
	for _, p := range f.takeAll() {
		if p.topic != contract.CurrentLookTopic("eds") {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeBus) takeAll() []published {
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
// and art - here from the player itself, with no catalog asked.
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
		t.Error("a catalog was asked although the player supplied the cover")
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

// A source that knows its album keeps its own name for it. The catalog's
// "Kind Of Blue (Legacy Edition)" is not an improvement.
func TestAKnownAlbumIsNotRenamedByACatalog(t *testing.T) {
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
		"not found":    resolve.ErrNotFound,
		"catalog down": errors.New("connection refused"),
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
// would make lightd repaint a room that is already the right color.
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

// --- sources reported without a cover (ADR-0016) -------------------------------

func coverless(bus *fakeBus, r Resolver, sources ...string) *Publisher {
	p := publisher(bus, r)
	p.Coverless = map[string]bool{}
	for _, s := range sources {
		p.Coverless[s] = true
	}
	return p
}

func from(source, title, album string) *contract.State {
	st := playing(title, album)
	st.Source, st.Player = source, source
	return st
}

// The track is reported, truthfully and in full, and nothing is published
// that could light a room: no cover from the source, none from a catalog.
func TestACoverlessSourceIsReportedWithNoPicture(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "X", Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}
	p := coverless(bus, res, "spotify")

	// Even if the source sent a cover, and names it.
	own := NewImage(pngA)
	st := from("spotify", "So What", "Kind of Blue")
	st.Art = &contract.Art{SHA256: own.SHA256}

	out, sent, err := p.Publish(context.Background(), st, own)
	if err != nil || !sent {
		t.Fatalf("sent=%v err=%v", sent, err)
	}

	got := bus.take()
	if len(got) != 1 || got[0].topic != "eds/nowplaying/current" {
		t.Fatalf("published %d messages; something went to the cover topic", len(got))
	}
	state := decode(t, got[0])
	if state.Art != nil || out.Art != nil {
		t.Errorf("the state names a cover: %+v", state.Art)
	}
	if state.Source != "spotify" || state.Track.Title != "So What" || state.Track.Album != "Kind of Blue" || state.State != contract.Playing {
		t.Errorf("the track was not reported in full: %+v %+v", state, state.Track)
	}
	if len(res.asked) != 0 {
		t.Error("a catalog was asked about a coverless source")
	}
}

// The sequence that matters in the room: iTunes, then Spotify, then iTunes on
// another album. The cover topic sees the first and the third and nothing in
// between, so a stand that follows it holds through Spotify.
func TestTheCoverTopicHoldsThroughACoverlessSource(t *testing.T) {
	bus := &fakeBus{}
	p := coverless(bus, &fakeResolver{err: resolve.ErrNotFound}, "spotify")

	first := NewImage(pngA)
	a := from("itunes", "One", "First Album")
	a.Art = &contract.Art{SHA256: first.SHA256}
	_, _, _ = p.Publish(context.Background(), a, first)

	_, _, _ = p.Publish(context.Background(), from("spotify", "Two", "Other"), nil)

	second := NewImage(pngB)
	c := from("itunes", "Three", "Second Album")
	c.Art = &contract.Art{SHA256: second.SHA256}
	_, _, _ = p.Publish(context.Background(), c, second)

	var covers, states []string
	for _, m := range bus.take() {
		if m.topic == "eds/nowplaying/current/art" {
			covers = append(covers, string(m.payload))
		} else {
			states = append(states, decode(t, m).Track.Title)
		}
	}
	if len(covers) != 2 || covers[0] != string(pngA) || covers[1] != string(pngB) {
		t.Errorf("the cover topic saw %d covers, want exactly the two iTunes ones", len(covers))
	}
	if len(states) != 3 || states[1] != "Two" {
		t.Errorf("current was %v, want all three tracks reported", states)
	}
}

// Going back to the same iTunes album after Spotify: its cover is still the
// one on the topic, so it is not sent again and the room is not repainted.
func TestReturningToTheSameAlbumAfterACoverlessSourceDoesNotRepaint(t *testing.T) {
	bus := &fakeBus{}
	p := coverless(bus, nil, "spotify")

	cover := NewImage(pngA)
	a := from("itunes", "One", "Album")
	a.Art = &contract.Art{SHA256: cover.SHA256}
	_, _, _ = p.Publish(context.Background(), a, cover)
	_, _, _ = p.Publish(context.Background(), from("spotify", "Two", "Other"), nil)
	bus.take()

	b := from("itunes", "One", "Album")
	b.Art = &contract.Art{SHA256: cover.SHA256}
	_, _, _ = p.Publish(context.Background(), b, cover)

	got := bus.take()
	if len(got) != 1 || got[0].topic != "eds/nowplaying/current" {
		t.Errorf("published %d messages; the same cover was sent again", len(got))
	}
}

func TestOtherSourcesAreUnaffectedByTheList(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Cover: &resolve.Cover{Image: jpeg, From: "deezer"}}}
	_, _, _ = coverless(bus, res, "spotify").Publish(context.Background(), from("tidal", "One", "Album"), nil)
	if got := bus.take(); len(got) != 2 {
		t.Errorf("published %d messages for a source not on the list, want cover and state", len(got))
	}
}

// --- looks (ADR-0019) -------------------------------------------------------------

func lookOf(t *testing.T, p published) contract.Look {
	t.Helper()
	var l contract.Look
	if err := json.Unmarshal(p.payload, &l); err != nil {
		t.Fatalf("look %q: %v", p.payload, err)
	}
	return l
}

// onAlbum is a track playing, naming the cover its source published.
func onAlbum(album string, cover *Image) *contract.State {
	return &contract.State{V: 1, Source: "itunes", Player: "itunes", State: contract.Playing,
		Track: &contract.Track{Title: "Time", Artist: "Pink Floyd", Album: album}, ChangedAt: at,
		Art: &contract.Art{SHA256: cover.SHA256}}
}

func spectrumFor(album string) func(contract.Track) string {
	return func(t contract.Track) string {
		if t.Album == album {
			return "spectrum"
		}
		return ""
	}
}

// A look is published for its cover, by hash, and before it: whoever lights
// the room from the cover should already know.
func TestALookGoesOutBeforeTheCoverItBelongsTo(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	p.Looks = spectrumFor("The Dark Side of the Moon")
	cover := NewImage(pngA)

	if _, _, err := p.Publish(context.Background(), onAlbum("The Dark Side of the Moon", cover), cover); err != nil {
		t.Fatal(err)
	}
	sent := bus.takeAll()
	if len(sent) != 3 || sent[0].topic != "eds/nowplaying/current/look" ||
		sent[1].topic != "eds/nowplaying/current/art" || sent[2].topic != "eds/nowplaying/current" {
		t.Fatalf("sent %d: %+v", len(sent), topicsOf(sent))
	}
	look := lookOf(t, sent[0])
	if look.V != 1 || look.Look != "spectrum" || look.Art != cover.SHA256 || !sent[0].retained {
		t.Fatalf("look %+v retained=%v", look, sent[0].retained)
	}
}

func topicsOf(sent []published) []string {
	var out []string
	for _, s := range sent {
		out = append(out, s.topic)
	}
	return out
}

// The album after a special one must not inherit its look. Its cover gets a
// message that says it has none.
func TestTheNextAlbumIsToldItHasNoLook(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	p.Looks = spectrumFor("The Dark Side of the Moon")
	p.Publish(context.Background(), onAlbum("The Dark Side of the Moon", NewImage(pngA)), NewImage(pngA))
	bus.takeAll()

	next := NewImage(pngB)
	p.Publish(context.Background(), onAlbum("Wish You Were Here", next), next)
	sent := bus.takeAll()
	if len(sent) == 0 || sent[0].topic != "eds/nowplaying/current/look" {
		t.Fatalf("sent %v", topicsOf(sent))
	}
	if look := lookOf(t, sent[0]); look.Look != "" || look.Art != next.SHA256 {
		t.Fatalf("look %+v, want none for the new cover", look)
	}
}

// Every cover gets one, the first included: a retained look from before a
// restart must not outlive the cover it was for.
func TestAnOrdinaryCoverIsToldItHasNoLookToo(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	cover := NewImage(pngA)
	p.Publish(context.Background(), onAlbum("Bloodshot", cover), cover)
	sent := bus.takeAll()
	if sent[0].topic != "eds/nowplaying/current/look" {
		t.Fatalf("sent %v", topicsOf(sent))
	}
	if look := lookOf(t, sent[0]); look.Look != "" || look.Art != cover.SHA256 {
		t.Fatalf("look %+v", look)
	}
}

func TestTheSameLookForTheSameCoverIsNotPublishedTwice(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	p.Looks = spectrumFor("The Dark Side of the Moon")
	cover := NewImage(pngA)
	p.Publish(context.Background(), onAlbum("The Dark Side of the Moon", cover), cover)
	bus.takeAll()

	// The next track on the album: the same cover, the same look.
	next := onAlbum("The Dark Side of the Moon", cover)
	next.Track.Title = "Money"
	p.Publish(context.Background(), next, cover)
	for _, s := range bus.takeAll() {
		if s.topic != "eds/nowplaying/current" {
			t.Errorf("republished %s", s.topic)
		}
	}
}

// A source reported without its cover publishes none, so there is nothing for
// a look to belong to, and the room is left as it was (ADR-0016).
func TestACoverlessSourceGetsNoLook(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	p.Looks = spectrumFor("The Dark Side of the Moon")
	p.Coverless = map[string]bool{"spotify": true}

	st := onAlbum("The Dark Side of the Moon", NewImage(pngA))
	st.Source, st.Player = "spotify", "spotify"
	p.Publish(context.Background(), st, NewImage(pngA))
	for _, s := range bus.takeAll() {
		if s.topic != "eds/nowplaying/current" {
			t.Errorf("published %s for a coverless source", s.topic)
		}
	}
}

// --- the small cover (ADR-0020) ----------------------------------------------

// shrunk stands in for the scaler: what it makes is recognizably made from
// what it was given, and it counts how often it was asked.
type shrunk struct {
	calls int
	err   error
}

func (s *shrunk) small(cover []byte) ([]byte, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte("small:"), cover...), nil
}

func withSmall(bus *fakeBus, r Resolver, s *shrunk) *Publisher {
	p := publisher(bus, r)
	p.Small = s.small
	return p
}

func covered(title string, image *Image) *contract.State {
	st := playing(title, "Kind of Blue")
	st.Art = &contract.Art{SHA256: image.SHA256}
	return st
}

func topics(sent []published) []string {
	var out []string
	for _, p := range sent {
		out = append(out, p.topic)
	}
	return out
}

// The cover, then its small form, then the state that names them: whoever
// reads the state must find both already there.
func TestTheSmallCoverGoesOutAfterTheCoverAndBeforeTheState(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{}
	cover := NewImage(pngA)

	if _, _, err := withSmall(bus, nil, s).Publish(context.Background(), covered("So What", cover), cover); err != nil {
		t.Fatal(err)
	}

	sent := bus.take()
	want := []string{"eds/nowplaying/current/art", "eds/nowplaying/current/art/small", "eds/nowplaying/current"}
	if got := topics(sent); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("published %v, want %v", got, want)
	}
	if string(sent[1].payload) != "small:"+string(pngA) || !sent[1].retained {
		t.Errorf("small cover = %q retained=%v", sent[1].payload, sent[1].retained)
	}
}

// One small cover per cover: the next track of the album does not make it
// again, and the next album does.
func TestTheSmallCoverIsMadeOncePerCover(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{}
	p := withSmall(bus, nil, s)
	a, b := NewImage(pngA), NewImage(pngB)

	for _, step := range []struct {
		title string
		cover *Image
	}{{"So What", a}, {"Freddie Freeloader", a}, {"Elsewhere", b}} {
		if _, _, err := p.Publish(context.Background(), covered(step.title, step.cover), step.cover); err != nil {
			t.Fatal(err)
		}
	}

	small := 0
	for _, sent := range bus.take() {
		if sent.topic == "eds/nowplaying/current/art/small" {
			small++
		}
	}
	if small != 2 || s.calls != 2 {
		t.Errorf("published %d small covers from %d makings, for two albums", small, s.calls)
	}
}

// A cover that cannot be made small empties the topic. Leaving it would
// leave a subscriber showing the album before, retained, as if it were this
// one.
func TestACoverWithNoSmallFormEmptiesTheSmallCover(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{err: errors.New("not a picture")}
	cover := NewImage(pngA)

	out, sentAny, err := withSmall(bus, nil, s).Publish(context.Background(), covered("So What", cover), cover)
	if err != nil || !sentAny {
		t.Fatalf("sent=%v err=%v: a cover with no small form is still published", sentAny, err)
	}
	if out.Art == nil {
		t.Error("the state lost its cover")
	}

	sent := bus.take()
	if len(sent) != 3 || sent[1].topic != "eds/nowplaying/current/art/small" {
		t.Fatalf("published %v", topics(sent))
	}
	if len(sent[1].payload) != 0 || !sent[1].retained {
		t.Errorf("small cover = %d bytes, retained=%v; want empty and retained", len(sent[1].payload), sent[1].retained)
	}
}

// No cover published, no small cover touched: a source reported without its
// cover (ADR-0016) leaves the small one exactly as it leaves the large.
func TestWithNoCoverTheSmallCoverIsLeftAlone(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{}
	p := coverless(bus, nil, "spotify")
	p.Small = s.small

	st := playing("Song", "Album")
	st.Source, st.Player = "spotify", "spotify"
	if _, _, err := p.Publish(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	if got := topics(bus.take()); len(got) != 1 || got[0] != "eds/nowplaying/current" {
		t.Errorf("published %v", got)
	}
	if s.calls != 0 {
		t.Error("something was made small with no cover to make it from")
	}
}

// A small cover the bus did not take is sent again, and the cover it was
// made from is not: lightd would repaint a room that is already right.
func TestASmallCoverTheBusRefusedIsSentAgainWithoutTheCover(t *testing.T) {
	bus, s := &failingAt{fail: "eds/nowplaying/current/art/small"}, &shrunk{}
	p := &Publisher{Bus: bus, Prefix: "eds", Small: s.small, Now: func() time.Time { return at }}
	cover := NewImage(pngA)

	if _, _, err := p.Publish(context.Background(), covered("So What", cover), cover); err == nil {
		t.Fatal("a failed publish was called a success")
	}
	bus.fail = ""
	if _, _, err := p.Publish(context.Background(), covered("So What", cover), cover); err != nil {
		t.Fatal(err)
	}

	count := map[string]int{}
	for _, topic := range bus.sent {
		count[topic]++
	}
	if count["eds/nowplaying/current/art"] != 1 || count["eds/nowplaying/current/art/small"] != 1 || count["eds/nowplaying/current"] != 1 {
		t.Errorf("published %v", count)
	}
}

// failingAt is a bus that refuses one topic.
type failingAt struct {
	fail string
	sent []string
}

func (f *failingAt) Publish(topic string, _ []byte, _ bool) error {
	if topic == f.fail {
		return errors.New("refused")
	}
	f.sent = append(f.sent, topic)
	return nil
}

// --- the owner's collection (ADR-0022) ---------------------------------------

type fakeCollection struct {
	result resolve.Result
	err    error
	asked  [][3]string
}

func (f *fakeCollection) Find(_ context.Context, artist, title, album string) (resolve.Result, error) {
	f.asked = append(f.asked, [3]string{artist, title, album})
	return f.result, f.err
}

func owned(bus *fakeBus, r Resolver, c Collection, sources ...string) *Publisher {
	p := publisher(bus, r)
	p.Collection = c
	p.Owned = map[string]bool{}
	for _, s := range sources {
		p.Owned[s] = true
	}
	return p
}

// The source heard a track and named the album a recognizer guessed. The
// collection knows the record: its title is the album, and its sleeve the
// cover. No catalog is asked.
func TestARecordTheOwnerHasIsNamedByTheCollection(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "The Singles", Cover: &resolve.Cover{Image: pngA, From: "deezer"}}}
	col := &fakeCollection{result: resolve.Result{Album: "No Jacket Required", Cover: &resolve.Cover{Image: jpeg, From: "discogs"}}}
	p := owned(bus, res, col, "turntable")

	out, _, err := p.Publish(context.Background(), from("turntable", "Take Me Home", "The Singles (Deluxe Edition)"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Track.Album != "No Jacket Required" || out.Art == nil || out.Art.From != "discogs" || out.Art.SHA256 != NewImage(jpeg).SHA256 {
		t.Fatalf("album %q, art %+v", out.Track.Album, out.Art)
	}
	if len(col.asked) != 1 || col.asked[0] != [3]string{"Artist", "Take Me Home", "The Singles (Deluxe Edition)"} {
		t.Fatalf("the collection was asked %v", col.asked)
	}
	if len(res.asked) != 0 {
		t.Fatalf("a catalog was asked as well: %v", res.asked)
	}
}

// A record that is not in the collection is completed as any track is.
func TestARecordTheOwnerDoesNotHaveIsLookedUpAsBefore(t *testing.T) {
	for name, err := range map[string]error{"not there": resolve.ErrNotFound, "could not be asked": errors.New("discogs answered 503")} {
		t.Run(name, func(t *testing.T) {
			bus := &fakeBus{}
			res := &fakeResolver{result: resolve.Result{Album: "X", Cover: &resolve.Cover{Image: pngA, From: "deezer"}}}
			p := owned(bus, res, &fakeCollection{err: err}, "turntable")

			out, _, perr := p.Publish(context.Background(), from("turntable", "So What", "Kind of Blue"), nil)
			if perr != nil {
				t.Fatal(perr)
			}
			if out.Track.Album != "Kind of Blue" || out.Art == nil || out.Art.From != "deezer" {
				t.Fatalf("album %q, art %+v", out.Track.Album, out.Art)
			}
		})
	}
}

// The collection knew the record and had no picture of it. The cover is then
// looked up - under the record's title, not the one the source guessed.
func TestARecordWithNoPictureIsLookedUpByItsOwnTitle(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "ignored", Cover: &resolve.Cover{Image: pngA, From: "deezer"}}}
	p := owned(bus, res, &fakeCollection{result: resolve.Result{Album: "No Jacket Required"}}, "turntable")

	out, _, err := p.Publish(context.Background(), from("turntable", "Take Me Home", "The Singles"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Track.Album != "No Jacket Required" || out.Art == nil || out.Art.From != "deezer" {
		t.Fatalf("album %q, art %+v", out.Track.Album, out.Art)
	}
	if len(res.asked) != 1 || res.asked[0].Album != "No Jacket Required" {
		t.Fatalf("the catalog was asked about %+v", res.asked)
	}
}

// Only the sources named. iTunes says which album it is playing, and knows.
func TestASourceThatIsNotNamedIsNeverAskedOfTheCollection(t *testing.T) {
	bus := &fakeBus{}
	res := &fakeResolver{result: resolve.Result{Album: "X", Cover: &resolve.Cover{Image: pngA, From: "deezer"}}}
	col := &fakeCollection{result: resolve.Result{Album: "Another", Cover: &resolve.Cover{Image: jpeg, From: "discogs"}}}
	p := owned(bus, res, col, "turntable")

	out, _, err := p.Publish(context.Background(), from("itunes", "So What", "Kind of Blue"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(col.asked) != 0 || out.Track.Album != "Kind of Blue" || out.Art.From != "deezer" {
		t.Fatalf("asked %v, album %q, art %+v", col.asked, out.Track.Album, out.Art)
	}
}

// Idle (ADR-0024): the track is taken away, the cover topics are emptied, and
// the source is still named, because a pause is undone from where it was made.
func TestIdleTakesTheTrackAndTheCoverAwayAndKeepsTheSource(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{}
	cover := NewImage(pngA)
	p := withSmall(bus, nil, s)

	paused := covered("So What", cover)
	paused.State = contract.Paused
	_, _, _ = p.Publish(context.Background(), paused, cover)
	bus.take()

	out, sent, err := p.PublishIdle(context.Background(), paused, false)
	if err != nil || !sent {
		t.Fatalf("idle: sent=%v err=%v", sent, err)
	}
	if out.State != contract.Stopped || out.Track != nil || out.Art != nil || out.Source != paused.Source {
		t.Errorf("idle state = %+v", out)
	}

	got := bus.take()
	want := []string{"eds/nowplaying/current", "eds/nowplaying/current/art", "eds/nowplaying/current/art/small"}
	if names := topics(got); len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("published %v, want %v", topics(got), want)
	}
	if len(got[1].payload) != 0 || len(got[2].payload) != 0 || !got[1].retained || !got[2].retained {
		t.Errorf("the cover topics were not emptied: %d and %d bytes", len(got[1].payload), len(got[2].payload))
	}

	// Said again, as it is when a late cover makes the hub decide again.
	if _, sent, _ := p.PublishIdle(context.Background(), paused, false); sent {
		t.Errorf("idle said twice published again: %v", topics(bus.take()))
	}
}

// Play pressed after an idle: the same track and the same cover, and all of
// it has to go out again, because the topics no longer hold it.
func TestTheSameCoverIsPublishedAgainAfterIdle(t *testing.T) {
	bus, s := &fakeBus{}, &shrunk{}
	cover := NewImage(pngA)
	p := withSmall(bus, nil, s)

	_, _, _ = p.Publish(context.Background(), covered("So What", cover), cover)
	_, _, _ = p.PublishIdle(context.Background(), covered("So What", cover), false)
	bus.takeAll()

	_, _, _ = p.Publish(context.Background(), covered("So What", cover), cover)
	got := topics(bus.takeAll())
	want := []string{"eds/nowplaying/current/look", "eds/nowplaying/current/art", "eds/nowplaying/current/art/small", "eds/nowplaying/current"}
	if len(got) != len(want) {
		t.Fatalf("published %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("published %v, want %v", got, want)
		}
	}
}

func TestIdleWithNobodyReportingNamesNoSource(t *testing.T) {
	bus := &fakeBus{}
	out, sent, err := publisher(bus, nil).PublishIdle(context.Background(), nil, false)
	if err != nil || !sent || out.Source != "" || out.State != contract.Stopped {
		t.Fatalf("out=%+v sent=%v err=%v", out, sent, err)
	}
	// No small cover is made by this publisher, so that topic is not its to empty.
	if got := topics(bus.take()); len(got) != 2 || got[1] != "eds/nowplaying/current/art" {
		t.Errorf("published %v", got)
	}
}

// ADR-0028: an idle room is told whether anything is switched on, as the
// look of no cover, before the cover is taken away, and again when the
// switch changes with nothing else to say.
func TestIdleNamesTheLookOfNoCover(t *testing.T) {
	bus := &fakeBus{}
	cover := NewImage(pngA)
	p := publisher(bus, nil)
	_, _, _ = p.Publish(context.Background(), covered("So What", cover), cover)
	bus.takeAll()

	_, _, _ = p.PublishIdle(context.Background(), nil, true)
	got := bus.takeAll()
	if names := topics(got); len(names) != 3 || names[1] != "eds/nowplaying/current/look" || names[2] != "eds/nowplaying/current/art" {
		t.Fatalf("published %v", names)
	}
	if string(got[1].payload) != `{"v":1,"look":"standby","art":""}` || !got[1].retained {
		t.Errorf("look = %s retained=%v", got[1].payload, got[1].retained)
	}

	// Switched off, in a room already idle: the look alone.
	_, sent, _ := p.PublishIdle(context.Background(), nil, false)
	got = bus.takeAll()
	if !sent || len(got) != 1 || string(got[0].payload) != `{"v":1,"art":""}` {
		t.Fatalf("switched off: sent=%v published %v %s", sent, topics(got), got[0].payload)
	}
	if _, sent, _ := p.PublishIdle(context.Background(), nil, false); sent {
		t.Errorf("said twice published again: %v", topics(bus.takeAll()))
	}
}

// ADR-0028: a source no room is lit for still has its cover published, and
// with it the look that says to light nothing.
func TestADarkSourcesCoverIsPublishedWithTheDarkLook(t *testing.T) {
	bus := &fakeBus{}
	cover := NewImage(pngA)
	p := publisher(bus, nil)
	p.Looks = func(contract.Track) string { return "spectrum" }
	st := covered("So What", cover)
	p.Dark = map[string]bool{st.Source: true}

	out, _, err := p.Publish(context.Background(), st, cover)
	if err != nil || out.Art == nil {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	got := bus.takeAll()
	if names := topics(got); len(names) != 3 || names[0] != "eds/nowplaying/current/look" || names[1] != "eds/nowplaying/current/art" {
		t.Fatalf("published %v", names)
	}
	if want := `{"v":1,"look":"dark","art":"` + cover.SHA256 + `"}`; string(got[0].payload) != want {
		t.Errorf("look = %s, want %s", got[0].payload, want)
	}
	if string(got[1].payload) != string(pngA) {
		t.Error("the cover itself was not published")
	}
}

// ADR-0030: a look that is for any source is published for a dark one too.
func TestALookForAnySourceIsNotDarkened(t *testing.T) {
	bus := &fakeBus{}
	cover := NewImage(pngA)
	p := publisher(bus, nil)
	p.Looks = func(contract.Track) string { return "scanner" }
	p.AnySource = func(look string) bool { return look == "scanner" }
	st := covered("Knight Rider Theme", cover)
	p.Dark = map[string]bool{st.Source: true}

	_, _, _ = p.Publish(context.Background(), st, cover)
	got := bus.takeAll()
	if want := `{"v":1,"look":"scanner","art":"` + cover.SHA256 + `"}`; string(got[0].payload) != want {
		t.Errorf("look = %s, want %s", got[0].payload, want)
	}

	// The next track on the same record has no look, and is dark again.
	p.Looks = func(contract.Track) string { return "" }
	_, _, _ = p.Publish(context.Background(), covered("Another", cover), cover)
	got = bus.takeAll()
	if want := `{"v":1,"look":"dark","art":"` + cover.SHA256 + `"}`; len(got) == 0 || string(got[0].payload) != want {
		t.Errorf("after it: published %v", topics(got))
	}
}

// ADR-0025: whoever draws buttons is told when there is nothing they would do.
func TestASourceWithoutControlsSaysSoInCurrent(t *testing.T) {
	bus := &fakeBus{}
	p := publisher(bus, nil)
	p.NoControls = map[string]bool{"turntable": true}

	record := playing("So What", "Kind of Blue")
	record.Source, record.Player = "turntable", "turntable"
	out, _, _ := p.Publish(context.Background(), record, nil)
	if !out.NoControls {
		t.Errorf("the turntable's state does not say it has no controls: %+v", out)
	}
	sent := bus.take()
	if got := decode(t, sent[len(sent)-1]); !got.NoControls {
		t.Errorf("published without it: %s", sent[len(sent)-1].payload)
	}

	out, _, _ = p.Publish(context.Background(), playing("So What", "Kind of Blue"), nil)
	if out.NoControls {
		t.Errorf("iTunes was said to have no controls: %+v", out)
	}
	sent = bus.take()
	if strings.Contains(string(sent[len(sent)-1].payload), "no_controls") {
		t.Errorf("the field is written where it is not so: %s", sent[len(sent)-1].payload)
	}
}

// --- the record by what is on it (ADR-0038) ----------------------------------

// fakeAlbums knows two records and which original album a track is on.
type fakeAlbums struct {
	tracks    map[string][]string // album: its tracks
	originals map[string]string   // title: its original album
	hasAsked  []string
}

func (f *fakeAlbums) Has(_ context.Context, _, album, title string) (bool, error) {
	f.hasAsked = append(f.hasAsked, album+": "+title)
	tracks, ok := f.tracks[album]
	if !ok {
		return false, resolve.ErrNotFound
	}
	for _, t := range tracks {
		if t == title {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAlbums) Original(_ context.Context, _, title, _ string) (string, error) {
	if a, ok := f.originals[title]; ok {
		return a, nil
	}
	return "", resolve.ErrNotFound
}

func pinkFloyd() *fakeAlbums {
	return &fakeAlbums{
		tracks: map[string][]string{
			"The Dark Side of the Moon": {"Speak to Me/Breathe", "Breathe (In the Air)", "On the Run", "Time", "Money"},
			"Animals":                   {"Pigs On the Wing, Pt. 1", "Dogs", "Sheep"},
		},
		originals: map[string]string{
			"Speak to Me/Breathe": "The Dark Side of the Moon", "Time": "The Dark Side of the Moon", "Money": "The Dark Side of the Moon",
			"Pigs On the Wing, Pt. 1": "Animals", "Dogs": "Animals",
		},
	}
}

func onTurntable(title, album string) *contract.State {
	st := from("turntable", title, album)
	st.Track.Artist = "Pink Floyd"
	return st
}

func albumOf(t *testing.T, p *Publisher, st *contract.State) string {
	t.Helper()
	out, _, err := p.Publish(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out.Track.Album
}

// The evening of 2026-10-18, as it should have gone: Dark Side, its other
// side, then Animals put on a minute later. No clock is consulted; what is on
// each record decides.
func TestTheRecordIsNamedByWhatIsOnIt(t *testing.T) {
	albums := pinkFloyd()
	p := owned(&fakeBus{}, &fakeResolver{err: resolve.ErrNotFound}, &fakeCollection{err: resolve.ErrNotFound}, "turntable")
	p.Albums = albums

	steps := []struct{ title, heard, want string }{
		// No album heard: the original, not the anniversary edition.
		{"Speak to Me/Breathe", "", "The Dark Side of the Moon"},
		{"On the Run", "The Dark Side of the Moon", "The Dark Side of the Moon"},
		// The other side, filed by the recognizer under a box set: on the
		// record playing, so the record stays.
		{"Money", "Discovery (Box Set)", "The Dark Side of the Moon"},
		// Another record by the same band: not on Dark Side, so its own.
		{"Pigs On the Wing, Pt. 1", "The Dark Side of the Moon", "Animals"},
		{"Dogs", "Animals", "Animals"},
	}
	for _, s := range steps {
		if got := albumOf(t, p, onTurntable(s.title, s.heard)); got != s.want {
			t.Errorf("%q heard as %q: named %q, want %q", s.title, s.heard, got, s.want)
		}
	}
}

// Nothing to be had about a track: the album as the source named it.
func TestWithNoAnswerTheHeardAlbumStands(t *testing.T) {
	p := owned(&fakeBus{}, &fakeResolver{err: resolve.ErrNotFound}, &fakeCollection{err: resolve.ErrNotFound}, "turntable")
	p.Albums = pinkFloyd()
	if got := albumOf(t, p, onTurntable("See Emily Play", "Relics")); got != "Relics" {
		t.Errorf("named %q, want the heard album", got)
	}
}

// The collection names the record, and the catalogs are not asked; the next
// track is checked against the collection's record.
func TestTheCollectionComesFirst(t *testing.T) {
	albums := pinkFloyd()
	col := &fakeCollection{result: resolve.Result{Album: "The Dark Side of the Moon"}}
	p := owned(&fakeBus{}, &fakeResolver{err: resolve.ErrNotFound}, col, "turntable")
	p.Albums = albums
	if got := albumOf(t, p, onTurntable("Time", "A Foot in the Door")); got != "The Dark Side of the Moon" {
		t.Errorf("named %q", got)
	}
	if len(albums.hasAsked) != 0 {
		t.Errorf("the catalogs were asked about a record the collection has: %v", albums.hasAsked)
	}

	// Then a record the collection does not have.
	col.result, col.err = resolve.Result{}, resolve.ErrNotFound
	if got := albumOf(t, p, onTurntable("Dogs", "The Dark Side of the Moon")); got != "Animals" {
		t.Errorf("named %q, want Animals", got)
	}
}

// Another source is left as it named its tracks.
func TestOnlyOwnedSourcesAreRenamed(t *testing.T) {
	p := owned(&fakeBus{}, &fakeResolver{err: resolve.ErrNotFound}, &fakeCollection{err: resolve.ErrNotFound}, "turntable")
	p.Albums = pinkFloyd()
	st := onTurntable("Dogs", "Animals (2018 Remix)")
	st.Source = "itunes"
	if got := albumOf(t, p, st); got != "Animals (2018 Remix)" {
		t.Errorf("named %q", got)
	}
}
