package dacp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func str(tag, s string) []byte { return Encode(tag, []byte(s)) }

func u32(tag string, v uint32) []byte {
	return Encode(tag, []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// playStatus is a cmst the shape iTunes sends while a track plays.
func playStatus(revision uint32, caps byte, title string) []byte {
	return Encode("cmst", join(
		u32("mstt", 200),
		u32("cmsr", revision),
		Encode("caps", []byte{caps}),
		Encode("cash", []byte{0}),
		Encode("canp", []byte{0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4}),
		str("cann", title),
		str("cana", "Miles Davis"),
		str("canl", "Kind of Blue"),
		str("cang", "Jazz"),
		EncodeUint64("asai", 0xABCDEF0123456789),
		u32("cant", 500000),
		u32("cast", 562000),
	))
}

func TestDecodeAPlayStatus(t *testing.T) {
	items, err := Decode(playStatus(42, capsPlaying, "So What"))
	if err != nil {
		t.Fatal(err)
	}
	root, ok := Find(items, "cmst")
	if !ok || len(root.Children) != 12 {
		t.Fatalf("cmst has %d children", len(root.Children))
	}

	s := parseStatus(root.Children)
	if s.Revision != 42 || !s.Playing || s.Paused {
		t.Errorf("revision %d playing=%v paused=%v", s.Revision, s.Playing, s.Paused)
	}
	if s.Title != "So What" || s.Artist != "Miles Davis" || s.Album != "Kind of Blue" || s.Genre != "Jazz" {
		t.Errorf("track = %+v", s)
	}
	if s.AlbumID != 0xABCDEF0123456789 || len(s.NowPlaying) != 16 {
		t.Errorf("album id %x, now playing %d bytes", s.AlbumID, len(s.NowPlaying))
	}
	if s.DurationMS != 562000 || s.PositionMS() != 62000 {
		t.Errorf("duration %d position %d", s.DurationMS, s.PositionMS())
	}
}

func TestPlayStates(t *testing.T) {
	for caps, want := range map[byte][2]bool{capsPlaying: {true, false}, capsPaused: {false, true}, capsStopped: {false, false}} {
		items, _ := Decode(playStatus(1, caps, "T"))
		root, _ := Find(items, "cmst")
		s := parseStatus(root.Children)
		if s.Playing != want[0] || s.Paused != want[1] {
			t.Errorf("caps %d: playing=%v paused=%v", caps, s.Playing, s.Paused)
		}
	}
}

// A stopped player sends no track at all, and that must read as nothing.
func TestAStoppedPlayerHasNoTrack(t *testing.T) {
	items, err := Decode(Encode("cmst", join(u32("mstt", 200), u32("cmsr", 7), Encode("caps", []byte{capsStopped}))))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := Find(items, "cmst")
	s := parseStatus(root.Children)
	if s.Playing || s.Paused || s.Title != "" || s.PositionMS() != 0 || s.Revision != 7 {
		t.Errorf("status = %+v", s)
	}
}

// What comes off the network is not to be trusted to be well formed.
func TestDecodeRefusesMalformedInput(t *testing.T) {
	deep := []byte{}
	for i := 0; i < 20; i++ {
		deep = Encode("cmst", deep)
	}
	for name, data := range map[string][]byte{
		"truncated header":         []byte("cmst\x00\x00"),
		"length past the end":      []byte("cann\x00\x00\x00\x10short"),
		"a huge length":            []byte("cann\xff\xff\xff\xffx"),
		"a bad child":              Encode("cmst", []byte("cann\x00\x00\x00\x10short")),
		"nested without end":       deep,
		"trailing bytes after one": append(str("cann", "ok"), 1, 2, 3),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err == nil {
				t.Error("decoded")
			}
		})
	}

	if items, err := Decode(nil); err != nil || len(items) != 0 {
		t.Errorf("nothing decoded to %v, %v", items, err)
	}
}

