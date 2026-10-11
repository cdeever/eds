package hub

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
)

var (
	quiet  = slog.New(slog.NewTextHandler(io.Discard, nil))
	coverA = []byte("\x89PNG\r\n\x1a\n-cover-A")
	coverB = []byte("\x89PNG\r\n\x1a\n-cover-B")
)

// bench is a hub with its clock held: timers do not fire until told to.
type bench struct {
	hub       *Hub
	decisions []Decision
	timers    []func()
}

func newBench() *bench {
	b := &bench{}
	b.hub = New("eds", func(d Decision) { b.decisions = append(b.decisions, d) }, quiet)
	b.hub.After = func(_ time.Duration, f func()) { b.timers = append(b.timers, f) }
	return b
}

// elapse fires every timer that has been set, as if the grace had passed.
func (b *bench) elapse() {
	timers := b.timers
	b.timers = nil
	for _, f := range timers {
		f()
	}
}

func (b *bench) take() []Decision {
	out := b.decisions
	b.decisions = nil
	return out
}

func (b *bench) state(source, title string, art []byte, ps contract.PlayState) {
	st := contract.State{Source: source, Player: "itunes", State: ps,
		Track: &contract.Track{Title: title, Artist: "Artist", Album: "Album of " + title}, ChangedAt: time.Now()}
	if art != nil {
		st.Art = &contract.Art{SHA256: current.NewImage(art).SHA256}
	}
	payload, err := contract.Encode(st)
	if err != nil {
		panic(err)
	}
	b.hub.Handle(bus.Message{Topic: contract.SourceStateTopic("eds", source), Payload: payload})
}

func (b *bench) art(source string, image []byte) {
	b.hub.Handle(bus.Message{Topic: contract.SourceArtTopic("eds", source), Payload: image})
}

func (b *bench) status(source, s string) {
	b.hub.Handle(bus.Message{Topic: contract.SourceStatusTopic("eds", source), Payload: []byte(s)})
}

func title(d Decision) string {
	if d.Winner == nil || d.Winner.Track == nil {
		return ""
	}
	return d.Winner.Track.Title
}

// The order a driver sends them in: cover, then the state that names it.
func TestCoverThenStateIsDecidedAtOnce(t *testing.T) {
	b := newBench()
	b.art("itunes", coverA)
	if got := b.take(); len(got) != 0 {
		t.Fatalf("a cover alone produced %d decisions", len(got))
	}

	b.state("itunes", "One", coverA, contract.Playing)
	got := b.take()
	if len(got) != 1 || title(got[0]) != "One" || got[0].Cover == nil || string(got[0].Cover.Bytes) != string(coverA) {
		t.Fatalf("decisions = %+v", got)
	}
	if len(b.timers) != 0 {
		t.Error("a wait was started although the cover was already here")
	}
}

// Found by deploying: a broker handing over retained messages gave the state
// before the cover. nowplayd took the track to have none and published a
// catalogue's guess, then the player's own cover 24 ms later.
func TestStateThenCoverWaitsForTheCover(t *testing.T) {
	b := newBench()
	b.state("itunes", "One", coverA, contract.Playing)
	if got := b.take(); len(got) != 0 {
		t.Fatalf("decided before the cover the state names had arrived: %+v", got)
	}

	b.art("itunes", coverA)
	got := b.take()
	if len(got) != 1 || got[0].Cover == nil || got[0].Cover.SHA256 != current.NewImage(coverA).SHA256 {
		t.Fatalf("decisions = %+v", got)
	}

	// The wait that was started must not now produce a second decision.
	b.elapse()
	if extra := b.take(); len(extra) != 0 {
		t.Errorf("the overtaken wait still decided: %+v", extra)
	}
}

// A cover that never comes must not hold the track back for ever.
func TestACoverThatNeverComesIsGivenUpOn(t *testing.T) {
	b := newBench()
	b.state("itunes", "One", coverA, contract.Playing)
	b.elapse()

	got := b.take()
	if len(got) != 1 || title(got[0]) != "One" {
		t.Fatalf("decisions = %+v", got)
	}
	if got[0].Cover != nil {
		t.Error("a cover was supplied that never arrived")
	}
}

