// Package current publishes the one current track, complete.
//
// The arbiter says which source wins; this takes that source's state, fills
// in what it lacked, and puts the result on the bus as the current track and
// its cover. It is where the promise to subscribers is kept: while something
// is playing, current carries a title, an album and art, whichever driver
// reported it (ADR-0008).
package current

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// Bus is the part of the bus this package uses.
type Bus interface {
	Publish(topic string, payload []byte, retained bool) error
}

// Resolver finds a track's album and cover in a catalogue.
type Resolver interface {
	Resolve(ctx context.Context, t contract.Track) (resolve.Result, error)
}

// Image is a cover as it arrived from a source: bytes, with the hash they are
// known by.
type Image struct {
	SHA256 string
	MIME   string
	Bytes  []byte
}

// NewImage hashes and sniffs a cover. It returns nil for bytes that are not
// an image: a driver that publishes something else on its art topic has not
// supplied a cover.
func NewImage(data []byte) *Image {
	if len(data) == 0 {
		return nil
	}
	mime := http.DetectContentType(data)
	if len(mime) < 6 || mime[:6] != "image/" {
		return nil
	}
	sum := sha256.Sum256(data)
	return &Image{SHA256: hex.EncodeToString(sum[:]), MIME: mime, Bytes: data}
}

// Publisher turns a winning state into what is published.
type Publisher struct {
	Bus      Bus
	Resolver Resolver // nil: never look anything up
	Prefix   string
	Timeout  time.Duration // for one catalogue lookup
	Logger   *slog.Logger
	Now      func() time.Time

	// What was last put on the bus, so nothing is published twice. Retained
	// messages mean a subscriber that reconnects gets the same cover again
	// without help; republishing it would only make lightd repaint a room
	// that is already the right colour.
	lastState *contract.State
	lastArt   string
}

// Publish puts the current track on the bus. winner is the arbiter's choice,
// or nil when no source has anything to say. fromSource is the cover the
// winning source itself published, if it has.
//
// The cover goes out before the state that names it. A subscriber that reads
// the state and then looks for the image must find the one the state means,
// and the other order would leave a moment when it finds the last one.
//
// It returns the state as published - with the album and the cover it was
// completed with - and whether anything went out at all.
func (p *Publisher) Publish(ctx context.Context, winner *contract.State, fromSource *Image) (contract.State, bool, error) {
	out, cover := p.complete(ctx, winner, fromSource)
	sent := false

	if cover != nil && cover.SHA256 != p.lastArt {
		if err := p.Bus.Publish(contract.CurrentArtTopic(p.Prefix), cover.Bytes, true); err != nil {
			return out, false, err
		}
		p.lastArt = cover.SHA256
		sent = true
	}

	if contract.Same(&out, p.lastState) {
		return out, sent, nil
	}
	payload, err := contract.Encode(out)
	if err != nil {
		return out, sent, err
	}
	if err := p.Bus.Publish(contract.CurrentTopic(p.Prefix), payload, true); err != nil {
		return out, sent, err
	}
	p.lastState = &out
	return out, true, nil
}

// complete fills in what the winning source lacked, and returns the state to
// publish with the cover it names, if it names one.
func (p *Publisher) complete(ctx context.Context, winner *contract.State, fromSource *Image) (contract.State, *Image) {
	if winner == nil {
		// Nothing is reporting at all. Say so, rather than leave the last
		// track standing on a retained topic as if it were still playing.
		return contract.State{V: contract.Version, State: contract.Stopped, ChangedAt: p.now()}, nil
	}

	out := *winner
	out.V = contract.Version
	out.Art = nil
	if winner.Track == nil {
		return out, nil
	}
	track := *winner.Track
	out.Track = &track

	// The player's own cover is the right cover, and nothing is looked up
	// when it is there. It counts only if it is the one this state names: a
	// track change and its cover are two messages, and for a moment the
	// image on hand can be the previous track's.
	var cover *Image
	if fromSource != nil && winner.Art != nil && fromSource.SHA256 == winner.Art.SHA256 {
		cover = fromSource
		out.Art = &contract.Art{SHA256: fromSource.SHA256, MIME: fromSource.MIME, From: "player"}
		if track.Album != "" {
			return out, cover
		}
	}

	if p.Resolver == nil {
		return out, cover
	}
	lookup, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	found, err := p.Resolver.Resolve(lookup, track)
	if err != nil {
		if !errors.Is(err, resolve.ErrNotFound) {
			p.logger().Warn("lookup failed", "artist", track.Artist, "album", track.Album, "title", track.Title, "error", err)
		}
		// Published incomplete rather than withheld: a track with no cover
		// is still the track that is playing.
		return out, cover
	}

	if track.Album == "" && found.Album != "" {
		out.Track.Album = found.Album
	}
	if out.Art == nil && found.Cover != nil {
		if image := NewImage(found.Cover.Image); image != nil {
			cover = image
			out.Art = &contract.Art{SHA256: image.SHA256, MIME: image.MIME, From: found.Cover.From}
		}
	}
	return out, cover
}

func (p *Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Publisher) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 10 * time.Second
}

func (p *Publisher) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}
