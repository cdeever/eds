package view

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// picture is a real, small PNG, so that what is sniffed is what a cover is.
func picture(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(0, 0, color.RGBA{shade, 0, 0, 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

const bloodshot = `{"v":1,"source":"itunes","player":"itunes","state":"playing",
  "track":{"title":"House Party","artist":"The J. Geils Band","album":"Bloodshot","duration_ms":250000},
  "position_ms":1000,"changed_at":"2026-10-11T15:00:00Z","art":{"sha256":"abc","from":"player"}}`

const reaper = `{"v":1,"source":"spotify","player":"spotify","state":"playing",
  "track":{"title":"(Don't Fear) The Reaper","artist":"Blue Öyster Cult","album":"Agents Of Fortune"},
  "changed_at":"2026-10-11T15:05:00Z"}`

func TestTheCurrentTrackIsRead(t *testing.T) {
	v := New("eds")
	if !v.Apply("eds/nowplaying/current", []byte(bloodshot)) {
		t.Fatal("a new current track is a change")
	}
	c := v.Snapshot().Current
	if c == nil || c.State != "playing" || c.Player != "itunes" || c.Track.Title != "House Party" ||
		c.Track.Album != "Bloodshot" || c.Cover != "abc" || c.CoverFrom != "player" || *c.PositionMS != 1000 {
		t.Fatalf("got %+v", c)
	}
}

func TestWhatCannotBeReadChangesNothing(t *testing.T) {
	v := New("eds")
	v.Apply("eds/nowplaying/current", []byte(bloodshot))

	for name, payload := range map[string]string{
		"not json":         `playing`,
		"another version":  `{"v":2,"state":"playing"}`,
		"no version":       `{"state":"playing"}`,
		"an unknown state": `{"v":1,"state":"rewinding"}`,
	} {
		if v.Apply("eds/nowplaying/current", []byte(payload)) {
			t.Errorf("%s was taken as a change", name)
		}
	}
	if c := v.Snapshot().Current; c == nil || c.Track.Title != "House Party" {
		t.Fatalf("the picture was disturbed: %+v", c)
	}

	// Somebody else's topics are not this tenant's business.
	if v.Apply("other/nowplaying/current", []byte(reaper)) || v.Apply("eds/unknown/thing", []byte("x")) {
		t.Fatal("a topic that is not ours was taken")
	}
}

func TestTheLitCoverIsKeptAndServedByItsHash(t *testing.T) {
	v := New("eds")
	img := picture(t, 200)
	if !v.Apply("eds/nowplaying/current/art", img) {
		t.Fatal("a cover is a change")
	}
	if v.Apply("eds/nowplaying/current/art", img) {
		t.Fatal("the same cover again is not")
	}
	sha := v.Snapshot().LitCover
	got, mime, ok := v.Cover(sha)
	if !ok || mime != "image/png" || !bytes.Equal(got, img) {
		t.Fatalf("cover: ok=%v mime=%q", ok, mime)
	}
	if _, _, ok := v.Cover("some-other-hash"); ok {
		t.Fatal("a cover was served under a hash that is not its own")
	}

	// An empty retained message is how a topic is cleared.
	if !v.Apply("eds/nowplaying/current/art", nil) || v.Snapshot().LitCover != "" {
		t.Fatal("clearing the topic did not clear the cover")
	}
}

func TestOnlyAPictureIsKeptAsACover(t *testing.T) {
	v := New("eds")
	for name, payload := range map[string][]byte{
		"a web page": []byte("<!doctype html><script>alert(1)</script>"),
		"an svg":     []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"text":       []byte("not a cover"),
		"too big":    append(picture(t, 1), make([]byte, MaxCover)...),
	} {
		if v.Apply("eds/nowplaying/current/art", payload) || v.Snapshot().LitCover != "" {
			t.Errorf("%s was kept as a cover", name)
		}
	}
}

// Spotify is reported without its cover (ADR-0016): the current track then
// names none, and the cover held is the one the stand was last lit from.
func TestACoverlessTrackLeavesTheLitCoverAlone(t *testing.T) {
	v := New("eds")
	v.Apply("eds/nowplaying/current/art", picture(t, 200))
	lit := v.Snapshot().LitCover
	v.Apply("eds/nowplaying/current", []byte(reaper))

	s := v.Snapshot()
	if s.Current.Cover != "" || s.LitCover != lit {
		t.Fatalf("current names %q, lit is %q (was %q)", s.Current.Cover, s.LitCover, lit)
	}
}

func TestSourcesAndTheirPresence(t *testing.T) {
	v := New("eds")
	v.Apply("eds/nowplaying/source/spotify/status", []byte("online"))
	v.Apply("eds/nowplaying/source/itunes/state", []byte(bloodshot))
	v.Apply("eds/nowplaying/source/itunes/status", []byte("offline"))
	if v.Apply("eds/nowplaying/source/itunes/status", []byte("offline")) {
		t.Fatal("presence repeated is not a change")
	}
	if v.Apply("eds/nowplaying/source/itunes/status", []byte("maybe")) {
		t.Fatal("presence is online or offline")
	}

	s := v.Snapshot().Sources
	if len(s) != 2 || s[0].ID != "itunes" || s[1].ID != "spotify" {
		t.Fatalf("sources %+v", s)
	}
	if *s[0].Online || s[0].Now.Track.Album != "Bloodshot" || !*s[1].Online || s[1].Now != nil {
		t.Fatalf("sources %+v", s)
	}
}

func TestAStandsSceneStateAndPresence(t *testing.T) {
	v := New("eds")
	scene := `{"v":1,"effect":"breathe","speed":0.4,"brightness":0.8,
	  "palette":[{"rgb":[201,0,39],"weight":0.65},{"rgb":[172,0,48],"weight":0.35}]}`
	if !v.Apply("eds/lightstand/lp-stand-01/scene", []byte(scene)) {
		t.Fatal("a scene is a change")
	}
	v.Apply("eds/lightstand/lp-stand-01/state", []byte(`{"v":1,"effect":"breathe","colors":2,"brightness":0.80}`))
	v.Apply("eds/lightstand/lp-stand-01/status", []byte("online"))

	stands := v.Snapshot().Stands
	if len(stands) != 1 {
		t.Fatalf("stands %+v", stands)
	}
	st := stands[0]
	if st.ID != "lp-stand-01" || !*st.Online || st.Scene.Effect != "breathe" || len(st.Scene.Palette) != 2 ||
		st.Scene.Palette[0].RGB != [3]uint8{201, 0, 39} || st.Applied.Colors != 2 {
		t.Fatalf("stand %+v scene %+v", st, st.Scene)
	}
}

func TestABeaconsPresence(t *testing.T) {
	v := New("eds")
	if v.Apply("eds/beacon/turntable-power-01/status", []byte("on")) {
		t.Fatal("a word that is neither online nor offline is not a change")
	}
	if got := v.Snapshot().Beacons; len(got) != 0 {
		t.Fatalf("a beacon was made from a word that says nothing: %+v", got)
	}

	if !v.Apply("eds/beacon/turntable-power-01/status", []byte("online")) {
		t.Fatal("a beacon coming on is a change")
	}
	if v.Apply("eds/beacon/turntable-power-01/status", []byte("online")) {
		t.Fatal("said twice is not a change")
	}
	v.Apply("eds/beacon/another-01/status", []byte("offline"))

	got := v.Snapshot().Beacons
	if len(got) != 2 || got[0].ID != "another-01" || *got[0].Online ||
		got[1].ID != "turntable-power-01" || !*got[1].Online {
		t.Fatalf("beacons %+v", got)
	}
}

func TestAScreensPresence(t *testing.T) {
	v := New("eds")
	if v.Apply("eds/screen/cover-screen-01/status", []byte("")) {
		t.Fatal("an emptied topic is not a change")
	}
	if !v.Apply("eds/screen/cover-screen-01/status", []byte("online")) {
		t.Fatal("a screen coming on is a change")
	}
	v.Apply("eds/screen/cover-screen-01/status", []byte("offline"))

	got := v.Snapshot().Screens
	if len(got) != 1 || got[0].ID != "cover-screen-01" || *got[0].Online {
		t.Fatalf("screens %+v", got)
	}
}

func TestASceneIsHeldToWhatAStandWouldTake(t *testing.T) {
	v := New("eds")
	if v.Apply("eds/lightstand/a/scene", []byte(`{"v":1,"effect":"solid","palette":[]}`)) {
		t.Fatal("a scene with no colors was taken")
	}
	if v.Apply("eds/lightstand/a/scene", []byte(`{"v":2,"effect":"solid","palette":[{"rgb":[1,2,3],"weight":1}]}`)) {
		t.Fatal("a scene of another version was taken")
	}

	many := `{"v":1,"effect":"sweep","speed":7,"brightness":-1,"palette":[` +
		`{"rgb":[1,1,1],"weight":0.1},{"rgb":[2,2,2],"weight":0.1},{"rgb":[3,3,3],"weight":0.1},` +
		`{"rgb":[4,4,4],"weight":0.1},{"rgb":[5,5,5],"weight":0.1},{"rgb":[6,6,6],"weight":0.1},` +
		`{"rgb":[7,7,7],"weight":0.1},{"rgb":[8,8,8],"weight":0.1},{"rgb":[9,9,9],"weight":0.2}]}`
	v.Apply("eds/lightstand/a/scene", []byte(many))
	scene := v.Snapshot().Stands[0].Scene
	if len(scene.Palette) != 8 || scene.Speed != 1 || scene.Brightness != 0 {
		t.Fatalf("scene %+v", scene)
	}
}

func TestServices(t *testing.T) {
	v := New("eds")
	v.Apply("eds/lightd/status", []byte("online"))
	v.Apply("eds/nowplaying/status", []byte("offline"))
	s := v.Snapshot().Services
	if s["lightd"] != "online" || s["nowplayd"] != "offline" {
		t.Fatalf("services %v", s)
	}
}

func TestRecentPlaysAreNewTracksPlaying(t *testing.T) {
	v := New("eds")
	clock := time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC)
	v.now = func() time.Time { clock = clock.Add(time.Minute); return clock }

	paused := `{"v":1,"source":"itunes","player":"itunes","state":"paused","track":{"title":"House Party","artist":"The J. Geils Band","album":"Bloodshot"}}`
	v.Apply("eds/nowplaying/current", []byte(paused)) // found paused at start-up: not a play
	v.Apply("eds/nowplaying/current", []byte(bloodshot))
	v.Apply("eds/nowplaying/current", []byte(bloodshot)) // position moved on
	v.Apply("eds/nowplaying/current", []byte(paused))
	v.Apply("eds/nowplaying/current", []byte(bloodshot)) // resumed: the same play
	v.Apply("eds/nowplaying/current", []byte(reaper))

	recent := v.Snapshot().Recent
	if len(recent) != 2 || recent[0].Track.Title != "(Don't Fear) The Reaper" || recent[0].Player != "spotify" ||
		recent[1].Track.Title != "House Party" || !recent[0].At.After(recent[1].At) {
		t.Fatalf("recent %+v", recent)
	}

	for i := 0; i < keepRecent+10; i++ {
		v.Apply("eds/nowplaying/current", []byte(bloodshot))
		v.Apply("eds/nowplaying/current", []byte(reaper))
	}
	if n := len(v.Snapshot().Recent); n != keepRecent {
		t.Fatalf("kept %d plays, want %d", n, keepRecent)
	}
}

func TestASnapshotEncodesAndIsItsOwnCopy(t *testing.T) {
	v := New("eds")
	empty, err := json.Marshal(v.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	// A page given an empty picture must find lists, not nulls.
	for _, want := range []string{`"sources":[]`, `"stands":[]`, `"recent":[]`, `"services":{}`} {
		if !bytes.Contains(empty, []byte(want)) {
			t.Errorf("empty snapshot lacks %s: %s", want, empty)
		}
	}

	v.Apply("eds/nowplaying/current", []byte(bloodshot))
	snap := v.Snapshot()
	snap.Current.State = "tampered"
	if v.Snapshot().Current.State != "playing" {
		t.Fatal("changing a snapshot changed the view")
	}
}

func TestTopicsCoverWhatApplyReads(t *testing.T) {
	want := map[string]bool{
		"eds/nowplaying/current": true, "eds/nowplaying/current/art": true, "eds/nowplaying/status": true,
		"eds/nowplaying/source/+/state": true, "eds/nowplaying/source/+/status": true,
		"eds/lightstand/+/scene": true, "eds/lightstand/+/state": true, "eds/lightstand/+/status": true,
		"eds/lightd/status": true, "eds/beacon/+/status": true,
		"eds/screen/+/status": true,
	}
	got := Topics("eds")
	if len(got) != len(want) {
		t.Fatalf("topics %v", got)
	}
	for _, topic := range got {
		if !want[topic] {
			t.Errorf("unexpected topic %s", topic)
		}
	}
}
