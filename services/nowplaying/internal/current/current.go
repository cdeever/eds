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

// Collection knows the records their owner has (ADR-0022). Find names the
// record a track is on, and returns resolve.ErrNotFound when it is on none
// of them.
type Collection interface {
	Find(ctx context.Context, artist, title, album string) (resolve.Result, error)
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

	// Coverless names sources whose tracks are published without a cover:
	// none taken from the source, and none looked up. The track is still the
	// current track, and says so; it simply has no picture (ADR-0016).
	//
	// It is how a source is reported without lighting the stand. lightd
	// reacts to a cover arriving and to nothing else, so a current track with
	// no cover leaves the room as it was. The lookup is refused as well as
	// the source's own image, because a catalogue's cover would light the
	// room just the same.
	Coverless map[string]bool
	// Owned names sources that play the owner's own records, and Collection
	// is where those records are listed (ADR-0022). For such a source the
	// collection is asked first, by artist and title, and its answer is the
	// album: it replaces whatever album the source named, because a source
	// that hears a track cannot know which record it is on, and the
	// collection can. A track the collection does not have is completed the
	// ordinary way.
	Owned      map[string]bool
	Collection Collection
	// Looks names the look of the album a track is on, or "" (ADR-0019). Nil
	// means no album has one. It is published beside the cover, for the
	// cover: a look is something a cover has.
	Looks func(contract.Track) string
	// Small makes a cover small, for the subscriber that cannot take it as
	// it comes (ADR-0020). Nil means no small cover is published. It is
	// published after the cover and before the state, and when a cover
	// cannot be made small the topic is emptied instead: a subscriber left
	// holding the last album's small cover would be showing the wrong sleeve
	// with nothing to tell it so.
	Small func([]byte) ([]byte, error)
	// NoControls names sources a controller should offer nothing for,
	// because a command to them does nothing (ADR-0025). It is said in the
	// current state, for whoever draws buttons.
	NoControls map[string]bool
	Prefix     string
	Timeout    time.Duration // for one catalogue lookup
	Logger     *slog.Logger
	Now        func() time.Time

	// What was last put on the bus, so nothing is published twice. Retained
	// messages mean a subscriber that reconnects gets the same cover again
	// without help; republishing it would only make lightd repaint a room
	// that is already the right colour.
	lastState *contract.State
	lastArt   string
	// The cover whose small form was last published, or emptied.
	lastSmall string
	// The look last published, and the cover it was for.
	lastLook, lastLookArt string
	lookSent              bool
	// Whether the cover topics were last emptied, for idle (ADR-0024).
	emptied bool
}

