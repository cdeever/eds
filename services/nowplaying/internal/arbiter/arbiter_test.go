package arbiter

import (
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// clock is a hand-wound clock: each event in a test happens one second after
// the last, so "most recent" is never a matter of how fast the test ran.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }
func (c *clock) tick()          { c.t = c.t.Add(time.Second) }

var epoch = time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC)

func state(src string, ps contract.PlayState, title string) contract.State {
	st := contract.State{V: contract.Version, Source: src, Player: src, State: ps}
	if title != "" {
		st.Track = &contract.Track{Title: title, Artist: "Artist", Album: "Album of " + title}
	}
	return st
}

// step is one thing that happens, and what the current track should be after.
type step struct {
	name string
	do   func(a *Arbiter) (*contract.State, bool)

	wantSource  string // "" means nothing is current
	wantTitle   string
	wantState   contract.PlayState
	wantChanged bool
}

func run(t *testing.T, steps []step) {
	t.Helper()
	c := &clock{t: epoch}
	a := New(c.now)

	for _, s := range steps {
		c.tick()
		got, changed := s.do(a)

		if s.wantSource == "" {
			if got != nil {
				t.Fatalf("%s: current = %s %q, want nothing", s.name, got.Source, title(got))
			}
		} else {
			if got == nil {
				t.Fatalf("%s: nothing is current, want %s %q", s.name, s.wantSource, s.wantTitle)
			}
			if got.Source != s.wantSource || title(got) != s.wantTitle || got.State != s.wantState {
				t.Fatalf("%s: current = %s %q %s, want %s %q %s", s.name,
					got.Source, title(got), got.State, s.wantSource, s.wantTitle, s.wantState)
			}
		}
		if changed != s.wantChanged {
			t.Fatalf("%s: changed = %v, want %v", s.name, changed, s.wantChanged)
		}
	}
}

func title(s *contract.State) string {
	if s == nil || s.Track == nil {
		return ""
	}
	return s.Track.Title
}

func update(st contract.State) func(*Arbiter) (*contract.State, bool) {
	return func(a *Arbiter) (*contract.State, bool) { return a.Update(st) }
}

func presence(id string, online bool) func(*Arbiter) (*contract.State, bool) {
	return func(a *Arbiter) (*contract.State, bool) { return a.Presence(id, online) }
}

func TestNothingIsCurrentUntilSomethingReports(t *testing.T) {
	if got := New(nil).Current(); got != nil {
		t.Errorf("current = %+v, want nothing", got)
	}
}

func TestOneSourceIsSimplyFollowed(t *testing.T) {
	run(t, []step{
		{"starts", update(state("itunes", contract.Playing, "One")), "itunes", "One", contract.Playing, true},
		{"next track", update(state("itunes", contract.Playing, "Two")), "itunes", "Two", contract.Playing, true},
		{"pauses", update(state("itunes", contract.Paused, "Two")), "itunes", "Two", contract.Paused, true},
		{"resumes", update(state("itunes", contract.Playing, "Two")), "itunes", "Two", contract.Playing, true},
		{"stops", update(state("itunes", contract.Stopped, "")), "itunes", "", contract.Stopped, true},
	})
}

// The rule itself: whoever most recently started or changed track is current.
func TestMostRecentWins(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"spotify starts later", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"itunes changes track", update(state("itunes", contract.Playing, "C")), "itunes", "C", contract.Playing, true},
		{"spotify changes track", update(state("spotify", contract.Playing, "D")), "spotify", "D", contract.Playing, true},
	})
}

// A driver that reports progress every few seconds must not win by talking.
func TestRepeatingTheSameTrackDoesNotMakeASourceMoreRecent(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"itunes reports A again", update(state("itunes", contract.Playing, "A")), "spotify", "B", contract.Playing, false},
		{"and again", update(state("itunes", contract.Playing, "A")), "spotify", "B", contract.Playing, false},
	})
}

func TestWhenTheWinnerStopsWhatIsStillPlayingTakesOver(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"spotify pauses", update(state("spotify", contract.Paused, "B")), "itunes", "A", contract.Playing, true},
		{"spotify resumes", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"spotify stops", update(state("spotify", contract.Stopped, "")), "itunes", "A", contract.Playing, true},
	})
}

// With nothing playing, the last thing that was is still the most useful
// answer: "paused on B", not silence.
func TestWithNothingPlayingTheMostRecentIsStillCurrent(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"itunes pauses", update(state("itunes", contract.Paused, "A")), "spotify", "B", contract.Playing, false},
		{"spotify pauses", update(state("spotify", contract.Paused, "B")), "spotify", "B", contract.Paused, true},
	})
}

