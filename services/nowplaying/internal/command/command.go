// Package command passes a controller's command to the player it is meant
// for (ADR-0020).
//
// A controller - a screen with buttons on it - knows there is music and
// nothing about where it comes from. It says "next" on one topic. Which
// player that means is the question nowplayd already answers for everyone:
// the one whose track is current. So the command is passed to that source's
// own command topic, where its driver is listening if it takes commands at
// all.
//
// Nothing is decided here beyond that. A command is not judged, changed or
// kept: it is passed on as it arrived, or dropped.
package command

import (
	"log/slog"
	"sync"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// Bus is the part of the bus this package uses.
type Bus interface {
	Publish(topic string, payload []byte, retained bool) error
}

// Router passes commands to whichever source is current.
type Router struct {
	Prefix string
	Logger *slog.Logger

	mu     sync.Mutex
	bus    Bus
	source string
}

// SetBus says where commands are passed on to. Until it is called they are
// dropped: the connection that delivers a command is the one that would
// carry it on, and it delivers before it has been handed over.
func (r *Router) SetBus(b Bus) {
	r.mu.Lock()
	r.bus = b
	r.mu.Unlock()
}

// SetCurrent says which source the current track came from, as it was last
// published: "" when nothing is reporting. A paused track is still its
// source's, which is what lets a pause be undone from where it was done.
func (r *Router) SetCurrent(source string) {
	r.mu.Lock()
	r.source = source
	r.mu.Unlock()
}

// Handle takes one message from the command topic.
func (r *Router) Handle(msg bus.Message) {
	// A command is never retained, by anyone who follows the contract. One
	// that is has been put there by something that does not, and would be
	// handed over again on every reconnect: a track skipped each time the
	// broker restarts.
	if msg.Retained {
		r.logger().Warn("dropped a retained command; a command is never retained", "topic", msg.Topic)
		return
	}
	c, err := contract.DecodeCommand(msg.Payload)
	if err != nil {
		r.logger().Warn("dropped a command", "error", err)
		return
	}

	r.mu.Lock()
	out, source := r.bus, r.source
	r.mu.Unlock()
	if out == nil {
		r.logger().Info("dropped a command: not connected yet", "command", c.Command, "from", c.From)
		return
	}
	if source == "" {
		// Not kept for when something does play: by then it is not what
		// anyone meant.
		r.logger().Info("dropped a command: nothing is current", "command", c.Command, "from", c.From)
		return
	}

	// The payload as it came, so that a field this build does not know
	// reaches a driver that does. Whether the source takes commands is not
	// asked: one that does not is simply not listening.
	if err := out.Publish(contract.SourceCommandTopic(r.Prefix, source), msg.Payload, false); err != nil {
		r.logger().Warn("could not pass a command on", "command", c.Command, "source", source, "error", err)
		return
	}
	r.logger().Info("command", "command", c.Command, "from", c.From, "source", source)
}

func (r *Router) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