// On a track change the cover on hand is the previous album's. It must not be
// passed off as this one's, nor decided on before the right one arrives.
func TestTheCoverOfTheTrackBeforeIsNotThisOnes(t *testing.T) {
	b := newBench()
	b.art("itunes", coverA)
	b.state("itunes", "One", coverA, contract.Playing)
	b.take()

	b.state("itunes", "Two", coverB, contract.Playing) // names B; only A is held
	if got := b.take(); len(got) != 0 {
		t.Fatalf("decided with the wrong cover on hand: %+v", got)
	}
	b.art("itunes", coverB)
	got := b.take()
	if len(got) != 1 || title(got[0]) != "Two" || string(got[0].Cover.Bytes) != string(coverB) {
		t.Fatalf("decisions = %+v", got)
	}
}

// A track that moves on while its cover is awaited: the old wait is dead.
func TestAWaitIsOvertakenByTheNextTrack(t *testing.T) {
	b := newBench()
	b.state("itunes", "One", coverA, contract.Playing) // waits
	b.state("itunes", "Two", nil, contract.Playing)    // names no cover: decided now

	got := b.take()
	if len(got) != 1 || title(got[0]) != "Two" {
		t.Fatalf("decisions = %+v", got)
	}
	b.elapse()
	if extra := b.take(); len(extra) != 0 {
		t.Errorf("the wait for track One still decided: %+v", extra)
	}
}

func TestASourceWithNoCoverIsDecidedAtOnce(t *testing.T) {
	b := newBench()
	b.state("tidal", "One", nil, contract.Playing)
	got := b.take()
	if len(got) != 1 || got[0].Cover != nil || len(b.timers) != 0 {
		t.Fatalf("decisions = %+v, timers = %d", got, len(b.timers))
	}
}

func TestMostRecentSourceWinsAndCarriesItsOwnCover(t *testing.T) {
	b := newBench()
	b.art("itunes", coverA)
	b.state("itunes", "One", coverA, contract.Playing)
	b.art("spotify", coverB)
	b.state("spotify", "Two", coverB, contract.Playing)

	got := b.take()
	last := got[len(got)-1]
	if title(last) != "Two" || string(last.Cover.Bytes) != string(coverB) {
		t.Fatalf("last decision = %q with cover %v", title(last), last.Cover != nil)
	}

	b.status("spotify", contract.Offline)
	got = b.take()
	if len(got) != 1 || title(got[0]) != "One" || string(got[0].Cover.Bytes) != string(coverA) {
		t.Fatalf("after spotify went offline: %+v", got)
	}
}

func TestWhenTheLastSourceGoesNothingIsCurrent(t *testing.T) {
	b := newBench()
	b.state("itunes", "One", nil, contract.Playing)
	b.take()
	b.status("itunes", contract.Offline)

	got := b.take()
	if len(got) != 1 || got[0].Winner != nil || got[0].Cover != nil {
		t.Fatalf("decisions = %+v", got)
	}
}

func TestWhatIsNotAStateIsIgnored(t *testing.T) {
	b := newBench()
	for name, msg := range map[string]bus.Message{
		"garbage":            {Topic: contract.SourceStateTopic("eds", "itunes"), Payload: []byte("not json")},
		"a later version":    {Topic: contract.SourceStateTopic("eds", "itunes"), Payload: []byte(`{"v":2,"source":"itunes","state":"playing","changed_at":"2026-10-11T00:00:00Z"}`)},
		"claims to be other": {Topic: contract.SourceStateTopic("eds", "itunes"), Payload: []byte(`{"v":1,"source":"spotify","state":"playing","changed_at":"2026-10-11T00:00:00Z"}`)},
		"another topic":      {Topic: "eds/nowplaying/current", Payload: []byte(`{"v":1,"state":"playing","changed_at":"2026-10-11T00:00:00Z"}`)},
		"another tenant":     {Topic: contract.SourceStateTopic("other", "itunes"), Payload: []byte(`{"v":1,"source":"itunes","state":"playing","changed_at":"2026-10-11T00:00:00Z"}`)},
	} {
		b.hub.Handle(msg)
		if got := b.take(); len(got) != 0 {
			t.Errorf("%s produced a decision: %+v", name, got)
		}
	}
}

// Something that is not an image on an art topic is a source with no cover.
func TestWhatIsNotAnImageClearsTheCover(t *testing.T) {
	b := newBench()
	b.art("itunes", coverA)
	b.state("itunes", "One", nil, contract.Playing)
	b.take()

	b.art("itunes", []byte("online"))
	got := b.take()
	if len(got) != 1 || got[0].Cover != nil {
		t.Fatalf("decisions = %+v", got)
	}
}
