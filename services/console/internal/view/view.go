// Package view is what the console knows: the state of EdS as the bus last
// described it.
//
// It is fed messages and holds a picture. It knows the two contracts every
// subscriber knows - the now-playing state and the scene descriptor - and
// reads only the fields it shows, so a field added to either does not break
// it. It makes no network calls and decides nothing: what is current is
// nowplayd's decision, and what a cover becomes is lightd's.
package view

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	// CoverFrom is where that cover came from: the player, or a catalogue.
	CoverFrom string `json:"cover_from,omitempty"`
}

// Source is one driver: whether it can reach its player, and what it sees.
type Source struct {
	ID     string   `json:"id"`
	Online *bool    `json:"online,omitempty"` // nil until its presence has been heard
	Now    *Playing `json:"now,omitempty"`
}

// Swatch is one colour of a scene and its share of the strip.
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
	ID     string `json:"id"`
	Online *bool  `json:"online,omitempty"`
	// Scene is what the stand was sent. Applied is what it reported taking.
	// A stand that is offline was sent a scene it has not seen.
	Scene   *Scene   `json:"scene,omitempty"`
	Applied *Applied `json:"applied,omitempty"`
}

// Beacon is a device on a switch, which says only that the switch is on
// (ADR-0023). Offline is its ordinary state, not a fault: it is what the
// thing it shares a switch with being turned off looks like.
type Beacon struct {
	ID     string `json:"id"`
	Online *bool  `json:"online,omitempty"`
}

// Screen is a cover screen (ADR-0020): a device that shows the cover and can
// ask for the next track. The page shows only whether it is there.
type Screen struct {
	ID     string `json:"id"`
	Online *bool  `json:"online,omitempty"`
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

	mu       sync.RWMutex
	current  *Playing
	cover    *cover
	sources  map[string]*Source
	stands   map[string]*Stand
	beacons  map[string]*Beacon
	screens  map[string]*Screen
	services map[string]string
	recent   []Play
	lastKey  string
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
		services: map[string]string{},
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
	}
}

// Apply takes one message and reports whether the picture changed. A message
// it cannot read is ignored and changes nothing: the page keeps showing what
// it last knew, as the stand does.
func (v *View) Apply(topic string, payload []byte) bool {
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

	case rest == "nowplaying/status":
		return v.setService("nowplayd", payload)

	case rest == "lightd/status":
		return v.setService("lightd", payload)

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
			return setOnline(&s.Online, payload)
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
			return setOnline(&s.Online, payload)
		}

	case len(parts) == 3 && parts[0] == "beacon" && parts[2] == "status":
		// Read before the beacon is made: a word that is neither does not
		// put a beacon on the page that nothing is known about.
		if presence(payload) == "" {
			return false
		}
		return setOnline(&v.beacon(parts[1]).Online, payload)

	case len(parts) == 3 && parts[0] == "screen" && parts[2] == "status":
		if presence(payload) == "" {
			return false
		}
		return setOnline(&v.screen(parts[1]).Online, payload)
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

func (v *View) setService(name string, payload []byte) bool {
	word := presence(payload)
	if word == "" || v.services[name] == word {
		return false
	}
	v.services[name] = word
	return true
}

func setOnline(into **bool, payload []byte) bool {
	word := presence(payload)
	if word == "" {
		return false
	}
	online := word == "online"
	if *into != nil && **into == online {
		return false
	}
	*into = &online
	return true
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
		Sources:  make([]Source, 0, len(v.sources)),
		Stands:   make([]Stand, 0, len(v.stands)),
		Beacons:  make([]Beacon, 0, len(v.beacons)),
		Screens:  make([]Screen, 0, len(v.screens)),
		Services: make(map[string]string, len(v.services)),
		Recent:   append([]Play{}, v.recent...),
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
	for name, word := range v.services {
		out.Services[name] = word
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
	// The firmware takes at most eight colours and clamps the rest; the page
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
