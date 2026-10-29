package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/events"
)

// A driver with a switch runs only while it is switched on (ADR-0051). The
// switch is a retained setting on the broker; the agent obeys it, and off
// means the driver's run has ended, so it asks its service nothing at all.
// NP_DRIVERS still says which drivers may run.
//
// Only Spotify has one. iTunes costs nothing to ask, and the turntable asks
// only when it hears music.

// errSwitchedOff is a command for a driver that is switched off. It fails
// and is logged; it is never kept for when the driver is on again.
var errSwitchedOff = errors.New("the driver is switched off")

type driverSwitch struct {
	mu sync.Mutex
	on bool
	// moves counts every move. Two quick moves are one signal on changed,
	// and this is how off-and-on again is still seen as a move.
	moves uint64
	// changed holds one change not yet acted on. A second before the first
	// is taken is the same news: the runner reads where the switch is now.
	changed chan struct{}
}

func newSwitch() *driverSwitch {
	return &driverSwitch{changed: make(chan struct{}, 1)}
}

func (s *driverSwitch) On() bool {
	on, _ := s.state()
	return on
}

func (s *driverSwitch) state() (on bool, moves uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.on, s.moves
}

// set moves the switch and reports whether it moved. The same word again -
// the broker sends the retained setting on every reconnect - is no change.
func (s *driverSwitch) set(on bool) bool {
	s.mu.Lock()
	moved := s.on != on
	s.on = on
	if moved {
		s.moves++
	}
	s.mu.Unlock()
	if moved {
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	return moved
}

// hear takes a message on the switch's topic. Anything that is not a
// setting is off, and said to be unreadable.
func (s *driverSwitch) hear(logger *slog.Logger, source string, msg bus.Message) {
	word, err := contract.DecodeSwitch(msg.Payload)
	if err != nil {
		logger.Warn("a driver's switch could not be read, so it is off", events.Key, "driver.switch_unreadable",
			"source", source, "reason", err)
	}
	if s.set(word == contract.SwitchOn) {
		logger.Info("driver switched "+word, events.Key, "driver.switched", "source", source, "switch", word)
	}
}

// gate is a driver's commands, refused while it is switched off.
func (s *driverSwitch) gate(do func(context.Context, contract.Command) error) func(context.Context, contract.Command) error {
	return func(ctx context.Context, c contract.Command) error {
		if !s.On() {
			return errSwitchedOff
		}
		return do(ctx, c)
	}
}

// runSwitched runs j for as long as its switch is on, and starts it again
// each time it is switched on. It returns only when ctx ends: a driver that
// is switched off is still wanted, and the agent stays up to be switched.
//
// A driver that gives up while on is not started again until it has been
// switched off and on: whatever stopped it needs a person, and the switch is
// how that person says it is done.
func runSwitched(ctx context.Context, logger *slog.Logger, j job, sink driver.Sink) {
	sw := j.switched
	for {
		for !sw.On() {
			select {
			case <-ctx.Done():
				return
			case <-sw.changed:
			}
		}

		running, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- j.run(running, sink) }()

		gaveUp, gaveUpAt := false, uint64(0)
	following:
		for {
			select {
			case <-ctx.Done():
				stop()
				<-done
				return
			case err := <-done:
				stop()
				if ctx.Err() != nil {
					return
				}
				logger.Error("driver stopped", events.Key, "driver.stopped", "driver", j.name, "reason", err)
				sink.Online(false)
				gaveUp = true
				_, gaveUpAt = sw.state()
				break following
			case <-sw.changed:
				if sw.On() {
					continue // off and on again before this saw it: still on
				}
				stop()
				<-done
				sink.Online(false)
				break following
			}
		}

		// Any move since it gave up, ending on, is a switch off and on
		// again, however quickly it was done.
		for gaveUp {
			on, moves := sw.state()
			if on && moves != gaveUpAt {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-sw.changed:
			}
		}
	}
}
