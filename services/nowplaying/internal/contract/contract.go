// Package contract is what goes on the now-playing bus.
//
// Two kinds of program meet here and neither knows the other: drivers, which
// each know one player, and whatever reacts to a track changing. This package
// is the whole of what they share (ADR-0006), so it is versioned, and a
// message with a different version is refused rather than guessed at - the
// same rule the scene descriptor follows between lightd and the firmware.
package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Version is stamped on every message as "v".
const Version = 1

// PlayState is what a player is doing.
type PlayState string

const (
	Playing PlayState = "playing"
	Paused  PlayState = "paused"
	Stopped PlayState = "stopped"
)

// Track is what is known about the music. Any field may be empty on a source
// topic: a driver reports what its player says and nothing more. On the
// current topic, Title and Album are always set while a track is playing,
// because nowplayd fills in what the source lacked (ADR-0008).
type Track struct {
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	AlbumArtist string `json:"album_artist,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
}

// Key identifies a track well enough to tell one from the next. Case and
// surrounding space are ignored, because the same track reported twice by one
// player does not always arrive spelled the same.
func (t Track) Key() string {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	return norm(t.Artist) + "\x00" + norm(t.Album) + "\x00" + norm(t.Title)
}

// AlbumKey identifies the album a track is on, which is what a cover belongs
// to. Album artist is preferred to artist so a compilation is one album.
func (t Track) AlbumKey() string {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	artist := t.AlbumArtist
	if strings.TrimSpace(artist) == "" {
		artist = t.Artist
	}
	return norm(artist) + "\x00" + norm(t.Album)
}

// Art describes a cover that travels beside the state as its own retained
// message of image bytes. Keeping the image out of the JSON is what keeps the
// state readable in mosquitto_sub; SHA256 is what ties the two together, so a
// subscriber can tell whether the image it holds belongs to this track.
type Art struct {
	SHA256 string `json:"sha256"`
	MIME   string `json:"mime,omitempty"`
	// From says where the image came from: the player itself, or the
	// catalogue nowplayd found it in. A cover from the player is the right
	// cover; one from a catalogue is a good guess.
	From string `json:"from,omitempty"`
}

// State is one message: what one source is doing, or - on the current topic -
// what is playing as far as anyone listening should care.
type State struct {
	V int `json:"v"`

	// Source is the driver deployment that saw this, e.g. "itunes-den". It is
	// also a level in the topic the state is published on.
	Source string `json:"source,omitempty"`
	// Player is the kind of player: itunes, music, spotify, tidal, musicbee.
	Player string `json:"player,omitempty"`

	State PlayState `json:"state"`
	Track *Track    `json:"track,omitempty"`

	// PositionMS is where in the track playback was when this was sent. It
	// is a hint for display and is not kept current between messages.
	PositionMS *int64 `json:"position_ms,omitempty"`

	// ChangedAt is when the track or the play state last changed, by the
	// sender's clock.
	ChangedAt time.Time `json:"changed_at"`

	Art *Art `json:"art,omitempty"`
}

var sourceID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidSourceID reports whether id can name a source. It becomes a topic
// level, so it is kept to what cannot be mistaken for a wildcard or a
// separator; the broker would refuse those too, but without saying so.
func ValidSourceID(id string) bool {
	return sourceID.MatchString(id)
}

// Validate reports the first thing wrong with a state that a sender is about
// to publish or a receiver has just decoded.
func (s State) Validate() error {
	switch {
	case s.V != Version:
		return fmt.Errorf("contract: version %d, this program speaks %d", s.V, Version)
	case s.State != Playing && s.State != Paused && s.State != Stopped:
		return fmt.Errorf("contract: unknown state %q", s.State)
	case s.Source != "" && !ValidSourceID(s.Source):
		return fmt.Errorf("contract: %q cannot name a source", s.Source)
	case s.Art != nil && s.Art.SHA256 == "":
		return fmt.Errorf("contract: art without a hash")
	}
	return nil
}

// Encode stamps the version and marshals. A state that would not pass
// Validate is not sent.
func Encode(s State) ([]byte, error) {
	s.V = Version
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// Decode parses and validates a state from the bus. A missing "v" is refused
// along with a wrong one: every sender is this module, and a message without a
// version is one whose shape nobody has promised.
func Decode(payload []byte) (State, error) {
	var s State
	if err := json.Unmarshal(payload, &s); err != nil {
		return State{}, fmt.Errorf("contract: not a state: %w", err)
	}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	return s, nil
}

// Same reports whether two states would look the same to a subscriber: same
// source, same play state, same track, same cover. Position and the time of
// the change are deliberately left out, so a driver that reports progress
// every few seconds does not make the current track look new each time.
func Same(a, b *State) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Source != b.Source || a.Player != b.Player || a.State != b.State {
		return false
	}
	if (a.Track == nil) != (b.Track == nil) {
		return false
	}
	if a.Track != nil && *a.Track != *b.Track {
		return false
	}
	if (a.Art == nil) != (b.Art == nil) {
		return false
	}
	return a.Art == nil || a.Art.SHA256 == b.Art.SHA256
}

// Presence payloads, for the status topics.
const (
	Online  = "online"
	Offline = "offline"
)

// Topics, relative to nothing: each takes the tenant's prefix. The layout is
// the contract as much as the JSON is.
//
//	<prefix>/nowplaying/source/<id>/state    a driver's view, retained
//	<prefix>/nowplaying/source/<id>/status   online | offline, retained, last will
//	<prefix>/nowplaying/source/<id>/art      image bytes, retained
//	<prefix>/nowplaying/current              the one current track, retained
//	<prefix>/nowplaying/current/art          its cover, retained
//	<prefix>/nowplaying/status               nowplayd's own presence
func SourceStateTopic(prefix, id string) string {
	return prefix + "/nowplaying/source/" + id + "/state"
}

func SourceStatusTopic(prefix, id string) string {
	return prefix + "/nowplaying/source/" + id + "/status"
}

func SourceArtTopic(prefix, id string) string {
	return prefix + "/nowplaying/source/" + id + "/art"
}

// AllSourcesTopic is the one subscription that covers every source.
func AllSourcesTopic(prefix string) string {
	return prefix + "/nowplaying/source/+/+"
}

func CurrentTopic(prefix string) string    { return prefix + "/nowplaying/current" }
func CurrentArtTopic(prefix string) string { return prefix + "/nowplaying/current/art" }
func StatusTopic(prefix string) string     { return prefix + "/nowplaying/status" }

// ParseSourceTopic splits a source topic into the source's id and which of
// its three topics this is: "state", "status" or "art".
func ParseSourceTopic(prefix, topic string) (id, kind string, ok bool) {
	rest, found := strings.CutPrefix(topic, prefix+"/nowplaying/source/")
	if !found {
		return "", "", false
	}
	id, kind, found = strings.Cut(rest, "/")
	if !found || !ValidSourceID(id) {
		return "", "", false
	}
	switch kind {
	case "state", "status", "art":
		return id, kind, true
	}
	return "", "", false
}
