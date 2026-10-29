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
	"errors"
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
	// catalog nowplayd found it in. A cover from the player is the right
	// cover; one from a catalog is a good guess.
	From string `json:"from,omitempty"`
}

// State is one message: what one source is doing, or - on the current topic -
// what is playing as far as anyone listening should care.
type State struct {
	V int `json:"v"`

	// Source is the driver deployment that saw this, e.g. "itunes-den". It is
	// also a level in the topic the state is published on.
	Source string `json:"source,omitempty"`
	// Player is the kind of player: itunes, music, spotify, tidal, musicbee,
	// turntable.
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

	// NoControls is said on the current topic only, and only by nowplayd:
	// that a controller should offer nothing for this source, because there
	// is nothing a command to it would do (ADR-0025). Left out, controls
	// are offered, which is what every reader before this field does.
	NoControls bool `json:"no_controls,omitempty"`
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
	if a.Source != b.Source || a.Player != b.Player || a.State != b.State || a.NoControls != b.NoControls {
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
//	<prefix>/nowplaying/current/art/small    the same cover, small, retained
//	<prefix>/nowplaying/status               nowplayd's own presence
//	<prefix>/nowplaying/command              what a controller wants, never retained
//	<prefix>/nowplaying/source/<id>/command  the same, for one driver, never retained
//	<prefix>/nowplaying/source/<id>/switch   whether its driver runs, retained (ADR-0051)
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

// CurrentSmallArtTopic carries the cover on CurrentArtTopic again, made small
// enough for a subscriber that cannot take the original: a baseline JPEG that
// fits a small square (ADR-0020). An empty payload means the current cover
// has no small form, and the one a subscriber holds is not it.
func CurrentSmallArtTopic(prefix string) string { return prefix + "/nowplaying/current/art/small" }

// CommandTopic is where a controller says what it wants of whatever is
// playing (ADR-0020). It names no source, because a controller does not know
// one: nowplayd passes the command to the source that is current.
func CommandTopic(prefix string) string { return prefix + "/nowplaying/command" }

// BeaconStatusTopic is where a beacon says it has power: "online" while it
// does, and the broker's "offline" for it when it has not (ADR-0023). It is
// outside nowplaying/, because a beacon is not a source and reports no
// playing: it is a device on a switch, and says only that the switch is on.
func BeaconStatusTopic(prefix, id string) string {
	return prefix + "/beacon/" + id + "/status"
}

// SourceCommandTopic is where one driver hears commands meant for its player.
func SourceCommandTopic(prefix, id string) string {
	return prefix + "/nowplaying/source/" + id + "/command"
}

// What a controller may ask for. The list is short on purpose: it is what a
// hand reaches for while a record plays, and each is something every player
// worth a driver can do.
const (
	PlayPause  = "play_pause"
	Next       = "next"
	Previous   = "previous"
	VolumeUp   = "volume_up"
	VolumeDown = "volume_down"
)

// Duck and Unduck lower what is playing while a request is heard, and bring
// it back (ADR-0044). They go to the current source, as the buttons do; the
// driver brings the music back by itself if Unduck never comes.
const (
	Duck   = "duck"
	Unduck = "unduck"
)

// PlayAlbum is not a button: it chooses what plays, by naming an album, and
// goes to the source that has a library rather than to the current one
// (ADR-0040).
const PlayAlbum = "play_album"

// maxName bounds an artist's or an album's name in a command. The longest
// real ones are a few dozen characters; this is far past them, and short of
// anything that could only be a mistake or a trick.
const maxName = 200

// Command is one request to change what a player is doing (ADR-0020).
//
// A command is a moment, not a state, and that sets it apart from everything
// else on this bus: it is never retained, and a receiver handed a retained one
// drops it. A "next" kept by the broker would skip a track each time its
// reader reconnected.
type Command struct {
	V       int    `json:"v"`
	Command string `json:"command"`
	// From names the controller, for whoever reads the logs. Nothing is
	// decided by it: the broker's grant is what says who may command.
	From string `json:"from,omitempty"`

	// Artist and Album name the album play_album asks for, as the player's
	// library names it. Nothing else carries them.
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	// Instead are the artist's albums to try, in order, if the library has
	// not got Album (ADR-0045). play_album only, and at most MaxInstead.
	Instead []string `json:"instead,omitempty"`
}

// MaxInstead is how many alternatives a play_album may carry: other names
// for the album, then the artist's next albums. Each the library lacks is a
// second and a half of trying.
const MaxInstead = 6

// Validate reports the first thing wrong with a command.
func (c Command) Validate() error {
	if c.V != Version {
		return fmt.Errorf("contract: version %d, this program speaks %d", c.V, Version)
	}
	switch c.Command {
	case PlayPause, Next, Previous, VolumeUp, VolumeDown, Duck, Unduck:
		return nil
	case PlayAlbum:
		for _, f := range [...]struct{ field, name string }{{"artist", c.Artist}, {"album", c.Album}} {
			if strings.TrimSpace(f.name) == "" {
				return fmt.Errorf("contract: play_album names no %s", f.field)
			}
			if len(f.name) > maxName {
				return fmt.Errorf("contract: play_album's %s is longer than %d bytes", f.field, maxName)
			}
		}
		if len(c.Instead) > MaxInstead {
			return fmt.Errorf("contract: play_album names %d albums instead, at most %d", len(c.Instead), MaxInstead)
		}
		for _, a := range c.Instead {
			if strings.TrimSpace(a) == "" || len(a) > maxName {
				return fmt.Errorf("contract: play_album names an album instead that is empty or longer than %d bytes", maxName)
			}
		}
		return nil
	}
	return fmt.Errorf("contract: unknown command %q", c.Command)
}

// EncodeCommand stamps the version and marshals. A command that would not
// pass Validate is not sent.
func EncodeCommand(c Command) ([]byte, error) {
	c.V = Version
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

// DecodeCommand parses and validates a command from the bus. An unknown
// command is refused, not guessed at: the nearest known one is still the
// wrong thing to do to someone's music.
func DecodeCommand(payload []byte) (Command, error) {
	var c Command
	if err := json.Unmarshal(payload, &c); err != nil {
		return Command{}, fmt.Errorf("contract: not a command: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Command{}, err
	}
	return c, nil
}

// CurrentLookTopic carries the look of the cover on CurrentArtTopic, when its
// album has one of its own (ADR-0019).
func CurrentLookTopic(prefix string) string { return prefix + "/nowplaying/current/look" }

// Look says whether a cover has a look of its own, and names it.
//
// It belongs to one cover, named by hash: a subscriber applies it to that
// cover and to no other, so it does not matter which of the two messages
// arrives first, and a look cannot land on the album after its own. A cover
// with no look gets a message with none, which is how the look before it is
// let go.
type Look struct {
	V int `json:"v"`
	// Look is a name, or empty. What a name looks like is the subscriber's.
	Look string `json:"look,omitempty"`
	// Art is the SHA-256 of the cover this is for. Empty, the look is for
	// there being no cover (ADR-0028).
	Art string `json:"art"`
}

// LookStandby is the look of no cover at all, in a room where something is
// switched on and nothing is playing yet (ADR-0028). Like any look it is a
// name: what it looks like is the subscriber's.
const LookStandby = "standby"

// LookDark is the look of a cover the room is not to be lit for: it is
// shown on a screen, and that is all (ADR-0028).
const LookDark = "dark"

// EncodeNoCoverLook is the look of there being no cover: a name, or none,
// and no cover named. It goes out before the cover topics are emptied, as a
// cover's look goes out before its cover.
func EncodeNoCoverLook(look string) ([]byte, error) {
	return json.Marshal(Look{V: Version, Look: look})
}

// EncodeLook is a Look as it goes on the bus.
func EncodeLook(look, art string) ([]byte, error) {
	if art == "" {
		return nil, errors.New("contract: a look belongs to a cover, and none was named")
	}
	return json.Marshal(Look{V: Version, Look: look, Art: art})
}

// StandTopic holds which sources the stand is lit for (ADR-0039). It is a
// setting, retained, and not a command: whoever wrote it last decides, and it
// outlives a restart of everyone who reads it.
func StandTopic(prefix string) string { return prefix + "/nowplaying/stand" }

// What the stand may be lit for. Records is ADR-0028, and the default; All
// lights it for every source, the dark ones included.
const (
	StandRecords = "records"
	StandAll     = "all"
)

// Stand is the setting on StandTopic.
type Stand struct {
	V       int    `json:"v"`
	Sources string `json:"sources"`
}

// EncodeStand is a setting as it goes on the bus.
func EncodeStand(sources string) ([]byte, error) {
	if sources != StandRecords && sources != StandAll {
		return nil, fmt.Errorf("contract: the stand cannot be lit for %q", sources)
	}
	return json.Marshal(Stand{V: Version, Sources: sources})
}

// DecodeStand reads the setting. An empty payload is no setting, and is
// StandRecords with no error: nothing on the broker is the default. Anything
// it cannot read is StandRecords too, and the error says why, so that a
// setting gone wrong lights the room as it was meant to be by default.
func DecodeStand(payload []byte) (string, error) {
	if len(payload) == 0 {
		return StandRecords, nil
	}
	var st Stand
	if err := json.Unmarshal(payload, &st); err != nil {
		return StandRecords, fmt.Errorf("contract: not a stand setting: %w", err)
	}
	if st.V != Version {
		return StandRecords, fmt.Errorf("contract: version %d, this program speaks %d", st.V, Version)
	}
	if st.Sources != StandRecords && st.Sources != StandAll {
		return StandRecords, fmt.Errorf("contract: the stand cannot be lit for %q", st.Sources)
	}
	return st.Sources, nil
}

// SwitchTopic holds whether a source's driver runs (ADR-0051). It is a
// setting, retained, like the stand's: the agent obeys it, and the console
// reads it back. Only Spotify has one.
func SwitchTopic(prefix, id string) string {
	return prefix + "/nowplaying/source/" + id + "/switch"
}

// Whether a switched driver runs. Off is the default: a driver with a switch
// costs something to run, and nothing on the broker asks nothing of anyone.
const (
	SwitchOn  = "on"
	SwitchOff = "off"
)

// Switch is the setting on SwitchTopic.
type Switch struct {
	V      int    `json:"v"`
	Driver string `json:"driver"`
}

// EncodeSwitch is a setting as it goes on the bus.
func EncodeSwitch(driver string) ([]byte, error) {
	if driver != SwitchOn && driver != SwitchOff {
		return nil, fmt.Errorf("contract: a driver is switched %q or %q, not %q", SwitchOn, SwitchOff, driver)
	}
	return json.Marshal(Switch{V: Version, Driver: driver})
}

// DecodeSwitch reads the setting. An empty payload is no setting, and is
// SwitchOff with no error. Anything it cannot read is SwitchOff too, and the
// error says why: a setting gone wrong stops the asking rather than starting
// it.
func DecodeSwitch(payload []byte) (string, error) {
	if len(payload) == 0 {
		return SwitchOff, nil
	}
	var sw Switch
	if err := json.Unmarshal(payload, &sw); err != nil {
		return SwitchOff, fmt.Errorf("contract: not a driver's switch: %w", err)
	}
	if sw.V != Version {
		return SwitchOff, fmt.Errorf("contract: version %d, this program speaks %d", sw.V, Version)
	}
	if sw.Driver != SwitchOn && sw.Driver != SwitchOff {
		return SwitchOff, fmt.Errorf("contract: a driver is switched %q or %q, not %q", SwitchOn, SwitchOff, sw.Driver)
	}
	return sw.Driver, nil
}

// ParseSourceTopic splits a source topic into the source's id and which of
// the three topics a driver reports on this is: "state", "status" or "art".
// A source's command topic is not one of them and is not ok: it runs the
// other way, and whoever reads the sources has nothing to learn from it.
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
