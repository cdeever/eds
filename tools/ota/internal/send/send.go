// Package send offers a build to one device and sees it through (ADR-0035).
//
// The device leads. It answers the offer with the first byte it wants, and
// every piece with the next; the sender sends that, and nothing else. A piece
// lost, sent twice or cut off by a reconnection is put right by the device
// asking again for where it is. All the sender adds is a deadline: a piece
// that is not answered is sent again, and a device that stops answering is
// given up on.
package send

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cdeever/eds/tools/ota/internal/image"
)

// ContractVersion is the "v" of an offer and of every answer.
const ContractVersion = 1

// Chunk is how much of the image goes in one piece. Eight kilobytes is a few
// of each device's MQTT buffers, and about two hundred pieces for a build.
const Chunk = 8 * 1024

// Topics are one device's three ota topics.
type Topics struct {
	Offer, Chunk, State string
}

// TopicsFor names the topics of the device kind/id under prefix, e.g.
// ("eds", "lightstand", "lp-stand-01").
func TopicsFor(prefix, kind, id string) Topics {
	base := prefix + "/" + kind + "/" + id + "/ota"
	return Topics{Offer: base, Chunk: base + "/chunk", State: base + "/state"}
}

// Link is the broker, as the sender uses it.
type Link interface {
	// Publish sends at QoS 1, never retained.
	Publish(topic string, payload []byte) error
	// Answers delivers what the device publishes on its state topic. The
	// subscription is made before Send is called.
	Answers() <-chan []byte
}

// Answer is the device's word on its state topic.
type Answer struct {
	V       int    `json:"v"`
	State   string `json:"state"`
	Next    *int64 `json:"next,omitempty"`
	Version string `json:"version,omitempty"`
	ELF     string `json:"elf,omitempty"`
	Why     string `json:"why,omitempty"`
}

// Offer is what is published on the offer topic.
type Offer struct {
	V       int    `json:"v"`
	Project string `json:"project"`
	Size    int    `json:"size"`
	ELF     string `json:"elf"`
	Version string `json:"version"`
}

// Waits are how long the sender gives the device at each step.
type Waits struct {
	Ready   time.Duration // for its answer to the offer
	Piece   time.Duration // for its answer to a piece, before sending it again
	Tries   int           // pieces sent without an answer before giving up
	Restart time.Duration // from "rebooting" to "running"
}

// DefaultWaits are generous for WiFi on a bad day. Restart is the device's
// two-minute probation and some: a build that has not reached the broker by
// then has been undone.
var DefaultWaits = Waits{Ready: 30 * time.Second, Piece: 10 * time.Second, Tries: 6, Restart: 3 * time.Minute}

// Result is how it ended.
type Result struct {
	Running  image.Desc // what the device said it runs afterward (Project unset)
	Kept     bool       // the build sent; false means it was undone
	Sent     int        // bytes of image published, repeats included
	Duration time.Duration
}

// ErrRefused is wrapped by an error that is the device's refusal.
var ErrRefused = errors.New("refused")

