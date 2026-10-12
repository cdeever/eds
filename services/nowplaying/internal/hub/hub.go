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
	// fetched one from a catalogue, published it - and was given the
	// player's own cover twenty-four milliseconds later.
	Grace time.Duration

	// IdleAfter is how long a track may sit paused, or nothing be reporting,
	// before the decision is made again as idle (ADR-0024). Zero is never.
	IdleAfter time.Duration

	// After schedules f, and Now is the time. They exist so tests can hold
	// the clock.
	After func(d time.Duration, f func())
	Now   func() time.Time

	mu      sync.Mutex
	arb     *arbiter.Arbiter
	covers  map[string]*current.Image // by source
	waiting uint64                    // changes whenever a wait is started or overtaken

	// What has been quiet - paused, or nobody at all - and since when.
	quiet      bool
	quietKey   string
	quietSince time.Time
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
		winner, changed = h.arb.Presence(id, string(msg.Payload) == contract.Online)

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

// Power takes what a source's beacon said: that the source's equipment is
// switched on, or is not (ADR-0023).
func (h *Hub) Power(source string, on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if winner, changed := h.arb.Power(source, on); changed {
		h.decide(winner)
	}
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
// Stopped is not counted. A source that says "stopped" and is still the
// winner is one that is being held (ADR-0023), and a record between its
// sides is not a room to turn the lights down in.
func (h *Hub) submit(winner *contract.State, cover *current.Image) {
	quiet := winner == nil || winner.State == contract.Paused
	if !quiet || h.IdleAfter <= 0 {
		h.quiet = false
		h.Submit(Decision{Winner: winner, Cover: cover})
		return
	}

	// Since when is asked once, when the quiet began, and not each time the
	// same paused track is decided again: its cover arriving late is not the
	// pause starting over.
	key := ""
	if winner != nil {
		key = winner.Source + "\x00"
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

	left := h.IdleAfter - now.Sub(h.quietSince)
	if left <= 0 {
		h.Submit(Decision{Winner: winner, Idle: true})
		return
	}

	h.Submit(Decision{Winner: winner, Cover: cover})
	ticket := h.waiting
	h.After(left, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.waiting != ticket || !h.quiet {
			return // something was decided since
		}
		h.Submit(Decision{Winner: winner, Idle: true})
	})
}
