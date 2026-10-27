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
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// Bus is the part of the bus this package uses.
type Bus interface {
	Publish(topic string, payload []byte, retained bool) error
}

// Resolver finds a track's album and cover in a catalog.
type Resolver interface {
	Resolve(ctx context.Context, t contract.Track) (resolve.Result, error)
}

// Collection knows the records their owner has (ADR-0022). Find names the
// record a track is on, and returns resolve.ErrNotFound when it is on none
// of them.
type Collection interface {
	Find(ctx context.Context, artist, title, album string) (resolve.Result, error)
}

// Albums names the record a track is on by what is on it (ADR-0038), for the
// tracks the collection does not have. Has says whether an album's track list
// has a track; Original names the original studio album a track is on, hint
// being the album the source named. Both return resolve.ErrNotFound when
// there is no answer to be had.
type Albums interface {
	Has(ctx context.Context, artist, album, title string) (bool, error)
	Original(ctx context.Context, artist, title, hint string) (string, error)
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
	// the source's own image, because a catalog's cover would light the
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
	// Albums names the record for an owned source's track when the
	// collection does not have it (ADR-0038): the record already playing if
	// it has the track, and otherwise the original album the track is on.
	// Nil leaves the album the source named.
	Albums Albums
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
	// Dark names sources no room is lit for (ADR-0028). Their covers are
	// published like any other, for whoever shows a cover, and each is given
	// the look that says not to light anything from it.
	Dark map[string]bool
	// AnySource says a look is shown whatever source its track is from, a
	// dark one included (ADR-0030). Nil is none.
	AnySource func(look string) bool
	// AllSources lights the stand for every source, as if Dark named none
	// (ADR-0039). It is the setting the console switches; Stand changes it
	// while running.
	AllSources bool
	Prefix     string
	Timeout    time.Duration // for naming a record, and again for its cover's lookup
	Logger     *slog.Logger
	Now        func() time.Time
	// Report is told how each track was completed (ADR-0050): once for a
	// track, and again only if it comes out differently. It is called while
	// publishing, and must not keep the caller waiting. Nil reports nothing.
	Report func(Lookup)

	// What was last put on the bus, so nothing is published twice. Retained
	// messages mean a subscriber that reconnects gets the same cover again
	// without help; republishing it would only make lightd repaint a room
	// that is already the right color.
	lastState *contract.State
	lastArt   string
	// The record each owned source is playing: its artist and album, as last
	// named. A track on it keeps it; one that is not moves on (ADR-0038).
	playingRecord map[string][2]string
	// The cover whose small form was last published, or emptied.
	lastSmall string
	// The look last published, and the cover it was for.
	lastLook, lastLookArt string
	lookSent              bool
	// Whether the cover topics were last emptied, for idle (ADR-0024).
	emptied bool
	// The track and outcome last reported, so a track is reported once.
	lastReported string
}

// Lookup is how one track was completed: who named its album, where its
// cover came from, and what each took (ADR-0050).
type Lookup struct {
	Source, Artist, Title string
	// Heard is the album the source named; Album is the one published.
	Heard, Album string
	// NamedBy is who named the album: "collection", the owner's record;
	// "record", the one already playing, which has the track; "original",
	// the original album the track is on; "catalog", a catalog, for a track
	// that came with none; "source", left as the source named it.
	NamedBy string
	// CoverFrom is where the cover came from: "player", or the name of the
	// collection or catalog that had it, or "none".
	CoverFrom string
	// The time spent naming the record, and looking up the cover. Zero is a
	// step not taken.
	NameMS, CoverMS int64
	// Why a step gave up, when it did: "timeout" or "failed". Not finding
	// anything is not giving up.
	NameError, CoverError string
}

// Attrs is the lookup as the fields of a log line.
func (l Lookup) Attrs() []any {
	return []any{
		"source", l.Source, "artist", l.Artist, "title", l.Title,
		"heard_album", l.Heard, "album", l.Album, "named_by", l.NamedBy,
		"cover_from", l.CoverFrom, "name_ms", l.NameMS, "cover_ms", l.CoverMS,
		"name_error", l.NameError, "cover_error", l.CoverError,
	}
}

// LookupEvent is the name the lookups are reported under.
const LookupEvent = "cover.lookup"

