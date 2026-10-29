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

// --- commands (ADR-0020) -------------------------------------------------------

func TestTheCommandWireFormIsWhatTheReferencePageSays(t *testing.T) {
	got, err := EncodeCommand(Command{Command: PlayPause, From: "cover-screen-01"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"command":"play_pause","from":"cover-screen-01"}`
	if string(got) != want {
		t.Errorf("wire form changed:\n got %s\nwant %s", got, want)
	}

	got, err = EncodeCommand(Command{Command: Next})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"v":1,"command":"next"}` {
		t.Errorf("a command from no one in particular: %s", got)
	}
}

func TestEveryCommandRoundTrips(t *testing.T) {
	for _, name := range []string{PlayPause, Next, Previous, VolumeUp, VolumeDown, Duck, Unduck} {
		payload, err := EncodeCommand(Command{Command: name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		back, err := DecodeCommand(payload)
		if err != nil || back.Command != name || back.V != Version {
			t.Errorf("%s came back as %+v, %v", name, back, err)
		}
	}
}

func TestDecodeRefusesWhatIsNotACommand(t *testing.T) {
	for name, payload := range map[string]string{
		"not json":           `next`,
		"empty":              ``,
		"an unknown command": `{"v":1,"command":"shuffle"}`,
		"no command":         `{"v":1}`,
		"a later version":    `{"v":2,"command":"next"}`,
		"no version":         `{"command":"next"}`,
		"a state":            `{"v":1,"source":"itunes","state":"playing","changed_at":"2026-10-10T16:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCommand([]byte(payload)); err == nil {
				t.Error("accepted")
			}
		})
	}
	if _, err := EncodeCommand(Command{Command: "eject"}); err == nil {
		t.Error("an unknown command was encoded")
	}
}

// --- play_album (ADR-0040) ----------------------------------------------------

func TestPlayAlbumsWireForm(t *testing.T) {
	got, err := EncodeCommand(Command{Command: PlayAlbum, From: "voiced", Artist: "Van Halen", Album: "Van Halen"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"command":"play_album","from":"voiced","artist":"Van Halen","album":"Van Halen"}`
	if string(got) != want {
		t.Errorf("wire form changed:\n got %s\nwant %s", got, want)
	}
	back, err := DecodeCommand(got)
	if err != nil || back.Artist != "Van Halen" || back.Album != "Van Halen" {
		t.Errorf("came back as %+v, %v", back, err)
	}
}

// An album is played by both its names, or not at all: half a name is a
// guess, and the wrong record is worse than none.
func TestPlayAlbumNeedsBothNames(t *testing.T) {
	long := strings.Repeat("x", 201)
	for name, payload := range map[string]string{
		"no artist":          `{"v":1,"command":"play_album","album":"Van Halen"}`,
		"no album":           `{"v":1,"command":"play_album","artist":"Van Halen"}`,
		"a blank artist":     `{"v":1,"command":"play_album","artist":"  ","album":"Van Halen"}`,
		"an artist too long": `{"v":1,"command":"play_album","artist":"` + long + `","album":"Van Halen"}`,
		"an album too long":  `{"v":1,"command":"play_album","artist":"Van Halen","album":"` + long + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCommand([]byte(payload)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestACommandMayCarryFieldsThisProgramDoesNotKnow(t *testing.T) {
	if _, err := DecodeCommand([]byte(`{"v":1,"command":"volume_up","by":3}`)); err != nil {
		t.Errorf("an unknown field was refused: %v", err)
	}
}

// A source's command topic runs towards the driver. Whoever reads the sources
// subscribes to a filter that matches it, and must find nothing in it.
func TestACommandTopicIsNotSomethingASourceReports(t *testing.T) {
	if _, _, ok := ParseSourceTopic("eds", SourceCommandTopic("eds", "itunes")); ok {
		t.Error("a command topic was read as a source's report")
	}
	if got := SourceCommandTopic("eds", "itunes"); got != "eds/nowplaying/source/itunes/command" {
		t.Errorf("source command topic = %q", got)
	}
	if got := CommandTopic("eds"); got != "eds/nowplaying/command" {
		t.Errorf("command topic = %q", got)
	}
	if got := CurrentSmallArtTopic("eds"); got != "eds/nowplaying/current/art/small" {
		t.Errorf("small cover topic = %q", got)
	}
}

func TestTheStandSettingDefaultsToRecords(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    string
		err     bool
	}{
		{``, StandRecords, false},
		{`{"v":1,"sources":"all"}`, StandAll, false},
		{`{"v":1,"sources":"records"}`, StandRecords, false},
		{`{"v":1,"sources":"everything"}`, StandRecords, true},
		{`{"v":2,"sources":"all"}`, StandRecords, true},
		{`all`, StandRecords, true},
	} {
		got, err := DecodeStand([]byte(tc.payload))
		if got != tc.want || (err != nil) != tc.err {
			t.Errorf("DecodeStand(%q) = %q, %v; want %q, error %v", tc.payload, got, err, tc.want, tc.err)
		}
	}
	for _, sources := range []string{StandRecords, StandAll} {
		payload, err := EncodeStand(sources)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := DecodeStand(payload); err != nil || got != sources {
			t.Errorf("%s did not survive the wire: %q, %v", sources, got, err)
		}
	}
	if _, err := EncodeStand("everything"); err == nil {
		t.Error("encoded a setting nobody can read")
	}
}

// ADR-0051: nothing, or anything unreadable, is off.
func TestADriversSwitchDefaultsToOff(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    string
		err     bool
	}{
		{``, SwitchOff, false},
		{`{"v":1,"driver":"on"}`, SwitchOn, false},
		{`{"v":1,"driver":"off"}`, SwitchOff, false},
		{`{"v":1,"driver":"yes"}`, SwitchOff, true},
		{`{"v":2,"driver":"on"}`, SwitchOff, true},
		{`on`, SwitchOff, true},
	} {
		got, err := DecodeSwitch([]byte(tc.payload))
		if got != tc.want || (err != nil) != tc.err {
			t.Errorf("DecodeSwitch(%q) = %q, %v; want %q, error %v", tc.payload, got, err, tc.want, tc.err)
		}
	}
	for _, driver := range []string{SwitchOn, SwitchOff} {
		payload, err := EncodeSwitch(driver)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := DecodeSwitch(payload); err != nil || got != driver {
			t.Errorf("%s did not survive the wire: %q, %v", driver, got, err)
		}
	}
	if _, err := EncodeSwitch("yes"); err == nil {
		t.Error("encoded a setting nobody can read")
	}
	if got := SwitchTopic("eds", "spotify"); got != "eds/nowplaying/source/spotify/switch" {
		t.Errorf("SwitchTopic = %q", got)
	}
	// A source's reader learns nothing from its switch: the console reads it
	// on its own topic, and nowplayd not at all.
	if _, _, ok := ParseSourceTopic("eds", SwitchTopic("eds", "spotify")); ok {
		t.Error("the switch was read as one of a source's reports")
	}
}

// --- albums instead (ADR-0045) -------------------------------------------------

func TestPlayAlbumMayNameAFewAlbumsInstead(t *testing.T) {
	ok := Command{Command: PlayAlbum, Artist: "Genesis", Album: "From Genesis to Revelation", Instead: []string{"Trespass", "Nursery Cryme", "Foxtrot"}}
	payload, err := EncodeCommand(ok)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeCommand(payload)
	if err != nil || len(back.Instead) != 3 || back.Instead[0] != "Trespass" {
		t.Errorf("came back as %+v, %v", back, err)
	}
	for name, bad := range map[string][]string{
		"seven": {"a", "b", "c", "d", "e", "f", "g"},
		"empty": {"Trespass", " "},
		"long":  {strings.Repeat("x", 201)},
	} {
		c := ok
		c.Instead = bad
		if _, err := EncodeCommand(c); err == nil {
			t.Errorf("%s: encoded", name)
		}
	}
}
