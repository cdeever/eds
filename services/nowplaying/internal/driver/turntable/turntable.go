// Package turntable is the driver for a record on a platter.
//
// A turntable has nothing to ask. The driver listens instead: it takes the
// line-level signal the amplifier sends to a tape deck, and every so often
// hands a few seconds of it to a recognizer that names the track (ADR-0021).
//
// Three rules of its own:
//
//   - Silence is the only thing it knows for certain. Music it cannot name is
//     reported as playing, with no track; it is not made up, and it is not
//     called stopped.
//   - A track is not replaced on one answer. A recognizer is sometimes wrong
//     for a moment, and one wrong answer would repaint the room.
//   - It names no cover. What it hears has no picture; nowplayd finds one.
package turntable

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/events"
)

// Rate is the sample rate the driver listens at. A recognizer wants no more,
// and it is what keeps twelve seconds small.
const Rate = 16000

// blockSamples is a tenth of a second, the grain everything is counted in.
// The driver's clock is the audio itself: so many blocks heard, not so much
// time passed. A capture that stalls then stalls the counting with it,
// instead of being mistaken for silence.
const blockSamples = Rate / 10

// Capture is where the audio comes from.
type Capture interface {
	// Open starts listening. The stream is signed 16-bit little-endian mono
	// at Rate, and ends when the device does.
	Open(ctx context.Context) (io.ReadCloser, error)
}

// Driver follows one turntable.
type Driver struct {
	Source     string // the source id this driver reports as
	Capture    Capture
	Recognizer Recognizer
	Sink       driver.Sink
	Logger     *slog.Logger

	// Interval is how often a clip is sent while music plays.
	Interval time.Duration
	// SilenceDB is the level, in dBFS, below which nothing is playing.
	SilenceDB float64
	// StopAfter is how much silence means the record has stopped. Longer
	// than the gap between two tracks, or every gap would be a stop.
	StopAfter time.Duration

	Now func() time.Time

	// after is time.After, replaceable so tests do not wait out a backoff.
	after func(time.Duration) <-chan time.Time
}

// Run listens until ctx ends. A capture device that is missing or goes away
// is waited for, not given up on: it is a cable, and cables are put back.
func (d *Driver) Run(ctx context.Context) error {
	defer d.Sink.Online(false)

	var (
		last    *contract.State
		backoff = time.Second
		told    bool
	)
	for {
		heard, err := d.listen(ctx, &last)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// A capture that ran for a while and then ended is a new failure,
		// not the old one again.
		if heard {
			backoff, told = time.Second, false
		}
		// Said once for each time it goes away. An interface left unplugged
		// is tried twice a minute, and that is not twice a minute of news.
		if !told {
			told = true
			d.Sink.Online(false)
			d.logger().Warn("not listening to the turntable", events.Key, "turntable.capture_lost", "reason", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.sleep(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// answer is what a recognizer said about one clip.
type answer struct {
	match Match
	err   error
}

// listen follows one capture until it ends. It reports whether anything was
// heard at all, which is what tells a device that went away from one that was
// never there.
func (d *Driver) listen(ctx context.Context, last **contract.State) (heard bool, err error) {
	stream, err := d.Capture.Open(ctx)
	if err != nil {
		return false, err
	}
	// Closing is also what stops the reader below.
	defer stream.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	blocks := make(chan []int16)
	readErr := make(chan error, 1)
	go func() {
		raw := make([]byte, blockSamples*2)
		for {
			if _, err := io.ReadFull(stream, raw); err != nil {
				readErr <- err
				return
			}
			block := make([]int16, blockSamples)
			for i := range block {
				block[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
			}
			select {
			case blocks <- block:
			case <-ctx.Done():
				return
			}
		}
	}()

	f := newFollower(d.interval(), d.stopAfter(), d.silenceDB())
	answers := make(chan answer, 1)
	failing := false

	for {
		select {
		case <-ctx.Done():
			return heard, ctx.Err()

		case err := <-readErr:
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				err = errors.New("the capture ended")
			}
			return heard, err

		case block := <-blocks:
			if !heard {
				heard = true
				d.Sink.Online(true)
			}
			if clip := f.hear(block); clip != nil {
				go func() {
					m, err := d.Recognizer.Recognize(ctx, clip)
					answers <- answer{m, err}
				}()
			}

		case a := <-answers:
			switch {
			case a.err == nil, errors.Is(a.err, ErrNoMatch):
				if failing {
					failing = false
					d.logger().Info("the recognizer is answering again", events.Key, "turntable.recognizer_back")
				}
			default:
				// Said once for a run of failures. It is asked several times
				// a minute, and the log store is not the place to count them.
				if !failing {
					failing = true
					d.logger().Warn("the recognizer could not be asked", events.Key, "turntable.recognizer_failed", "reason", a.err)
				}
			}
			f.answer(a.match, a.err)
		}

		d.report(f, last)
	}
}

// report publishes what the follower now believes, if that is news.
func (d *Driver) report(f *follower, last **contract.State) {
	state := contract.State{V: contract.Version, Source: d.Source, Player: "turntable", State: contract.Stopped}
	playing, track := f.state()
	if playing {
		state.State = contract.Playing
		state.Track = track
	}

	if *last != nil {
		state.ChangedAt = (*last).ChangedAt
	}
	if *last == nil || (*last).State != state.State || key(*last) != key(&state) {
		state.ChangedAt = d.now()
	}
	if contract.Same(*last, &state) {
		return
	}
	if playing && track == nil {
		d.logger().Info("the turntable is playing something the recognizer does not know",
			events.Key, "turntable.unrecognized")
	}
	d.Sink.Publish(driver.Update{State: state})
	*last = &state
}

func key(s *contract.State) string {
	if s == nil || s.Track == nil {
		return ""
	}
	return s.Track.Key()
}

func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Driver) sleep(wait time.Duration) <-chan time.Time {
	if d.after != nil {
		return d.after(wait)
	}
	return time.After(wait)
}

func (d *Driver) interval() time.Duration {
	if d.Interval > 0 {
		return d.Interval
	}
	return 20 * time.Second
}

func (d *Driver) stopAfter() time.Duration {
	if d.StopAfter > 0 {
		return d.StopAfter
	}
	return 20 * time.Second
}

func (d *Driver) silenceDB() float64 {
	if d.SilenceDB != 0 {
		return d.SilenceDB
	}
	return DefaultSilenceDB
}

func (d *Driver) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// String is for the line the agent logs when it starts.
func (d *Driver) String() string {
	return fmt.Sprintf("a clip every %s, silent below %.0f dBFS, stopped after %s of it",
		d.interval(), d.silenceDB(), d.stopAfter())
}