// Pausing is not doing something. A source must not become current by
// stopping.
func TestPausingDoesNotMakeASourceCurrent(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"itunes pauses", update(state("itunes", contract.Paused, "A")), "itunes", "A", contract.Paused, true},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"spotify pauses", update(state("spotify", contract.Paused, "B")), "spotify", "B", contract.Paused, true},
		{"itunes reports its pause again", update(state("itunes", contract.Paused, "A")), "spotify", "B", contract.Paused, false},
	})
}

// Seen on 2026-10-11: the Spotify driver started, reported that nothing was
// playing, and displaced the track paused in iTunes.
func TestASourceWithNothingLoadedDoesNotDisplaceAPausedTrack(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"itunes pauses", update(state("itunes", contract.Paused, "A")), "itunes", "A", contract.Paused, true},
		{"spotify reports nothing", update(state("spotify", contract.Stopped, "")), "itunes", "A", contract.Paused, false},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"spotify stops", update(state("spotify", contract.Stopped, "")), "itunes", "A", contract.Paused, true},
	})
}

// A driver that dies leaves its last state retained on the broker saying
// "playing". Its last will is what says otherwise.
func TestASourceThatGoesOfflineStopsCounting(t *testing.T) {
	run(t, []step{
		{"itunes starts", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"spotify starts", update(state("spotify", contract.Playing, "B")), "spotify", "B", contract.Playing, true},
		{"spotify's driver dies", presence("spotify", false), "itunes", "A", contract.Playing, true},
		{"it comes back, still on B", presence("spotify", true), "spotify", "B", contract.Playing, true},
		{"itunes's driver dies", presence("itunes", false), "spotify", "B", contract.Playing, false},
		{"and spotify's", presence("spotify", false), "", "", "", true},
	})
}

