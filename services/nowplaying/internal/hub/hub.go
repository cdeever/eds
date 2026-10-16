// Package hub turns what arrives from the sources into decisions: which track
// is current, and which image - if the source sent one - is its cover.
//
// It sits between the bus and the publisher. The arbiter says who wins; this
// keeps the covers each source has sent, and knows that a state and its cover
// are two messages which need not arrive in the order they were sent.
package hub

import (
	"log/slog"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/arbiter"
	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
)

// Decision is what the publisher is asked to put on the bus: the winner, and
// the cover its own source supplied, if it did.
type Decision struct {
	Winner *contract.State
	Cover  *current.Image
	// Idle says the winner has been paused, or there has been no winner,
	// for long enough that nothing is playing as far as a room is concerned
	// (ADR-0024). The winner is still named: it is who a command is for.
	Idle bool
	// On says some source's equipment is switched on (ADR-0023): somebody
	// is in the room to play something, whether or not anything is playing
	// yet. It is what an idle room is waiting for, or is not (ADR-0028).
	On bool
}

// Hub holds the state of every source.
type Hub struct {
	Prefix string
	Logger *slog.Logger
	// Submit takes a decision to be published. It must not block.
	Submit func(Decision)

	// Grace is how long a state that names a cover is held for that cover to
	// arrive, before the decision goes ahead without it.
	//
	// A driver publishes the image first and the state that names it second.
	// But both are retained, and a broker handing a new subscriber its
	// retained messages promises no order: found by deploying, where nowplayd
	// started, was given the state first, took the track to have no cover,
	// fetched one from a catalog, published it - and was given the
	// player's own cover twenty-four milliseconds later.
	Grace time.Duration

	// IdleAfter is how long a track may sit paused, or nothing be reporting,
	// before the decision is made again as idle (ADR-0024). Zero is never.
	IdleAfter time.Duration
	// StoppedIdleAfter is the same for a winner that says "stopped": a
	// source held while its equipment is on (ADR-0023), with nothing on it.
	// It is a wait of its own because it was once much the longer, to keep
	// the sleeve while a record was turned over; it is now short (ADR-0026).
	StoppedIdleAfter time.Duration

	// PresenceLapse is how long an "online" is believed without being
	// said again, by a driver or a beacon (ADR-0034). Everything repeats its
	// presence every thirty seconds, so a stored online whose sender is
	// gone, kept by the broker across a restart, lapses here. Zero is never.
	PresenceLapse time.Duration

	// After schedules f, and Now is the time. They exist so tests can hold
	// the clock.
	After func(d time.Duration, f func())
	Now   func() time.Time

	mu      sync.Mutex
	arb     *arbiter.Arbiter
	covers  map[string]*current.Image // by source
	waiting uint64                    // changes whenever a wait is started or overtaken
	heard   map[string]uint64         // by presence: changes whenever it is said

	// What has been quiet - paused, or nobody at all - and since when.
	quiet      bool
	quietKey   string
	quietSince time.Time

	// The last decision handed on, so that a switch which changes no winner
	// can still change what an idle room is told.
	sent bool
	last Decision
}

// New builds a Hub.
func New(prefix string, submit func(Decision), logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		Prefix: prefix,
		Logger: logger,
		Submit: submit,
		Grace:  2 * time.Second,
		After:  func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		Now:    time.Now,
		arb:    arbiter.New(nil),
		covers: make(map[string]*current.Image),
		heard:  make(map[string]uint64),
	}
}

// Handle takes one message from a source topic.
func (h *Hub) Handle(msg bus.Message) {
	id, kind, ok := contract.ParseSourceTopic(h.Prefix, msg.Topic)
	if !ok {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var (
		winner  *contract.State
		changed bool
	)
	switch kind {
	case "state":
		st, err := contract.Decode(msg.Payload)
		if err != nil {
			// Ignored, and the source's last good state stands - the same
			// answer the firmware gives a scene it cannot render.
			h.Logger.Warn("ignored a state", "source", id, "error", err)
			return
		}
		if st.Source != id {
			// The topic is what the broker checked; the payload is only what
			// the sender wrote. They have to agree.
			h.Logger.Warn("ignored a state claiming another source", "topic", msg.Topic, "claims", st.Source)
			return
		}
		winner, changed = h.arb.Update(st)

	case "status":
		online := string(msg.Payload) == contract.Online
		h.hold("source:"+id, online, func() {
			h.Logger.Warn("a source's presence lapsed, unrepeated", "source", id, "after", h.PresenceLapse)
			if winner, changed := h.arb.Presence(id, false); changed {
				h.decide(winner)
			}
		})
		winner, changed = h.arb.Presence(id, online)

	case "art":
		if image := current.NewImage(msg.Payload); image != nil {
			h.covers[id] = image
		} else {
			delete(h.covers, id)
		}
		// A cover is not a decision, but it may be the one the current track
		// is waiting for.
		winner = h.arb.Current()
		changed = winner != nil && winner.Source == id
	}

	if changed {
		h.decide(winner)
	}
}

// Settle is called once the bus is being listened to. Retained messages
// arrive within moments of that, and if none of them made a decision there
// is nobody reporting: every source is offline, or there are none. That is
// decided too, so that it goes idle like any other silence (ADR-0024). It
// would otherwise never be said, since no winner is what a hub starts with,
// and the track that was current before this program started would stand
// on its retained topic, and light a room, until a source came back.
func (h *Hub) Settle() {
	h.After(h.Grace, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if !h.sent {
			h.decide(h.arb.Current())
		}
	})
}