func TestUintWidths(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want uint64
	}{{[]byte{7}, 7}, {[]byte{1, 0}, 256}, {[]byte{0, 0, 1, 0}, 256}, {[]byte{0, 0, 0, 0, 0, 0, 1, 0}, 256}} {
		if got, ok := (Item{Data: tc.data}).Uint(); !ok || got != tc.want {
			t.Errorf("%v read as %d, %v", tc.data, got, ok)
		}
	}
	if _, ok := (Item{Data: []byte{1, 2, 3}}).Uint(); ok {
		t.Error("three bytes read as an integer")
	}
}

// --- pairing -----------------------------------------------------------------

func TestPairingCodeIsTheHashThePlayerComputes(t *testing.T) {
	// MD5 of the GUID then each PIN digit as a two-byte character. Computed
	// independently: printf '0000000000000001' '1\0' '2\0' '3\0' '4\0' | md5
	got := PairingCode("0000000000000001", "1234")
	if got != "690E6FF61E0D7C747654A42AED17047D" {
		t.Errorf("code = %q", got)
	}
	if PairingCode("0000000000000001", "1235") == got || PairingCode("0000000000000002", "1234") == got {
		t.Error("the code does not depend on both the GUID and the PIN")
	}
}

func TestThePlayersCallBackIsAnsweredWithTheGUID(t *testing.T) {
	const guid, pin = "00000000DEADBEEF", "4071"
	done := make(chan Paired, 1)
	srv := httptest.NewServer(PairHandler(guid, pin, "EdS", done))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/pair?pairingcode=" + strings.ToLower(PairingCode(guid, pin)) + "&servicename=ABC")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/x-dmap-tagged" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	body := make([]byte, 256)
	n, _ := resp.Body.Read(body)
	items, err := Decode(body[:n])
	if err != nil {
		t.Fatal(err)
	}
	cmpg, ok := Find(items, "cmpg")
	if v, _ := cmpg.Uint(); !ok || v != 0xDEADBEEF {
		t.Errorf("answered with guid %x", v)
	}
	if name, _ := Find(items, "cmnm"); name.String() != "EdS" {
		t.Errorf("name = %q", name.String())
	}

	select {
	case paired := <-done:
		if net.ParseIP(paired.Host) == nil {
			t.Errorf("the player's address was recorded as %q", paired.Host)
		}
	case <-time.After(time.Second):
		t.Error("a correct code did not complete the pairing")
	}
}

func TestAWrongCodePairsNothing(t *testing.T) {
	const guid, pin = "00000000DEADBEEF", "4071"
	done := make(chan Paired, 1)
	srv := httptest.NewServer(PairHandler(guid, pin, "EdS", done))
	defer srv.Close()

	for _, path := range []string{
		"/pair?pairingcode=" + PairingCode(guid, "0000"),
		"/pair?pairingcode=",
		"/pair",
		"/other?pairingcode=" + PairingCode(guid, pin),
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s was accepted", path)
		}
	}
	select {
	case <-done:
		t.Error("a wrong code completed the pairing")
	default:
	}
}

func TestGUIDsAndPINs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		guid, err := NewGUID()
		if err != nil || len(guid) != 16 || guid != strings.ToUpper(guid) || seen[guid] {
			t.Fatalf("guid %q, %v", guid, err)
		}
		if _, err := strconv.ParseUint(guid, 16, 64); err != nil {
			t.Fatalf("guid %q is not hex", guid)
		}
		seen[guid] = true

		pin, err := NewPIN()
		if n, convErr := strconv.Atoi(pin); err != nil || convErr != nil || len(pin) != 4 || n < 0 {
			t.Fatalf("pin %q", pin)
		}
	}
	if txt := TXT("ABC", "EdS"); txt["Pair"] != "ABC" || txt["DvNm"] != "EdS" || txt["RemV"] != "10000" {
		t.Errorf("txt = %v", txt)
	}
}

// --- the client, against a stand-in player -----------------------------------

type player struct {
	t        *testing.T
	guid     string
	revision uint32
	title    string
	change   chan string // send a title to change track
	requests []string
}