func TestPresenceMayArriveBeforeState(t *testing.T) {
	run(t, []step{
		{"online, nothing said yet", presence("itunes", true), "", "", "", false},
		{"then its state", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
	})
}

func TestOfflineBeforeAnyStateKeepsTheSourceOut(t *testing.T) {
	run(t, []step{
		{"offline first", presence("itunes", false), "", "", "", false},
		{"a stale retained state", update(state("itunes", contract.Playing, "A")), "", "", "", false},
		{"it really comes online", presence("itunes", true), "itunes", "A", contract.Playing, true},
	})
}

// When nowplayd starts it reads every source's retained state at once. Which
// arrives first means nothing, so the senders' own times decide.
func TestRetainedStatesAtStartupAreOrderedByWhenTheyHappened(t *testing.T) {
	older := state("itunes", contract.Playing, "Older")
	older.ChangedAt = epoch.Add(-10 * time.Minute)
	newer := state("spotify", contract.Playing, "Newer")
	newer.ChangedAt = epoch.Add(-1 * time.Minute)

	// The newer one is delivered first; the older one must not win by
	// arriving last.
	run(t, []step{
		{"newer arrives first", update(newer), "spotify", "Newer", contract.Playing, true},
		{"older arrives second", update(older), "spotify", "Newer", contract.Playing, false},
	})
	run(t, []step{
		{"older arrives first", update(older), "itunes", "Older", contract.Playing, true},
		{"newer arrives second", update(newer), "spotify", "Newer", contract.Playing, true},
	})
}

// A driver whose clock runs fast must not hold the stand forever.
func TestASendersClockCannotPutItInTheFuture(t *testing.T) {
	fast := state("itunes", contract.Playing, "Fast clock")
	fast.ChangedAt = epoch.Add(24 * time.Hour)

	run(t, []step{
		{"a source from tomorrow", update(fast), "itunes", "Fast clock", contract.Playing, true},
		{"something actually newer", update(state("spotify", contract.Playing, "Now")), "spotify", "Now", contract.Playing, true},
	})
}

// A driver's state names its cover by hash, so a new cover is a new state.
func TestADifferentCoverIsAChangeAndTheSameCoverIsNot(t *testing.T) {
	withArt := func(sha string) contract.State {
		st := state("itunes", contract.Playing, "A")
		st.Art = &contract.Art{SHA256: sha, MIME: "image/png"}
		return st
	}
	run(t, []step{
		{"starts with no cover", update(state("itunes", contract.Playing, "A")), "itunes", "A", contract.Playing, true},
		{"its cover is known", update(withArt("abc")), "itunes", "A", contract.Playing, true},
		{"the same cover again", update(withArt("abc")), "itunes", "A", contract.Playing, false},
		{"a different cover", update(withArt("def")), "itunes", "A", contract.Playing, true},
	})
}

func TestPositionAloneIsNotAChange(t *testing.T) {
	a := New(nil)
	first := state("itunes", contract.Playing, "A")
	a.Update(first)

	later := first
	pos := int64(42000)
	later.PositionMS = &pos
	got, changed := a.Update(later)
	if changed {
		t.Error("a position update was reported as a change")
	}
	if got.PositionMS == nil || *got.PositionMS != 42000 {
		t.Error("the newest position was not kept")
	}
}

func TestTiesDoNotDependOnMapOrder(t *testing.T) {
	for i := 0; i < 50; i++ {
		at := epoch
		a := New(func() time.Time { return at })
		x, y := state("b-source", contract.Playing, "X"), state("a-source", contract.Playing, "Y")
		x.ChangedAt, y.ChangedAt = epoch, epoch
		a.Update(x)
		got, _ := a.Update(y)
		if got.Source != "a-source" {
			t.Fatalf("run %d: a tie went to %s", i, got.Source)
		}
	}
}

func switched(id string, on bool) func(*Arbiter) (*contract.State, bool) {
	return func(a *Arbiter) (*contract.State, bool) { return a.Power(id, on) }
}

// The needle lifted between sides, and a track left paused on another player:
// the reason there is a beacon at all (ADR-0023).
func TestASourceSwitchedOnHoldsThroughItsSilence(t *testing.T) {
	run(t, []step{
		{"itunes is left paused", update(state("itunes", contract.Paused, "Left Paused")), "itunes", "Left Paused", contract.Paused, true},
		{"the turntable is switched on", switched("turntable", true), "itunes", "Left Paused", contract.Paused, false},
		{"its driver hears nothing yet", update(state("turntable", contract.Stopped, "")), "turntable", "", contract.Stopped, true},
		{"side one", update(state("turntable", contract.Playing, "Side One")), "turntable", "Side One", contract.Playing, true},
		{"the needle is lifted", update(state("turntable", contract.Stopped, "")), "turntable", "", contract.Stopped, true},
		{"side two", update(state("turntable", contract.Playing, "Side Two")), "turntable", "Side Two", contract.Playing, true},
	})
}

func TestASourceSwitchedOnHoldsAgainstOnePlaying(t *testing.T) {
	run(t, []step{
		{"the turntable is switched on", switched("turntable", true), "", "", "", false},
		{"a record plays", update(state("turntable", contract.Playing, "Record")), "turntable", "Record", contract.Playing, true},
		{"itunes starts, more recently", update(state("itunes", contract.Playing, "Elsewhere")), "turntable", "Record", contract.Playing, false},
		{"the record ends", update(state("turntable", contract.Stopped, "")), "turntable", "", contract.Stopped, true},
	})
}

// What the turntable's driver hears when the turntable is off is whatever
// else is on the same wire, and it is not a record.
func TestASourceSwitchedOffIsNotHeardWhateverItReports(t *testing.T) {
	run(t, []step{
		{"itunes is paused", update(state("itunes", contract.Paused, "Paused")), "itunes", "Paused", contract.Paused, true},
		{"the turntable is off", switched("turntable", false), "itunes", "Paused", contract.Paused, false},
		{"and its driver names a track", update(state("turntable", contract.Playing, "Not A Record")), "itunes", "Paused", contract.Paused, false},
		{"switched on, it was playing all along", switched("turntable", true), "turntable", "Not A Record", contract.Playing, true},
		{"switched off, it is gone", switched("turntable", false), "itunes", "Paused", contract.Paused, true},
	})
}

func TestOnlyASourceSwitchedOffLeavesNothingCurrent(t *testing.T) {
	run(t, []step{
		{"a record plays", update(state("turntable", contract.Playing, "Record")), "turntable", "Record", contract.Playing, true},
		{"and is switched off", switched("turntable", false), "", "", "", true},
	})
}

// A beacon nobody has heard from is no beacon: the rule is the one there was
// before, which is also what a source without a beacon gets.
func TestASourceWhoseSwitchIsUnknownIsArbitratedAsBefore(t *testing.T) {
	run(t, []step{
		{"itunes is paused", update(state("itunes", contract.Paused, "Paused")), "itunes", "Paused", contract.Paused, true},
		{"the turntable hears nothing", update(state("turntable", contract.Stopped, "")), "itunes", "Paused", contract.Paused, false},
		{"a record plays", update(state("turntable", contract.Playing, "Record")), "turntable", "Record", contract.Playing, true},
		{"the needle is lifted", update(state("turntable", contract.Stopped, "")), "itunes", "Paused", contract.Paused, true},
	})
}

// The switch says the turntable is on. It does not say anyone is listening
// to it: with its driver gone there is nothing to hold with.
func TestASourceSwitchedOnButOfflineDoesNotHold(t *testing.T) {
	run(t, []step{
		{"itunes plays", update(state("itunes", contract.Playing, "Elsewhere")), "itunes", "Elsewhere", contract.Playing, true},
		{"the turntable is on and silent", switched("turntable", true), "itunes", "Elsewhere", contract.Playing, false},
		{"its driver reports", update(state("turntable", contract.Stopped, "")), "turntable", "", contract.Stopped, true},
		{"its driver goes", presence("turntable", false), "itunes", "Elsewhere", contract.Playing, true},
		{"and comes back", presence("turntable", true), "turntable", "", contract.Stopped, true},
	})
}