// Power takes what a source's beacon said: that the source's equipment is
// switched on, or is not (ADR-0023).
func (h *Hub) Power(source string, on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// A beacon's on that is not said again is not believed: the source goes
	// back to having no beacon, which changes nothing (ADR-0023).
	h.hold("power:"+source, on, func() {
		h.Logger.Warn("a source's equipment has not said it is on lately; it is no longer believed", "source", source, "after", h.PresenceLapse)
		h.powered(h.arb.PowerUnknown(source))
	})
	h.powered(h.arb.Power(source, on))
}

// hold notes that a presence was just said. An online lapses, calling
// lapse with the lock held, unless it is said again within PresenceLapse;
// anything else simply cancels the lapse. Called with the lock.
func (h *Hub) hold(key string, online bool, lapse func()) {
	h.heard[key]++
	if !online || h.PresenceLapse <= 0 {
		return
	}
	said := h.heard[key]
	h.After(h.PresenceLapse, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.heard[key] == said {
			lapse()
		}
	})
}

// powered acts on what a switch did to the arbiter. Called with the lock.
func (h *Hub) powered(winner *contract.State, changed bool) {
	if changed {
		h.decide(winner)
		return
	}
	// The winner is who it was. But an idle room is told whether anything
	// is switched on, and that may just have changed: a turntable switched
	// on whose driver has nothing to say yet (ADR-0028).
	if h.sent && h.last.Idle && h.last.On != h.arb.AnyOn() {
		h.send(h.last)
	}
}

// send hands a decision on, saying with it whether anything is switched on.
// Called with the lock.
func (h *Hub) send(d Decision) {
	d.On = h.arb.AnyOn()
	h.sent, h.last = true, d
	h.Submit(d)
}

// decide submits the decision for a winner, unless the winner names a cover
// that has not arrived, in which case it waits for it. Called with the lock.
func (h *Hub) decide(winner *contract.State) {
	h.waiting++ // whatever was being waited for has been overtaken

	if winner == nil {
		h.submit(nil, nil)
		return
	}

	cover := h.covers[winner.Source]
	if winner.Art == nil || (cover != nil && cover.SHA256 == winner.Art.SHA256) {
		h.submit(winner, cover)
		return
	}

	// The state names a cover this hub does not hold: either it is still on
	// its way, or the one held belongs to the track before.
	ticket := h.waiting
	h.After(h.Grace, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.waiting != ticket {
			return // it arrived, or the track moved on
		}
		// It did not come. Go ahead; the publisher will look one up.
		h.Logger.Info("a cover a source named did not arrive", "source", winner.Source, "waited", h.Grace)
		now := h.arb.Current()
		var cover *current.Image
		if now != nil {
			cover = h.covers[now.Source]
		}
		h.submit(now, cover)
	})
}

// submit hands a decision on, and keeps the time for it going idle: a track
// left paused, or nobody reporting, is not something playing for ever
// (ADR-0024). Called with the lock.
//
// Stopped has a wait of its own. A source that says "stopped" and is still
// the winner is one that is being held (ADR-0023): a turntable switched on
// with nothing playing, which is not something to light a room for.
func (h *Hub) submit(winner *contract.State, cover *current.Image) {
	wait := time.Duration(0)
	switch {
	case winner == nil || winner.State == contract.Paused:
		wait = h.IdleAfter
	case winner.State == contract.Stopped:
		wait = h.StoppedIdleAfter
	}
	if wait <= 0 {
		h.quiet = false
		h.send(Decision{Winner: winner, Cover: cover})
		return
	}

	// Since when is asked once, when the quiet began, and not each time the
	// same paused track is decided again: its cover arriving late is not the
	// pause starting over.
	key := ""
	if winner != nil {
		key = winner.Source + "\x00" + string(winner.State) + "\x00"
		if winner.Track != nil {
			key += winner.Track.Key()
		}
	}
	now := h.Now()
	if !h.quiet || h.quietKey != key {
		h.quiet, h.quietKey, h.quietSince = true, key, now
		// A pause is dated by whoever paused, when that is in the past: it
		// is how a track paused an hour ago is not lit again for a quarter
		// of a minute each time this program starts.
		if winner != nil && !winner.ChangedAt.IsZero() && winner.ChangedAt.Before(now) {
			h.quietSince = winner.ChangedAt
		}
	}

	left := wait - now.Sub(h.quietSince)
	if left <= 0 {
		h.send(Decision{Winner: winner, Idle: true})
		return
	}

	h.send(Decision{Winner: winner, Cover: cover})
	ticket := h.waiting
	h.After(left, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.waiting != ticket || !h.quiet {
			return // something was decided since
		}
		h.send(Decision{Winner: winner, Idle: true})
	})
}