// Send offers img, described by desc, on t, and follows it until the device
// says what it runs. progress, if not nil, is told each byte the device
// asks for.
func Send(ctx context.Context, link Link, t Topics, img []byte, desc image.Desc, w Waits, progress func(next, size int)) (Result, error) {
	start := time.Now()
	res := Result{}

	offer, err := json.Marshal(Offer{V: ContractVersion, Project: desc.Project, Size: len(img), ELF: desc.ELF, Version: desc.Version})
	if err != nil {
		return res, err
	}
	drain(link.Answers())
	if err := link.Publish(t.Offer, offer); err != nil {
		return res, fmt.Errorf("offer: %w", err)
	}

	// The answer to the offer.
	a, err := await(ctx, link, w.Ready, func(a Answer) bool {
		return a.State == "ready" || a.State == "refused"
	})
	if err != nil {
		return res, fmt.Errorf("the device did not answer the offer in %s: is it on, and running a build that takes updates? (%w)", w.Ready, err)
	}
	if a.State == "refused" {
		return res, fmt.Errorf("%w: %s", ErrRefused, a.Why)
	}

	// The pieces, as asked for.
	sent := -1
	next := int(*a.Next)
	tries := 0
	for {
		if next >= len(img) {
			// Everything asked for has been sent; the device's answer to the
			// last piece is "rebooting" or "refused", which ends this.
			next = len(img)
		}
		if next != sent && next < len(img) {
			if progress != nil {
				progress(next, len(img))
			}
			if err := link.Publish(t.Chunk, piece(img, next)); err != nil {
				return res, fmt.Errorf("piece at %d: %w", next, err)
			}
			res.Sent += min(Chunk, len(img)-next)
			sent = next
			tries = 1
		}

		a, err := await(ctx, link, w.Piece, func(a Answer) bool {
			return a.State == "receiving" || a.State == "rebooting" || a.State == "refused"
		})
		switch {
		case errors.Is(err, errTimeout):
			if tries >= w.Tries {
				return res, fmt.Errorf("the device stopped answering at byte %d of %d", sent, len(img))
			}
			if sent >= 0 && sent < len(img) {
				if err := link.Publish(t.Chunk, piece(img, sent)); err != nil {
					return res, fmt.Errorf("piece at %d: %w", sent, err)
				}
				res.Sent += min(Chunk, len(img)-sent)
			}
			tries++
			continue
		case err != nil:
			return res, err
		}

		switch a.State {
		case "refused":
			return res, fmt.Errorf("%w: %s", ErrRefused, a.Why)
		case "rebooting":
			if progress != nil {
				progress(len(img), len(img))
			}
			if a.ELF != "" && a.ELF != desc.ELF {
				return res, fmt.Errorf("the device is restarting into %s, not the build sent (%s)", a.ELF, desc.ELF)
			}
			return restarted(ctx, link, w, desc, res, start)
		}
		// "receiving". An answer for a piece already answered says where the
		// device was then; the one that matters is the latest, which a
		// repeat of the piece just sent would only ask for again.
		if a.Next != nil && int(*a.Next) != sent {
			next = int(*a.Next)
		}
	}
}

// restarted waits for the device to say what it came back running.
func restarted(ctx context.Context, link Link, w Waits, desc image.Desc, res Result, start time.Time) (Result, error) {
	a, err := await(ctx, link, w.Restart, func(a Answer) bool { return a.State == "running" })
	res.Duration = time.Since(start)
	if err != nil {
		return res, fmt.Errorf("the device restarted and has not been heard from in %s", w.Restart)
	}
	res.Running = image.Desc{Version: a.Version, ELF: a.ELF}
	res.Kept = a.ELF == desc.ELF
	return res, nil
}

// piece is the chunk message for the bytes from at: four bytes of offset,
// big-endian, then up to Chunk bytes of the image.
func piece(img []byte, at int) []byte {
	n := min(Chunk, len(img)-at)
	b := make([]byte, 4+n)
	binary.BigEndian.PutUint32(b, uint32(at))
	copy(b[4:], img[at:at+n])
	return b
}

var errTimeout = errors.New("no answer")

// await returns the first answer that want accepts, skipping what does not
// parse, is another version, or is not wanted.
func await(ctx context.Context, link Link, d time.Duration, want func(Answer) bool) (Answer, error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return Answer{}, ctx.Err()
		case <-timer.C:
			return Answer{}, errTimeout
		case raw, ok := <-link.Answers():
			if !ok {
				return Answer{}, errors.New("the broker connection closed")
			}
			var a Answer
			if json.Unmarshal(raw, &a) != nil || a.V != ContractVersion || !want(a) {
				continue
			}
			if (a.State == "ready" || a.State == "receiving") && a.Next == nil {
				continue
			}
			return a, nil
		}
	}
}

func drain(ch <-chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
