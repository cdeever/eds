// Package arbiter decides which of several sources is the current track.
//
// Several drivers may report at once - a paused iTunes, a Spotify playing on
// a phone - and a subscriber wants one answer. The rule is the one a person
// means by "what's playing": the source whose track most recently started or
// changed wins, and when it stops, whatever else is still playing takes over
// (ADR-0008).
//
// It is pure: no clock of its own, no network, no goroutines. Everything it
// does is a function of the events it is handed, which is what lets the rule
// be tested as a table of sequences.
package arbiter

import (
	"sort"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

type source struct {
	state   contract.State
	online  bool
	hasSeen bool

	// since is when this source last did something a listener would notice:
	// started playing, or changed track. It is nowplayd's clock, not the
	// driver's, because drivers run on different machines and the order of
	// two of them is only meaningful by one clock.
	since time.Time
}

// Arbiter holds the last known state of every source.
type Arbiter struct {
	now     func() time.Time
	sources map[string]*source
	last    *contract.State
}

// New builds an Arbiter. now is the clock; tests supply their own.
func New(now func() time.Time) *Arbiter {
	if now == nil {
		now = time.Now
	}
	return &Arbiter{now: now, sources: make(map[string]*source)}
}

func (a *Arbiter) get(id string) *source {
	s, ok := a.sources[id]
	if !ok {
		// A source is taken to be online until it says otherwise. Its state
		// and its presence are separate retained messages and may arrive in
		// either order; a state with no presence yet is not a dead source.
		s = &source{online: true}
		a.sources[id] = s
	}
	return s
}

// Update records a source's state and reports the current track and whether
// it changed.
func (a *Arbiter) Update(st contract.State) (*contract.State, bool) {
	s := a.get(st.Source)
	now := a.now()

	switch {
	case !s.hasSeen:
		// The first sight of a source is usually a retained message, read
		// when nowplayd starts, and several arrive together. Their receive
		// times say nothing about which track started last, so here - and
		// only here - the sender's own time is believed, held to the present
		// so a clock running fast cannot win forever.
		s.since = st.ChangedAt
		if s.since.IsZero() || s.since.After(now) {
			s.since = now
		}
	case noticeable(s.state, st):
		s.since = now
	}

	s.state = st
	s.hasSeen = true
	return a.decide()
}

// noticeable reports whether a listener would say something just happened:
// playback started, or the track changed while playing. Pausing is not it - a
// source that pauses does not become more current by doing so.
func noticeable(before, after contract.State) bool {
	if after.State != contract.Playing {
		return false
	}
	if before.State != contract.Playing {
		return true
	}
	return trackKey(before) != trackKey(after)
}

func trackKey(s contract.State) string {
	if s.Track == nil {
		return ""
	}
	return s.Track.Key()
}

// Presence records a source coming or going, as its status topic says, and
// reports the current track and whether it changed.
func (a *Arbiter) Presence(id string, online bool) (*contract.State, bool) {
	a.get(id).online = online
	return a.decide()
}

// Current is the current track, or nil when no source has anything to say.
func (a *Arbiter) Current() *contract.State {
	return a.last
}

func (a *Arbiter) decide() (*contract.State, bool) {
	winner := a.pick()
	if contract.Same(winner, a.last) {
		// Keep the newest copy all the same, so a position or timestamp that
		// moved is what Current returns - without calling it a change.
		a.last = winner
		return winner, false
	}
	a.last = winner
	return winner, true
}

// pick applies the rule. Among sources that are online and have reported:
// one that is playing beats one that is not, and within either group the most
// recently noticeable wins. Ties are broken by name so the answer does not
// depend on map order.
func (a *Arbiter) pick() *contract.State {
	ids := make([]string, 0, len(a.sources))
	for id, s := range a.sources {
		if s.online && s.hasSeen {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	sort.Slice(ids, func(i, j int) bool {
		x, y := a.sources[ids[i]], a.sources[ids[j]]
		xp, yp := x.state.State == contract.Playing, y.state.State == contract.Playing
		if xp != yp {
			return xp
		}
		if !x.since.Equal(y.since) {
			return x.since.After(y.since)
		}
		return ids[i] < ids[j]
	})

	winner := a.sources[ids[0]].state
	return &winner
}