func giveUp(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "failed"
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
	var lookup Lookup
	out, cover := p.complete(ctx, winner, fromSource, &lookup)
	p.report(lookup, out)
	sent := false

	// The look before the cover it belongs to. Whoever lights a room from the
	// cover then already knows whether this one has a look of its own, and
	// does not light it twice. The order is a courtesy, not a guarantee: the
	// message names its cover, so a subscriber that gets them the other way
	// around still pairs them.
	if cover != nil {
		look := p.lookFor(out)
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

// lookFor is the look of the cover a state names.
func (p *Publisher) lookFor(out contract.State) string {
	look := ""
	if p.Looks != nil && out.Track != nil {
		look = p.Looks(*out.Track)
	}
	// Before any look the album has: a room that is not lit for this source
	// is not lit for its best-known record either. The few looks that are for
	// any source are the exception, and the only one. Unless the stand has
	// been switched to every source (ADR-0039).
	if !p.AllSources && p.Dark[out.Source] && !(look != "" && p.AnySource != nil && p.AnySource(look)) {
		look = contract.LookDark
	}
	return look
}

// Stand changes which sources the stand is lit for (ADR-0039), and says the
// look of the cover that is up again if that changes it: the setting applies
// to the track playing, and not from the next one. It returns whether
// anything was published.
func (p *Publisher) Stand(sources string) (bool, error) {
	p.AllSources = sources == contract.StandAll
	if !p.lookSent || p.lastLookArt == "" || p.lastState == nil || p.emptied {
		return false, nil
	}
	look := p.lookFor(*p.lastState)
	if look == p.lastLook {
		return false, nil
	}
	payload, err := contract.EncodeLook(look, p.lastLookArt)
	if err != nil {
		return false, err
	}
	if err := p.Bus.Publish(contract.CurrentLookTopic(p.Prefix), payload, true); err != nil {
		return false, err
	}
	p.lastLook = look
	return true, nil
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
//
// on says some source's equipment is switched on. Then the room is waiting
// for a record, and the look of no cover is named so; otherwise it has no
// look, and what a room with nothing in it looks like is whoever lights
// it's to say (ADR-0028). The look is said each time it changes, the cover
// topics emptied or not: a turntable can be switched on in an idle room.
func (p *Publisher) PublishIdle(ctx context.Context, winner *contract.State, on bool) (contract.State, bool, error) {
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

	// The look before the cover is taken away, as a cover's look goes
	// before its cover: whoever lights the room is not left to guess, and
	// does not light it twice.
	look := ""
	if on {
		look = contract.LookStandby
	}
	if !p.lookSent || look != p.lastLook || p.lastLookArt != "" {
		payload, err := contract.EncodeNoCoverLook(look)
		if err != nil {
			return out, sent, err
		}
		if err := p.Bus.Publish(contract.CurrentLookTopic(p.Prefix), payload, true); err != nil {
			return out, sent, err
		}
		p.lastLook, p.lastLookArt, p.lookSent = look, "", true
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
		// Its look goes again too, the look topic now holding no cover's.
		p.emptied, p.lastArt, p.lastSmall = true, "", ""
		sent = true
	}
	return out, sent, nil
}

// complete fills in what the winning source lacked, and returns the state to
// publish with the cover it names, if it names one.
//
// What it did is written into lk, which is left empty when there was no
// track to complete.
func (p *Publisher) complete(ctx context.Context, winner *contract.State, fromSource *Image, lk *Lookup) (contract.State, *Image) {
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
	*lk = Lookup{Source: winner.Source, Artist: track.Artist, Title: track.Title, Heard: track.Album, NamedBy: "source"}

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

	if p.Owned[winner.Source] && cover == nil {
		// Naming the record has its own time, and the cover's lookup below
		// its own: MusicBrainz, one call a second, can spend all of one, and
		// a track it did not name still has a cover to be found.
		lookup, cancel := context.WithTimeout(ctx, p.timeout())
		defer cancel()
		started := time.Now()
		named := false
		if p.Collection != nil {
			owned, err := p.Collection.Find(lookup, track.Artist, track.Title, track.Album)
			switch {
			case err == nil:
				// The record on the platter. Its title is the album, whatever
				// the source called it, and what is looked up below - when the
				// collection had no picture of it - is looked up by that title.
				out.Track.Album = owned.Album
				track.Album = owned.Album
				named = true
				lk.NamedBy = "collection"
				p.playing(winner.Source, track)
				if owned.Cover != nil {
					if image := NewImage(owned.Cover.Image); image != nil {
						out.Art = &contract.Art{SHA256: image.SHA256, MIME: image.MIME, From: owned.Cover.From}
						lk.NameMS = time.Since(started).Milliseconds()
						return out, image
					}
				}
			case !errors.Is(err, resolve.ErrNotFound):
				lk.NameError = giveUp(err)
				p.logger().Warn("the collection could not be asked", "artist", track.Artist, "title", track.Title, "error", err)
			}
		}
		if !named && p.Albums != nil {
			album, by, err := p.recordFor(lookup, winner.Source, track)
			if album != "" {
				out.Track.Album = album
				track.Album = album
				lk.NamedBy = by
			}
			if err != nil {
				lk.NameError = giveUp(err)
			}
			p.playing(winner.Source, track)
		}
		lk.NameMS = time.Since(started).Milliseconds()
	}

	if p.Resolver == nil {
		return out, cover
	}

	lookup, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	started := time.Now()
	found, err := p.Resolver.Resolve(lookup, track)
	lk.CoverMS = time.Since(started).Milliseconds()
	if err != nil {
		if !errors.Is(err, resolve.ErrNotFound) {
			lk.CoverError = giveUp(err)
			p.logger().Warn("lookup failed", "artist", track.Artist, "album", track.Album, "title", track.Title, "error", err)
		}
		// Published incomplete rather than withheld: a track with no cover
		// is still the track that is playing.
		return out, cover
	}

	if track.Album == "" && found.Album != "" {
		out.Track.Album = found.Album
		lk.NamedBy = "catalog"
	}
	if out.Art == nil && found.Cover != nil {
		if image := NewImage(found.Cover.Image); image != nil {
			cover = image
			out.Art = &contract.Art{SHA256: image.SHA256, MIME: image.MIME, From: found.Cover.From}
		}
	}
	return out, cover
}

// recordFor names the album for a track the collection does not have
// (ADR-0038). The record already playing keeps it when its track list has the
// track: a side turned over, or a track the source filed under a box set. A
// track that is not on it is another record, however soon it came, and is
// named by the original album it is on. "" leaves the source's album.
//
// by says which named it: "record" or "original". err is the last failure
// to ask, already logged; it is nil when the answer was that nothing is known.
func (p *Publisher) recordFor(ctx context.Context, source string, track contract.Track) (album, by string, failed error) {
	if held, ok := p.playingRecord[source]; ok && held[1] != "" && resolve.Simplify(held[0]) == resolve.Simplify(track.Artist) {
		has, err := p.Albums.Has(ctx, track.Artist, held[1], track.Title)
		switch {
		case err == nil && has:
			return held[1], "record", nil
		case err != nil && !errors.Is(err, resolve.ErrNotFound):
			failed = err
			p.logger().Warn("an album's tracks could not be asked", "artist", track.Artist, "album", held[1], "error", err)
		}
	}
	album, err := p.Albums.Original(ctx, track.Artist, track.Title, track.Album)
	switch {
	case err == nil:
		if resolve.Simplify(album) != resolve.Simplify(track.Album) {
			p.logger().Info("named the original album the track is on", "artist", track.Artist, "title", track.Title,
				"heard_as", track.Album, "album", album)
		}
		return album, "original", nil
	case !errors.Is(err, resolve.ErrNotFound):
		failed = err
		p.logger().Warn("the original album could not be asked", "artist", track.Artist, "title", track.Title, "error", err)
	}
	return "", "", failed
}

// report hands Report how a track was completed, unless that track was last
// reported coming out the same way: a pause and its resume are not news, a
// cover found the second time is.
func (p *Publisher) report(lk Lookup, out contract.State) {
	if p.Report == nil || lk.Source == "" {
		return
	}
	if out.Track != nil {
		lk.Album = out.Track.Album
	}
	lk.CoverFrom = "none"
	if out.Art != nil {
		lk.CoverFrom = out.Art.From
		if lk.CoverFrom == "" {
			lk.CoverFrom = "player"
		}
	}
	key := strings.Join([]string{lk.Source, lk.Artist, lk.Title, lk.Heard, lk.Album, lk.CoverFrom}, "\x00")
	if key == p.lastReported {
		return
	}
	p.lastReported = key
	p.Report(lk)
}

// playing remembers the record an owned source is playing.
func (p *Publisher) playing(source string, track contract.Track) {
	if strings.TrimSpace(track.Album) == "" {
		return
	}
	if p.playingRecord == nil {
		p.playingRecord = make(map[string][2]string)
	}
	p.playingRecord[source] = [2]string{track.Artist, track.Album}
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