// Publish puts the current track on the bus. winner is the arbiter's choice,
// or nil when no source has anything to say. fromSource is the cover the
// winning source itself published, if it has.
//
// The cover goes out before the state that names it, and its small form with
// it. A subscriber that reads the state and then looks for the image must
// find the one the state means, and the other order would leave a moment when
// it finds the last one.
//
// It returns the state as published - with the album and the cover it was
// completed with - and whether anything went out at all.
func (p *Publisher) Publish(ctx context.Context, winner *contract.State, fromSource *Image) (contract.State, bool, error) {
	out, cover := p.complete(ctx, winner, fromSource)
	sent := false

	// The look before the cover it belongs to. Whoever lights a room from the
	// cover then already knows whether this one has a look of its own, and
	// does not light it twice. The order is a courtesy, not a guarantee: the
	// message names its cover, so a subscriber that gets them the other way
	// round still pairs them.
	if cover != nil {
		look := ""
		if p.Looks != nil && out.Track != nil {
			look = p.Looks(*out.Track)
		}
		if !p.lookSent || look != p.lastLook || cover.SHA256 != p.lastLookArt {
			payload, err := contract.EncodeLook(look, cover.SHA256)
			if err != nil {
				return out, false, err
			}
			if err := p.Bus.Publish(contract.CurrentLookTopic(p.Prefix), payload, true); err != nil {
				return out, false, err
			}
			p.lastLook, p.lastLookArt, p.lookSent = look, cover.SHA256, true
			sent = true
		}
	}
	if cover != nil && cover.SHA256 != p.lastArt {
		if err := p.Bus.Publish(contract.CurrentArtTopic(p.Prefix), cover.Bytes, true); err != nil {
			return out, false, err
		}
		p.lastArt = cover.SHA256
		p.emptied = false
		sent = true
	}
	// Kept apart from the cover above so that a small cover the bus did not
	// take is sent again without the cover itself going out a second time.
	if cover != nil && p.Small != nil && cover.SHA256 != p.lastSmall {
		small, err := p.Small(cover.Bytes)
		if err != nil {
			p.logger().Warn("the cover has no small form; the small cover is emptied", "art", cover.SHA256, "error", err)
			small = nil
		}
		if err := p.Bus.Publish(contract.CurrentSmallArtTopic(p.Prefix), small, true); err != nil {
			return out, false, err
		}
		p.lastSmall = cover.SHA256
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

// PublishIdle says that nothing is playing as far as a room is concerned:
// the winner has sat paused, or there has been none, for long enough
// (ADR-0024).
//
// The state is "stopped" with no track. It still names the winner's source,
// because that is who a command is for, and a pause is undone from where it
// was made. The cover topics are emptied, which is what whoever lights a
// room from the cover is waiting to be told: an emptied cover is no cover,
// where a cover merely not sent leaves the room as it was (ADR-0016).
func (p *Publisher) PublishIdle(ctx context.Context, winner *contract.State) (contract.State, bool, error) {
	out := contract.State{V: contract.Version, State: contract.Stopped, ChangedAt: p.now()}
	if winner != nil {
		out.Source, out.Player = winner.Source, winner.Player
		out.NoControls = p.NoControls[winner.Source]
	}
	sent := false

	// The state first, here: a subscriber told "stopped" and then shown no
	// cover has nothing to get wrong, and one shown no cover while the track
	// still reads as paused would look for a cover that is not coming.
	if !contract.Same(&out, p.lastState) {
		payload, err := contract.Encode(out)
		if err != nil {
			return out, false, err
		}
		if err := p.Bus.Publish(contract.CurrentTopic(p.Prefix), payload, true); err != nil {
			return out, false, err
		}
		p.lastState = &out
		sent = true
	}

	if !p.emptied {
		topics := []string{contract.CurrentArtTopic(p.Prefix)}
		if p.Small != nil {
			topics = append(topics, contract.CurrentSmallArtTopic(p.Prefix))
		}
		for _, topic := range topics {
			if err := p.Bus.Publish(topic, nil, true); err != nil {
				return out, sent, err
			}
		}
		// Whatever comes next is published in full, the same cover included:
		// the topics no longer hold it, and nobody lit from it still is.
		p.emptied, p.lastArt, p.lastSmall, p.lookSent = true, "", "", false
		sent = true
	}
	return out, sent, nil
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
	out.NoControls = p.NoControls[winner.Source]
	if winner.Track == nil {
		return out, nil
	}
	track := *winner.Track
	out.Track = &track

	if p.Coverless[winner.Source] {
		return out, nil
	}

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

	lookup, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	if p.Collection != nil && p.Owned[winner.Source] && cover == nil {
		owned, err := p.Collection.Find(lookup, track.Artist, track.Title, track.Album)
		switch {
		case err == nil:
			// The record on the platter. Its title is the album, whatever
			// the source called it, and what is looked up below - when the
			// collection had no picture of it - is looked up by that title.
			out.Track.Album = owned.Album
			track.Album = owned.Album
			if owned.Cover != nil {
				if image := NewImage(owned.Cover.Image); image != nil {
					out.Art = &contract.Art{SHA256: image.SHA256, MIME: image.MIME, From: owned.Cover.From}
					return out, image
				}
			}
		case !errors.Is(err, resolve.ErrNotFound):
			p.logger().Warn("the collection could not be asked", "artist", track.Artist, "title", track.Title, "error", err)
		}
	}

	if p.Resolver == nil {
		return out, cover
	}

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
