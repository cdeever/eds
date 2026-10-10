package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func playing() State {
	pos := int64(12000)
	return State{
		Source: "itunes-den", Player: "itunes", State: Playing,
		Track:      &Track{Title: "Title", Artist: "Artist", Album: "Album", DurationMS: 215000},
		PositionMS: &pos,
		ChangedAt:  time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC),
		Art:        &Art{SHA256: "abc123", MIME: "image/png", From: "player"},
	}
}

// The wire form is the contract. If this changes, it is a new version.
func TestTheWireFormIsWhatTheReferencePageSays(t *testing.T) {
	got, err := Encode(playing())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"source":"itunes-den","player":"itunes","state":"playing",` +
		`"track":{"title":"Title","artist":"Artist","album":"Album","duration_ms":215000},` +
		`"position_ms":12000,"changed_at":"2026-10-10T16:00:00Z",` +
		`"art":{"sha256":"abc123","mime":"image/png","from":"player"}}`
	if string(got) != want {
		t.Errorf("wire form changed:\n got %s\nwant %s", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	payload, err := Encode(playing())
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := playing()
	want.V = Version
	if !Same(&back, &want) || *back.PositionMS != 12000 || !back.ChangedAt.Equal(want.ChangedAt) {
		t.Errorf("round trip lost something: %+v", back)
	}
}

// A source may know very little, and saying so must be allowed.
func TestASourceMayBeIncomplete(t *testing.T) {
	payload, err := Encode(State{Source: "tidal", Player: "tidal", State: Playing,
		Track: &Track{Title: "Only a title"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "album") || strings.Contains(string(payload), `"art"`) {
		t.Errorf("absent fields were sent: %s", payload)
	}
	if _, err := Decode(payload); err != nil {
		t.Errorf("an incomplete source was refused: %v", err)
	}
}

func TestAMismatchedVersionIsRefusedNotGuessedAt(t *testing.T) {
	for name, payload := range map[string]string{
		"a later version": `{"v":2,"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z"}`,
		"no version":      `{"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z"}`,
		"version zero":    `{"v":0,"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(payload)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestDecodeRefusesWhatIsNotAState(t *testing.T) {
	for name, payload := range map[string]string{
		"not json":         `online`,
		"empty":            ``,
		"an unknown state": `{"v":1,"source":"itunes","state":"buffering","changed_at":"2026-10-10T16:00:00Z"}`,
		"no state":         `{"v":1,"source":"itunes","changed_at":"2026-10-10T16:00:00Z"}`,
		"art with no hash": `{"v":1,"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z","art":{"mime":"image/png"}}`,
		"a wildcard id":    `{"v":1,"source":"+","state":"playing","changed_at":"2026-10-10T16:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(payload)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// Unknown fields must be tolerated, or no field can ever be added without
// breaking a subscriber that has not been rebuilt.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	payload := `{"v":1,"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z","new_thing":{"x":1}}`
	if _, err := Decode([]byte(payload)); err != nil {
		t.Errorf("an unknown field was refused: %v", err)
	}
}

func TestEncodeStampsTheVersionAndRefusesNonsense(t *testing.T) {
	payload, err := Encode(State{Source: "itunes", State: Stopped})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal(payload, &raw)
	if raw["v"] != float64(Version) {
		t.Errorf("v = %v", raw["v"])
	}

	if _, err := Encode(State{Source: "itunes"}); err == nil {
		t.Error("a state with no play state was encoded")
	}
	if _, err := Encode(State{Source: "bad/id", State: Playing}); err == nil {
		t.Error("a source id with a slash was encoded")
	}
}

func TestSourceIDs(t *testing.T) {
	for _, ok := range []string{"itunes", "itunes-den", "dev-1", "a", "spotify"} {
		if !ValidSourceID(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "+", "#", "a/b", "A", "-lead", "has space", "ünï", strings.Repeat("a", 64)} {
		if ValidSourceID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSameIgnoresPositionAndTime(t *testing.T) {
	a, b := playing(), playing()
	pos := int64(99000)
	b.PositionMS = &pos
	b.ChangedAt = b.ChangedAt.Add(time.Minute)
	if !Same(&a, &b) {
		t.Error("position or time made two states differ")
	}

	for name, change := range map[string]func(*State){
		"state":  func(s *State) { s.State = Paused },
		"title":  func(s *State) { s.Track = &Track{Title: "Other", Artist: "Artist", Album: "Album", DurationMS: 215000} },
		"cover":  func(s *State) { s.Art = &Art{SHA256: "different"} },
		"no art": func(s *State) { s.Art = nil },
		"source": func(s *State) { s.Source = "other" },
	} {
		c := playing()
		change(&c)
		if Same(&a, &c) {
			t.Errorf("a different %s was called the same", name)
		}
	}

	if !Same(nil, nil) || Same(&a, nil) || Same(nil, &a) {
		t.Error("nil handling")
	}
}

func TestTrackKeysForgiveSpelling(t *testing.T) {
	a := Track{Title: "Title", Artist: "Artist", Album: "Album"}
	b := Track{Title: " title ", Artist: "ARTIST", Album: "album"}
	if a.Key() != b.Key() {
		t.Error("case and space made one track two")
	}
	if a.Key() == (Track{Title: "Title", Artist: "Artist", Album: "Other"}).Key() {
		t.Error("a different album was the same track")
	}

	// A compilation is one album whoever performs each track.
	x := Track{Title: "1", Artist: "Someone", AlbumArtist: "Various Artists", Album: "Hits"}
	y := Track{Title: "2", Artist: "Someone Else", AlbumArtist: "Various Artists", Album: "Hits"}
	if x.AlbumKey() != y.AlbumKey() {
		t.Error("a compilation was split by track artist")
	}
}

func TestTopics(t *testing.T) {
	if got := SourceStateTopic("eds", "itunes-den"); got != "eds/nowplaying/source/itunes-den/state" {
		t.Errorf("state topic = %q", got)
	}
	if got := CurrentArtTopic("eds"); got != "eds/nowplaying/current/art" {
		t.Errorf("current art topic = %q", got)
	}

	for topic, want := range map[string][2]string{
		"eds/nowplaying/source/itunes-den/state":  {"itunes-den", "state"},
		"eds/nowplaying/source/itunes-den/status": {"itunes-den", "status"},
		"eds/nowplaying/source/spotify/art":       {"spotify", "art"},
	} {
		id, kind, ok := ParseSourceTopic("eds", topic)
		if !ok || id != want[0] || kind != want[1] {
			t.Errorf("%s parsed as %q %q %v", topic, id, kind, ok)
		}
	}
	for _, topic := range []string{
		"eds/nowplaying/current",
		"eds/nowplaying/source/itunes-den",
		"eds/nowplaying/source/itunes-den/state/extra",
		"eds/nowplaying/source/itunes-den/other",
		"other/nowplaying/source/itunes-den/state",
		"eds/nowplaying/source//state",
	} {
		if _, _, ok := ParseSourceTopic("eds", topic); ok {
			t.Errorf("%s was taken for a source topic", topic)
		}
	}
}
