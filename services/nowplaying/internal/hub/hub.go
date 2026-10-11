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

	// After schedules f. It exists so tests can hold the clock.
	After func(d time.Duration, f func())

	mu      sync.Mutex
	arb     *arbiter.Arbiter
	covers  map[string]*current.Image // by source
	waiting uint64                    // changes whenever a wait is started or overtaken
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

// decide submits the decision for a winner, unless the winner names a cover
// that has not arrived, in which case it waits for it. Called with the lock.
func (h *Hub) decide(winner *contract.State) {
	h.waiting++ // whatever was being waited for has been overtaken

	if winner == nil {
		h.Submit(Decision{})
		return
	}

	cover := h.covers[winner.Source]
	if winner.Art == nil || (cover != nil && cover.SHA256 == winner.Art.SHA256) {
		h.Submit(Decision{Winner: winner, Cover: cover})
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
		d := Decision{Winner: now}
		if now != nil {
			d.Cover = h.covers[now.Source]
		}
		h.Submit(d)
	})
}
