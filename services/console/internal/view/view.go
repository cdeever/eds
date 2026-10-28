// Package view is what the console knows: the state of EdS as the bus last
// described it.
//
// It is fed messages and holds a picture. It knows the two contracts every
// subscriber knows - the now-playing state and the scene descriptor - and
// reads only the fields it shows, so a field added to either does not break
// it. It makes no network calls and decides nothing: what is current is
// nowplayd's decision, and what a cover becomes is lightd's. Which sources
// the stand is lit for is the setting on the bus, as last heard (ADR-0039),
// and so is whether Spotify's driver is switched on (ADR-0051).
package view

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ContractVersion is the version of both contracts this reads. A message
// stamped with another is not guessed at.
const ContractVersion = 1

// MaxCover bounds a cover held in memory. Covers on the bus are a few
// hundred kilobytes; this is the guard against something else arriving.
const MaxCover = 8 << 20

// keepRecent is how many plays are remembered. The durable history is the
// log store's; this is what scrolled past since the console started.
const keepRecent = 30

// Track is a track as the now-playing contract names it.
type Track struct {
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	AlbumArtist string `json:"album_artist,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
}

// Playing is one now-playing state: the current one, or a source's.
type Playing struct {
	Source     string     `json:"source,omitempty"`
	Player     string     `json:"player,omitempty"`
	State      string     `json:"state"`
	Track      *Track     `json:"track,omitempty"`
	PositionMS *int64     `json:"position_ms,omitempty"`
	ChangedAt  *time.Time `json:"changed_at,omitempty"`
	// Cover is the sha256 of the cover this state names, when it names one.
	Cover string `json:"cover,omitempty"`
	// CoverFrom is where that cover came from: the player, or a catalog.
	CoverFrom string `json:"cover_from,omitempty"`
}

// Lapse is how long an online is believed without being said again
// (ADR-0034). Everything says it every thirty seconds.
const Lapse = 90 * time.Second

// The words Presence.State takes.
const (
	PresenceOnline  = "online"  // heard live, lately
	PresenceClaimed = "claimed" // a stored online, not yet heard live
	PresenceOffline = "offline" // said offline, stored or live
	PresenceUnheard = "unheard" // an online not said again for Lapse
)

// Presence is whether a part is there, as far as the console has heard. A
// broker keeps a retained online across its own restarts, after the part
// that said it is gone, so a stored one is only a claim (ADR-0034).
type Presence struct {
	// Online is true only for a part heard live lately, false for one that
	// said offline, and nil otherwise.
	Online *bool  `json:"online,omitempty"`
	State  string `json:"presence,omitempty"` // empty until something is heard
	heard  time.Time
}

// hear takes a presence message and reports whether anything changed.
func (p *Presence) hear(payload []byte, retained bool, now time.Time) bool {
	word := presence(payload)
	if word == "" {
		return false
	}
	if word == PresenceOnline && retained {
		// A stored copy of what was just heard live, as a reconnect brings
		// back, confirms nothing and takes nothing away.
		if p.State == PresenceOnline {
			return false
		}
		word = PresenceClaimed
	}
	if word != PresenceOffline && !(word == PresenceClaimed && p.State == PresenceClaimed) {
		p.heard = now
	}
	return p.set(word)
}

// lapse turns an online or a claim not said again for Lapse into unheard.
func (p *Presence) lapse(now time.Time) bool {
	if (p.State == PresenceOnline || p.State == PresenceClaimed) && now.Sub(p.heard) >= Lapse {
		return p.set(PresenceUnheard)
	}
	return false
}

func (p *Presence) set(state string) bool {
	if p.State == state {
		return false
	}
	p.State, p.Online = state, nil
	switch state {
	case PresenceOnline:
		on := true
		p.Online = &on
	case PresenceOffline:
		off := false
		p.Online = &off
	}
	return true
}

// Source is one driver: whether it can reach its player, and what it sees.
type Source struct {
	ID string `json:"id"`
	Presence
	Now *Playing `json:"now,omitempty"`
}

// Swatch is one color of a scene and its share of the strip.
type Swatch struct {
	RGB    [3]uint8 `json:"rgb"`
	Weight float64  `json:"weight"`
}

// Scene is a scene descriptor, as lightd sent it.
type Scene struct {
	Effect     string   `json:"effect"`
	Speed      float64  `json:"speed"`
	Brightness float64  `json:"brightness"`
	Palette    []Swatch `json:"palette"`
}

// Applied is what a stand says it took.
type Applied struct {
	Effect     string  `json:"effect"`
	Colors     int     `json:"colors"`
	Brightness float64 `json:"brightness"`
}

// Stand is one light stand.
type Stand struct {
	ID string `json:"id"`
	Presence
	// Scene is what the stand was sent. Applied is what it reported taking.
	// A stand that is offline was sent a scene it has not seen.
	Scene   *Scene   `json:"scene,omitempty"`
	Applied *Applied `json:"applied,omitempty"`
}

// Beacon is a device on a switch, which says only that the switch is on
// (ADR-0023). Offline is its ordinary state, not a fault: it is what the
// thing it shares a switch with being turned off looks like.
type Beacon struct {
	ID string `json:"id"`
	Presence
}

// Screen is a cover screen (ADR-0020): a device that shows the cover and can
// ask for the next track. The page shows only whether it is there.
type Screen struct {
	ID string `json:"id"`
	Presence
}

// What the stand may be lit for (ADR-0039). Records is the default, and what
// nothing on the broker means.
const (
	StandRecords = "records"
	StandAll     = "all"
)

// StandTopic is the setting's topic.
func StandTopic(prefix string) string { return prefix + "/nowplaying/stand" }

// EncodeStand is the setting as it goes on the bus, or an error for a word
// that is not one of the two.
func EncodeStand(sources string) ([]byte, error) {
	if sources != StandRecords && sources != StandAll {
		return nil, fmt.Errorf("view: the stand cannot be lit for %q", sources)
	}
	return json.Marshal(struct {
		V       int    `json:"v"`
		Sources string `json:"sources"`
	}{ContractVersion, sources})
}

// Whether Spotify's driver runs (ADR-0051). Off is the default, and what
// nothing on the broker means.
const (
	DriverOn  = "on"
	DriverOff = "off"
)

// SpotifySwitchTopic is the setting's topic.
func SpotifySwitchTopic(prefix string) string { return prefix + "/nowplaying/source/spotify/switch" }

// EncodeSwitch is the setting as it goes on the bus, or an error for a word
// that is not one of the two.
func EncodeSwitch(driver string) ([]byte, error) {
	if driver != DriverOn && driver != DriverOff {
		return nil, fmt.Errorf("view: a driver is switched %q or %q, not %q", DriverOn, DriverOff, driver)
	}
	return json.Marshal(struct {
		V      int    `json:"v"`
		Driver string `json:"driver"`
	}{ContractVersion, driver})
}

// Play is one entry in what was recently played.
type Play struct {
	At     time.Time `json:"at"`
	Player string    `json:"player,omitempty"`
	Source string    `json:"source,omitempty"`
	Track  Track     `json:"track"`
}

// Snapshot is everything the page shows.
type Snapshot struct {
	// Current is what nowplayd says is playing. Nil until it has said.
	Current *Playing `json:"current,omitempty"`
	// LitCover is the sha256 of the cover on the cover topic: the one the
	// stand is lit from. It is not always the current track's - a source can
	// be reported without its cover, and then this is an earlier track's.
	LitCover string            `json:"lit_cover,omitempty"`
	Sources  []Source          `json:"sources"`
	Stands   []Stand           `json:"stands"`
	Beacons  []Beacon          `json:"beacons"`
	Screens  []Screen          `json:"screens"`
	Services map[string]string `json:"services"`
	Recent   []Play            `json:"recent"`
	// StandSources is which sources the stand is lit for: StandRecords or
	// StandAll (ADR-0039).
	StandSources string `json:"stand_sources"`
	// SpotifyDriver is whether Spotify's driver is switched on: DriverOn or
	// DriverOff (ADR-0051).
	SpotifyDriver string `json:"spotify_driver"`
}

type cover struct {
	sha   string
	mime  string
	image []byte
}

// View holds the picture. It is safe to use from several goroutines.
type View struct {
	prefix string
	now    func() time.Time

	mu        sync.RWMutex
	current   *Playing
	cover     *cover
	sources   map[string]*Source
	stands    map[string]*Stand
	beacons   map[string]*Beacon
	screens   map[string]*Screen
	services  map[string]*Presence
	recent    []Play
	lastKey   string
	standAll  bool
	spotifyOn bool
}

// New makes an empty View for topics under prefix.
func New(prefix string) *View {
	return &View{
		prefix:   prefix,
		now:      time.Now,
		sources:  map[string]*Source{},
		stands:   map[string]*Stand{},
		beacons:  map[string]*Beacon{},
		screens:  map[string]*Screen{},
		services: map[string]*Presence{},
	}
}

// Topics are the subscriptions that feed a View, relative to nothing. Each
// has to be granted to the console's account as written: the broker matches
// a subscription against what was granted as a whole, and says nothing when
// it refuses one.
func Topics(prefix string) []string {
	return []string{
		prefix + "/nowplaying/current",
		prefix + "/nowplaying/current/art",
		prefix + "/nowplaying/status",
		prefix + "/nowplaying/source/+/state",
		prefix + "/nowplaying/source/+/status",
		prefix + "/lightstand/+/scene",
		prefix + "/lightstand/+/state",
		prefix + "/lightstand/+/status",
		prefix + "/lightd/status",
		prefix + "/beacon/+/status",
		prefix + "/screen/+/status",
		StandTopic(prefix),
		SpotifySwitchTopic(prefix),
	}
}

// Apply takes one message, as heard live, and reports whether the picture
// changed. A message it cannot read is ignored and changes nothing: the page
// keeps showing what it last knew, as the stand does.
func (v *View) Apply(topic string, payload []byte) bool {
	return v.apply(topic, payload, false)
}

// ApplyStored takes a message the broker kept and sent on subscribing. It
// is Apply, except that a stored online is only a claim (ADR-0034).
func (v *View) ApplyStored(topic string, payload []byte) bool {
	return v.apply(topic, payload, true)
}

func (v *View) apply(topic string, payload []byte, retained bool) bool {
	rest, ok := strings.CutPrefix(topic, v.prefix+"/")
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")

	v.mu.Lock()
	defer v.mu.Unlock()

	switch {
	case rest == "nowplaying/current":
		p, ok := decodePlaying(payload)
		if !ok {
			return false
		}
		v.current = p
		v.remember(p)
		return true

	case rest == "nowplaying/current/art":
		return v.setCover(payload)

	case rest == "nowplaying/stand":
		// Empty is no setting, which is records. A word the console does not
		// know is what nowplayd does with one: records.
		all := false
		if len(payload) > 0 {
			var m struct {
				V       int    `json:"v"`
				Sources string `json:"sources"`
			}
			all = json.Unmarshal(payload, &m) == nil && m.V == ContractVersion && m.Sources == StandAll
		}
		changed := all != v.standAll
		v.standAll = all
		return changed

	case rest == "nowplaying/source/spotify/switch":
		// Empty is no setting, which is off, and so is anything the agent
		// could not read: it would be off there too.
		on := false
		if len(payload) > 0 {
			var m struct {
				V      int    `json:"v"`
				Driver string `json:"driver"`
			}
			on = json.Unmarshal(payload, &m) == nil && m.V == ContractVersion && m.Driver == DriverOn
		}
		changed := on != v.spotifyOn
		v.spotifyOn = on
		return changed

	case rest == "nowplaying/status":
		return v.service("nowplayd").hear(payload, retained, v.now())

	case rest == "lightd/status":
		return v.service("lightd").hear(payload, retained, v.now())

	case len(parts) == 4 && parts[0] == "nowplaying" && parts[1] == "source":
		s := v.source(parts[2])
		switch parts[3] {
		case "state":
			p, ok := decodePlaying(payload)
			if !ok {
				return false
			}
			s.Now = p
			return true
		case "status":
			return s.hear(payload, retained, v.now())
		}

	case len(parts) == 3 && parts[0] == "lightstand":
		s := v.stand(parts[1])
		switch parts[2] {
		case "scene":
			scene, ok := decodeScene(payload)
			if !ok {
				return false
			}
			s.Scene = scene
			return true
		case "state":
			var a struct {
				V int `json:"v"`
				Applied
			}
			if json.Unmarshal(payload, &a) != nil || a.V != ContractVersion {
				return false
			}
			s.Applied = &a.Applied
			return true
		case "status":
			return s.hear(payload, retained, v.now())
		}

	case len(parts) == 3 && parts[0] == "beacon" && parts[2] == "status":
		// Read before the beacon is made: a word that is neither does not
		// put a beacon on the page that nothing is known about.
		if presence(payload) == "" {
			return false
		}
		return v.beacon(parts[1]).hear(payload, retained, v.now())

	case len(parts) == 3 && parts[0] == "screen" && parts[2] == "status":
		if presence(payload) == "" {
			return false
		}
		return v.screen(parts[1]).hear(payload, retained, v.now())
	}
	return false
}

func (v *View) source(id string) *Source {
	s, ok := v.sources[id]
	if !ok {
		s = &Source{ID: id}
		v.sources[id] = s
	}
	return s
}

func (v *View) stand(id string) *Stand {
	s, ok := v.stands[id]
	if !ok {
		s = &Stand{ID: id}
		v.stands[id] = s
	}
	return s
}

func (v *View) beacon(id string) *Beacon {
	b, ok := v.beacons[id]
	if !ok {
		b = &Beacon{ID: id}
		v.beacons[id] = b
	}
	return b
}

func (v *View) screen(id string) *Screen {
	s, ok := v.screens[id]
	if !ok {
		s = &Screen{ID: id}
		v.screens[id] = s
	}
	return s
}

func (v *View) service(name string) *Presence {
	p, ok := v.services[name]
	if !ok {
		p = &Presence{}
		v.services[name] = p
	}
	return p
}

// Expire lapses every online, and every claim, not said again for Lapse,
// and reports whether the picture changed. It is called on a ticker: a part
// that has gone without a will says nothing, and silence has to be noticed.
func (v *View) Expire() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now, changed := v.now(), false
	for _, p := range v.services {
		changed = p.lapse(now) || changed
	}
	for _, s := range v.sources {
		changed = s.lapse(now) || changed
	}
	for _, s := range v.stands {
		changed = s.lapse(now) || changed
	}
	for _, b := range v.beacons {
		changed = b.lapse(now) || changed
	}
	for _, s := range v.screens {
		changed = s.lapse(now) || changed
	}
	return changed
}

// presence is "online" or "offline", or nothing for anything else.
func presence(payload []byte) string {
	switch word := strings.TrimSpace(string(payload)); word {
	case "online", "offline":
		return word
	}
	return ""
}

// setCover keeps the image on the cover topic. An empty message clears it;
// anything that is not a picture is ignored.
func (v *View) setCover(image []byte) bool {
	if len(image) == 0 {
		changed := v.cover != nil
		v.cover = nil
		return changed
	}
	if len(image) > MaxCover {
		return false
	}
	mime := http.DetectContentType(image)
	// Named, not "image/*": what is served back is served under this type,
	// and an SVG is a document that can carry script.
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" && mime != "image/gif" {
		return false
	}
	sum := sha256.Sum256(image)
	sha := hex.EncodeToString(sum[:])
	if v.cover != nil && v.cover.sha == sha {
		return false
	}
	v.cover = &cover{sha: sha, mime: mime, image: image}
	return true
}

// remember adds a play to the recent list when the current track is a new
// one that is playing. A pause, or the same track reported again, is not a
// play.
func (v *View) remember(p *Playing) {
	if p.State != "playing" || p.Track == nil {
		if p.State == "stopped" {
			v.lastKey = ""
		}
		return
	}
	key := p.Source + "\x00" + p.Track.Artist + "\x00" + p.Track.Album + "\x00" + p.Track.Title
	if key == v.lastKey {
		return
	}
	v.lastKey = key
	v.recent = append([]Play{{At: v.now().UTC(), Player: p.Player, Source: p.Source, Track: *p.Track}}, v.recent...)
	if len(v.recent) > keepRecent {
		v.recent = v.recent[:keepRecent]
	}
}

// Snapshot is the picture as it stands, safe to keep and to encode.
func (v *View) Snapshot() Snapshot {
	v.mu.RLock()
	defer v.mu.RUnlock()

	out := Snapshot{
		Sources:       make([]Source, 0, len(v.sources)),
		Stands:        make([]Stand, 0, len(v.stands)),
		Beacons:       make([]Beacon, 0, len(v.beacons)),
		Screens:       make([]Screen, 0, len(v.screens)),
		Services:      make(map[string]string, len(v.services)),
		Recent:        append([]Play{}, v.recent...),
		StandSources:  StandRecords,
		SpotifyDriver: DriverOff,
	}
	if v.standAll {
		out.StandSources = StandAll
	}
	if v.spotifyOn {
		out.SpotifyDriver = DriverOn
	}
	if v.current != nil {
		c := *v.current
		out.Current = &c
	}
	if v.cover != nil {
		out.LitCover = v.cover.sha
	}
	for _, s := range v.sources {
		out.Sources = append(out.Sources, *s)
	}
	for _, s := range v.stands {
		out.Stands = append(out.Stands, *s)
	}
	for _, b := range v.beacons {
		out.Beacons = append(out.Beacons, *b)
	}
	for _, s := range v.screens {
		out.Screens = append(out.Screens, *s)
	}
	for name, p := range v.services {
		if p.State != "" {
			out.Services[name] = p.State
		}
	}
	// Maps have no order, and a page that reshuffles on every update is
	// harder to read than one that is merely alphabetical.
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })
	sort.Slice(out.Stands, func(i, j int) bool { return out.Stands[i].ID < out.Stands[j].ID })
	sort.Slice(out.Beacons, func(i, j int) bool { return out.Beacons[i].ID < out.Beacons[j].ID })
	sort.Slice(out.Screens, func(i, j int) bool { return out.Screens[i].ID < out.Screens[j].ID })
	return out
}

// Cover returns the held cover if it is the one asked for.
func (v *View) Cover(sha string) (image []byte, mime string, ok bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.cover == nil || v.cover.sha != sha {
		return nil, "", false
	}
	return v.cover.image, v.cover.mime, true
}

func decodePlaying(payload []byte) (*Playing, bool) {
	var m struct {
		V          int        `json:"v"`
		Source     string     `json:"source"`
		Player     string     `json:"player"`
		State      string     `json:"state"`
		Track      *Track     `json:"track"`
		PositionMS *int64     `json:"position_ms"`
		ChangedAt  *time.Time `json:"changed_at"`
		Art        *struct {
			SHA256 string `json:"sha256"`
			From   string `json:"from"`
		} `json:"art"`
	}
	if json.Unmarshal(payload, &m) != nil || m.V != ContractVersion {
		return nil, false
	}
	switch m.State {
	case "playing", "paused", "stopped":
	default:
		return nil, false
	}
	p := &Playing{Source: m.Source, Player: m.Player, State: m.State, Track: m.Track,
		PositionMS: m.PositionMS, ChangedAt: m.ChangedAt}
	if m.Art != nil {
		p.Cover, p.CoverFrom = m.Art.SHA256, m.Art.From
	}
	return p, true
}

func decodeScene(payload []byte) (*Scene, bool) {
	var m struct {
		V int `json:"v"`
		Scene
	}
	if json.Unmarshal(payload, &m) != nil || m.V != ContractVersion || len(m.Palette) == 0 {
		return nil, false
	}
	// The firmware takes at most eight colors and clamps the rest; the page
	// draws what a stand would.
	if len(m.Palette) > 8 {
		m.Palette = m.Palette[:8]
	}
	m.Speed, m.Brightness = clamp01(m.Speed), clamp01(m.Brightness)
	return &m.Scene, true
}

func clamp01(x float64) float64 {
	switch {
	case x != x, x < 0: // NaN, or below
		return 0
	case x > 1:
		return 1
	}
	return x
}
