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
// catalog's guess, then the player's own cover 24 ms later.
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

// A switch is a decision like a presence: it goes to the publisher at once,
// with the cover of whoever it leaves current (ADR-0023).
func TestASwitchDecidesAndCarriesTheRightCover(t *testing.T) {
	b := newBench()
	b.art("itunes", coverA)
	b.state("itunes", "Paused", coverA, contract.Paused)
	b.state("turntable", "Record", nil, contract.Playing)
	b.take()

	b.hub.Power("turntable", false)
	got := b.take()
	if len(got) != 1 || title(got[0]) != "Paused" || got[0].Cover == nil {
		t.Fatalf("switched off: decisions = %+v", got)
	}

	b.hub.Power("turntable", false)
	if got := b.take(); len(got) != 0 {
		t.Fatalf("said twice: decisions = %+v", got)
	}

	b.hub.Power("turntable", true)
	got = b.take()
	if len(got) != 1 || title(got[0]) != "Record" || got[0].Cover != nil {
		t.Fatalf("switched on: decisions = %+v", got)
	}
}

// idleBench is a bench whose hub goes idle, with the time held too.
func idleBench() (*bench, *time.Time) {
	b := newBench()
	now := time.Now()
	b.hub.IdleAfter = 15 * time.Second
	b.hub.Now = func() time.Time { return now }
	return b, &now
}

// ADR-0024: paused long enough is not playing.
func TestAPausedTrackGoesIdleWhenItsTimeIsUp(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Playing)
	b.elapse()
	if got := b.take(); len(got) != 1 || got[0].Idle {
		t.Fatalf("playing: decisions = %+v", got)
	}

	b.state("itunes", "One", nil, contract.Paused)
	if got := b.take(); len(got) != 1 || got[0].Idle || title(got[0]) != "One" {
		t.Fatalf("paused, at first: decisions = %+v", got)
	}

	b.elapse()
	got := b.take()
	if len(got) != 1 || !got[0].Idle || got[0].Winner == nil || got[0].Winner.Source != "itunes" {
		t.Fatalf("paused, later: decisions = %+v", got)
	}
}

func TestPlayingAgainOvertakesTheIdleThatWasComing(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Paused)
	b.state("itunes", "One", nil, contract.Playing)
	b.take()

	b.elapse()
	if got := b.take(); len(got) != 0 {
		t.Fatalf("an idle came for a track that is playing: %+v", got)
	}
}

// A track paused an hour ago is idle at once, as when this program starts and
// is handed what was retained.
func TestATrackPausedLongAgoIsIdleAtOnce(t *testing.T) {
	b, now := idleBench()
	*now = now.Add(time.Hour)
	b.state("itunes", "One", nil, contract.Paused) // dated by the bench an hour before "now"

	got := b.take()
	if len(got) != 1 || !got[0].Idle || got[0].Cover != nil {
		t.Fatalf("decisions = %+v", got)
	}
	if len(b.timers) != 0 {
		t.Errorf("a timer was set for what is already idle")
	}
}

func TestNobodyReportingGoesIdleToo(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Playing)
	b.status("itunes", contract.Offline)
	b.take()

	b.elapse()
	if got := b.take(); len(got) != 1 || !got[0].Idle || got[0].Winner != nil {
		t.Fatalf("decisions = %+v", got)
	}
}

// A record between its sides is held, stopped (ADR-0023), and is not a room
// to turn the lights down in: it has a longer wait of its own.
func TestAStoppedWinnerWaitsItsOwnLongerWait(t *testing.T) {
	b, now := idleBench()
	b.hub.StoppedIdleAfter = 5 * time.Minute
	var waits []time.Duration
	b.hub.After = func(d time.Duration, f func()) { waits = append(waits, d); b.timers = append(b.timers, f) }

	b.hub.Power("turntable", true)
	b.state("turntable", "Side One", nil, contract.Playing)
	b.state("turntable", "Side One", nil, contract.Stopped)
	if got := b.take(); len(got) != 2 || got[1].Idle {
		t.Fatalf("the needle lifted: decisions = %+v", got)
	}
	if len(waits) != 1 || waits[0] < 4*time.Minute || waits[0] > 5*time.Minute {
		t.Fatalf("waits = %v, want one of about five minutes", waits)
	}

	// The other side, before the time is up: nothing dims.
	b.state("turntable", "Side Two", nil, contract.Playing)
	b.take()
	b.elapse()
	if got := b.take(); len(got) != 0 {
		t.Fatalf("an idle came for a record that is playing: %+v", got)
	}

	// Left on with nothing on it.
	b.state("turntable", "Side Two", nil, contract.Stopped)
	b.take()
	*now = now.Add(6 * time.Minute)
	b.elapse()
	got := b.take()
	if len(got) != 1 || !got[0].Idle || got[0].Winner == nil || got[0].Winner.Source != "turntable" {
		t.Fatalf("left on: decisions = %+v", got)
	}
}

func TestAStoppedWinnerNeverGoesIdleWhenItsWaitIsOff(t *testing.T) {
	b, _ := idleBench()
	b.hub.Power("turntable", true)
	b.state("turntable", "Side One", nil, contract.Playing)
	b.state("turntable", "Side One", nil, contract.Stopped)
	b.take()

	b.elapse()
	if got := b.take(); len(got) != 0 {
		t.Fatalf("a held, stopped source went idle: %+v", got)
	}
}