func (p *player) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.requests = append(p.requests, r.URL.RequestURI())
	if r.Header.Get("Viewer-Only-Client") != "1" {
		p.t.Errorf("%s arrived without Viewer-Only-Client", r.URL.Path)
	}

	switch r.URL.Path {
	case "/login":
		if r.URL.Query().Get("pairing-guid") != "0x"+p.guid {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write(Encode("mlog", join(u32("mstt", 200), u32("mlid", 99))))

	case "/ctrl-int/1/playstatusupdate":
		if r.URL.Query().Get("session-id") != "99" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		asked, _ := strconv.ParseUint(r.URL.Query().Get("revision-number"), 10, 32)
		if uint32(asked) == p.revision {
			// Nothing newer: hold the request, as the player does.
			select {
			case title := <-p.change:
				p.title = title
				p.revision++
			case <-r.Context().Done():
				return
			}
		}
		_, _ = w.Write(playStatus(p.revision, capsPlaying, p.title))

	case "/ctrl-int/1/nowplayingartwork":
		if r.URL.Query().Get("mw") != "600" {
			p.t.Errorf("artwork asked for at %q", r.URL.Query().Get("mw"))
		}
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n-cover"))

	default:
		http.NotFound(w, r)
	}
}

func against(t *testing.T, p *player) *Client {
	t.Helper()
	p.t = t
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	n, _ := strconv.Atoi(port)
	return NewClient(host, n)
}

func TestLoginThenAskThenWait(t *testing.T) {
	p := &player{guid: "00000000DEADBEEF", revision: 5, title: "So What", change: make(chan string, 1)}
	c := against(t, p)
	ctx := context.Background()

	if err := c.Login(ctx, "00000000DEADBEEF"); err != nil {
		t.Fatal(err)
	}
	now, err := c.Now(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if now.Title != "So What" || now.Revision != 5 {
		t.Fatalf("now = %+v", now)
	}

	// The push: Wait must not return until the track changes.
	got := make(chan Status, 1)
	go func() {
		next, err := c.Wait(ctx, now.Revision)
		if err != nil {
			t.Errorf("wait: %v", err)
		}
		got <- next
	}()

	select {
	case early := <-got:
		t.Fatalf("Wait returned before anything changed: %+v", early)
	case <-time.After(200 * time.Millisecond):
	}

	p.change <- "Freddie Freeloader"
	select {
	case next := <-got:
		if next.Title != "Freddie Freeloader" || next.Revision != 6 {
			t.Errorf("after the change: %+v", next)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return when the track changed")
	}
}

// A player that does not know this remote says 403, and that has to be
// distinguishable: the answer is to pair again, not to retry.
func TestAnUnknownRemoteIsRefusedDistinctly(t *testing.T) {
	c := against(t, &player{guid: "00000000DEADBEEF"})
	err := c.Login(context.Background(), "1111111111111111")

	var refused *StatusError
	if !errors.As(err, &refused) || refused.Code != http.StatusForbidden {
		t.Errorf("err = %v, want a 403", err)
	}
}

func TestWaitGivesUpWhenToldTo(t *testing.T) {
	p := &player{guid: "00000000DEADBEEF", revision: 5, title: "So What", change: make(chan string)}
	c := against(t, p)
	if err := c.Login(context.Background(), p.guid); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := c.Wait(ctx, 5); err == nil {
		t.Error("Wait returned without a change or an error")
	}
}

func TestArtwork(t *testing.T) {
	p := &player{guid: "00000000DEADBEEF", revision: 5, title: "So What", change: make(chan string, 1)}
	c := against(t, p)
	if err := c.Login(context.Background(), p.guid); err != nil {
		t.Fatal(err)
	}
	art, err := c.Artwork(context.Background(), 600)
	if err != nil || !strings.HasPrefix(string(art), "\x89PNG") {
		t.Errorf("art = %q, %v", art, err)
	}
}

func TestNoArtworkIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	n, _ := strconv.Atoi(port)

	art, err := NewClient(host, n).Artwork(context.Background(), 600)
	if art != nil || err != nil {
		t.Errorf("art = %v, err = %v", art, err)
	}
}