// ADR-0028: an idle decision says whether anything is switched on, and a
// switch that changes no winner still says it again.
func TestAnIdleRoomIsToldWhenASwitchChanges(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Paused)
	b.elapse()
	got := b.take()
	if last := got[len(got)-1]; !last.Idle || last.On {
		t.Fatalf("paused, nothing switched on: decisions = %+v", got)
	}

	// The turntable is switched on, and its driver has said nothing: iTunes
	// is still who is current.
	b.hub.Power("turntable", true)
	got = b.take()
	if len(got) != 1 || !got[0].Idle || !got[0].On || got[0].Winner == nil || got[0].Winner.Source != "itunes" {
		t.Fatalf("switched on: decisions = %+v", got)
	}
	b.hub.Power("turntable", true)
	if got := b.take(); len(got) != 0 {
		t.Fatalf("said on twice: decisions = %+v", got)
	}

	b.hub.Power("turntable", false)
	if got := b.take(); len(got) != 1 || !got[0].Idle || got[0].On {
		t.Fatalf("switched off: decisions = %+v", got)
	}
}

// A switch does not make a room idle that is not: a track playing elsewhere
// is not decided again for it.
func TestASwitchThatChangesNoWinnerLeavesAPlayingRoomAlone(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Playing)
	b.take()
	b.hub.Power("turntable", false)
	if got := b.take(); len(got) != 0 {
		t.Fatalf("decisions = %+v", got)
	}
}

// Started with every source offline: nothing changes the winner from none,
// and it has to be decided all the same, or the room is never told.
func TestNobodyReportingAtTheStartIsDecidedAndGoesIdle(t *testing.T) {
	b, _ := idleBench()
	b.status("turntable", "offline")
	b.hub.Power("turntable", true)
	b.hub.Settle()
	if got := b.take(); len(got) != 0 {
		t.Fatalf("before the retained messages are in: decisions = %+v", got)
	}

	b.elapse()
	if got := b.take(); len(got) != 1 || got[0].Idle || got[0].Winner != nil {
		t.Fatalf("settled: decisions = %+v", got)
	}
	b.elapse()
	if got := b.take(); len(got) != 1 || !got[0].Idle || !got[0].On || got[0].Winner != nil {
		t.Fatalf("later: decisions = %+v", got)
	}
}

func TestSettlingDecidesNothingWhenASourceAlreadyHas(t *testing.T) {
	b, _ := idleBench()
	b.state("itunes", "One", nil, contract.Playing)
	b.hub.Settle()
	b.take()
	b.elapse()
	if got := b.take(); len(got) != 0 {
		t.Fatalf("decisions = %+v", got)
	}
}

// ADR-0034: an online that is not said again lapses. A broker can keep a
// driver's stored online long after the driver has gone.
func TestASourcesPresenceLapsesUnrepeated(t *testing.T) {
	b := newBench()
	b.hub.PresenceLapse = 90 * time.Second
	b.status("itunes", "online")
	b.state("itunes", "One", nil, contract.Playing)
	if got := b.take(); len(got) == 0 || title(got[len(got)-1]) != "One" {
		t.Fatalf("playing: decisions = %+v", got)
	}

	b.elapse()
	got := b.take()
	if len(got) != 1 || got[0].Winner != nil {
		t.Fatalf("lapsed: decisions = %+v", got)
	}
}

func TestARepeatedPresenceDoesNotLapse(t *testing.T) {
	b := newBench()
	b.hub.PresenceLapse = 90 * time.Second
	b.status("itunes", "online")
	b.state("itunes", "One", nil, contract.Playing)
	b.status("itunes", "online") // thirty seconds later
	b.take()
	if len(b.timers) != 2 {
		t.Fatalf("timers = %d, want one for each online", len(b.timers))
	}

	b.timers[0]() // ninety seconds after the first, sixty after the repeat
	if got := b.take(); len(got) != 0 {
		t.Fatalf("lapsed though repeated: decisions = %+v", got)
	}
	b.timers[1]() // ninety seconds after the repeat
	if got := b.take(); len(got) != 1 || got[0].Winner != nil {
		t.Fatalf("lapsed: decisions = %+v", got)
	}
}

func TestOfflineCancelsALapse(t *testing.T) {
	b := newBench()
	b.hub.PresenceLapse = 90 * time.Second
	b.status("itunes", "online")
	b.status("itunes", "offline")
	b.status("itunes", "online")
	b.state("itunes", "One", nil, contract.Playing)
	b.take()

	b.timers[0]() // the first online's, overtaken
	if got := b.take(); len(got) != 0 {
		t.Fatalf("an overtaken lapse acted: decisions = %+v", got)
	}
}

// A beacon's on that lapses is unknown, not off: the source is arbitrated
// as one without a beacon (ADR-0023), and the room is told nothing is on.
func TestABeaconsOnLapsesToUnknown(t *testing.T) {
	b, _ := idleBench()
	b.hub.PresenceLapse = 90 * time.Second
	b.state("itunes", "One", nil, contract.Paused)
	b.elapse()
	b.take()

	b.hub.Power("turntable", true)
	if got := b.take(); len(got) != 1 || !got[0].On {
		t.Fatalf("switched on: decisions = %+v", got)
	}
	b.elapse()
	if got := b.take(); len(got) != 1 || got[0].On {
		t.Fatalf("lapsed: decisions = %+v", got)
	}
}
