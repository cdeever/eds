package spotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/spotifyauth"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// answer is one reply from the stand-in for Spotify.
type answer struct {
	status int
	body   string
	header map[string]string
}

// api is Spotify, answering from a list. When the list runs out it repeats
// the last answer, as Spotify does while nothing changes.
type api struct {
	mu      sync.Mutex
	answers []answer
	asked   []string // every request made of the API, method and path
	bearers []string

	// Pictures are on another host in life. Here they are any path outside
	// the API's, and do not use up an answer.
	pictures   []string // each one asked for
	withBearer int      // how many of those carried the account's token
	noPictures bool
}

// pictureOf is the image the stand-in serves for a path: a PNG by its first
// bytes, and different for every path.
func pictureOf(path string) []byte { return []byte("\x89PNG\r\n\x1a\n" + path) }

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		a.mu.Lock()
		a.pictures = append(a.pictures, r.URL.Path)
		if r.Header.Get("Authorization") != "" {
			a.withBearer++
		}
		none := a.noPictures
		a.mu.Unlock()
		if none {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(pictureOf(r.URL.Path))
		return
	}
	a.mu.Lock()
	a.asked = append(a.asked, r.Method+" "+r.URL.Path)
	a.bearers = append(a.bearers, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	i := len(a.asked) - 1
	if i >= len(a.answers) {
		i = len(a.answers) - 1
	}
	ans := a.answers[i]
	a.mu.Unlock()

	for k, v := range ans.header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(ans.status)
	_, _ = io.WriteString(w, ans.body)
}

func (a *api) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.asked...)
}

// tokens hands out "access-1", "access-2", ... a new one after each Invalidate.
type tokens struct {
	mu          sync.Mutex
	issued      int
	valid       bool
	err         error
	authorized  time.Time
	invalidated int
}

func (t *tokens) Access(context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return "", t.err
	}
	if !t.valid {
		t.issued++
		t.valid = true
	}
	return fmt.Sprintf("access-%d", t.issued), nil
}

func (t *tokens) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.valid = false
	t.invalidated++
}

func (t *tokens) Token() spotifyauth.Token {
	at := t.authorized
	if at.IsZero() {
		at = time.Now()
	}
	return spotifyauth.Token{ClientID: "0123456789abcdef0123456789abcdef", RefreshToken: "refresh", AuthorizedAt: at}
}

// sink records what the driver reports.
type sink struct {
	mu      sync.Mutex
	updates []driver.Update
	online  []bool
}

func (s *sink) Online(online bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = append(s.online, online)
}

func (s *sink) Publish(u driver.Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, u)
}

func (s *sink) states() []contract.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.State, 0, len(s.updates))
	for _, u := range s.updates {
		out = append(out, u.State)
	}
	return out
}

func (s *sink) presence() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.online...)
}

// playing is Spotify's answer for a track, as its API documents it, with the
// album's picture in three sizes, widest first, as Spotify lists them.
func playing(device, title, album string, isPlaying bool) string {
	return fmt.Sprintf(`{
	  "device": {"name": %q, "type": "Computer", "is_private_session": false},
	  "is_playing": %t,
	  "currently_playing_type": "track",
	  "progress_ms": 61000,
	  "item": {
	    "name": %q,
	    "duration_ms": 215000,
	    "artists": [{"name": "Awolnation"}, {"name": "A Guest"}],
	    "album": {
	      "name": %q,
	      "artists": [{"name": "Awolnation"}],
	      "images": [
	        {"url": "IMAGE_HOST/image/%[5]s-640", "width": 640, "height": 640},
	        {"url": "IMAGE_HOST/image/%[5]s-300", "width": 300, "height": 300},
	        {"url": "IMAGE_HOST/image/%[5]s-64", "width": 64, "height": 64}
	      ]
	    }
	  }
	}`, device, isPlaying, title, album, strings.ReplaceAll(album, " ", "-"))
}

type run struct {
	api    *api
	tokens *tokens
	sink   *sink
	waits  []time.Duration
	err    error
}

// follow runs the driver through the answers given, one poll per answer, and
// then stops it. Nothing sleeps: each wait is recorded and returns at once.
func follow(t *testing.T, polls int, configure func(*Driver), answers ...answer) *run {
	t.Helper()
	r := &run{api: &api{answers: answers}, tokens: &tokens{}, sink: &sink{}}
	server := httptest.NewServer(r.api)
	t.Cleanup(server.Close)
	for i := range r.api.answers {
		r.api.answers[i].body = strings.ReplaceAll(r.api.answers[i].body, "IMAGE_HOST", server.URL)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &Driver{
		Source:   "spotify",
		API:      server.URL,
		HTTP:     server.Client(),
		Tokens:   r.tokens,
		Sink:     r.sink,
		Logger:   quiet,
		Devices:  []string{"Study PC"},
		Interval: 10 * time.Second,
	}
	d.after = func(wait time.Duration) <-chan time.Time {
		r.waits = append(r.waits, wait)
		if len(r.waits) >= polls {
			cancel()
		}
		done := make(chan time.Time, 1)
		done <- time.Time{}
		return done
	}
	if configure != nil {
		configure(d)
	}

	finished := make(chan error, 1)
	go func() { finished <- d.Run(ctx) }()
	select {
	case r.err = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the driver did not stop")
	}
	return r
}

func ok(body string) answer { return answer{status: http.StatusOK, body: body} }

func one(t *testing.T, a answer, configure func(*Driver)) contract.State {
	t.Helper()
	r := follow(t, 1, configure, a)
	states := r.sink.states()
	if len(states) != 1 {
		t.Fatalf("published %d states, want 1", len(states))
	}
	return states[0]
}

// --- what Spotify says, and what is reported ---------------------------------------

func TestATrackPlayingOnANamedDevice(t *testing.T) {
	s := one(t, ok(playing("Study PC", "Sail", "Megalithic Symphony", true)), nil)

	if s.State != contract.Playing || s.Source != "spotify" || s.Player != "spotify" {
		t.Fatalf("got %+v", s)
	}
	want := contract.Track{Title: "Sail", Artist: "Awolnation, A Guest", Album: "Megalithic Symphony",
		AlbumArtist: "Awolnation", DurationMS: 215000}
	if *s.Track != want {
		t.Fatalf("track %+v, want %+v", *s.Track, want)
	}
	if s.PositionMS == nil || *s.PositionMS != 61000 {
		t.Fatalf("position %v, want 61000", s.PositionMS)
	}
	if s.ChangedAt.IsZero() {
		t.Fatal("no time of change")
	}
	if _, err := contract.Encode(s); err != nil {
		t.Fatalf("the contract refuses it: %v", err)
	}
}

func TestATrackNotPlayingIsPaused(t *testing.T) {
	s := one(t, ok(playing("Study PC", "Sail", "Megalithic Symphony", false)), nil)
	if s.State != contract.Paused || s.Track == nil {
		t.Fatalf("got %+v", s)
	}
}

func TestDeviceNamesIgnoreCaseAndSpace(t *testing.T) {
	s := one(t, ok(playing("study pc", "Sail", "Megalithic Symphony", true)), func(d *Driver) {
		d.Devices = []string{" Kitchen ", "STUDY PC"}
	})
	if s.State != contract.Playing {
		t.Fatalf("got %+v", s)
	}
}

func TestWhatIsNotReported(t *testing.T) {
	private := strings.Replace(playing("Study PC", "Sail", "Megalithic Symphony", true),
		`"is_private_session": false`, `"is_private_session": true`, 1)
	advert := strings.Replace(playing("Study PC", "Advertisement", "", true),
		`"currently_playing_type": "track"`, `"currently_playing_type": "ad"`, 1)
	episode := strings.Replace(playing("Study PC", "Episode 12", "A Podcast", true),
		`"currently_playing_type": "track"`, `"currently_playing_type": "episode"`, 1)

	cases := map[string]answer{
		"nothing playing anywhere":   {status: http.StatusNoContent},
		"nothing loaded":             ok(`{"device": {"name": "Study PC"}, "is_playing": false, "item": null}`),
		"a device that is not named": ok(playing("Someone's iPhone", "Sail", "Megalithic Symphony", true)),
		"a private session":          ok(private),
		"an advertisement":           ok(advert),
		"a podcast episode":          ok(episode),
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			s := one(t, a, nil)
			// Stopped, and saying nothing about what it was.
			if s.State != contract.Stopped || s.Track != nil || s.PositionMS != nil || s.Art != nil {
				t.Fatalf("got %+v", s)
			}
		})
	}
}

// --- over time ---------------------------------------------------------------------

func TestARepeatedAnswerIsPublishedOnce(t *testing.T) {
	same := ok(playing("Study PC", "Sail", "Megalithic Symphony", true))
	r := follow(t, 4, nil, same, same, same, same)
	if n := len(r.sink.states()); n != 1 {
		t.Fatalf("published %d times for one track, want 1", n)
	}
	if got := r.sink.presence(); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("presence %v, want online then offline at the end", got)
	}
}

func TestChangesArePublishedAndTimed(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	r := follow(t, 5, func(d *Driver) {
		d.Now = func() time.Time { clock = clock.Add(10 * time.Second); return clock }
	},
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
		ok(playing("Study PC", "Kill Your Heroes", "Megalithic Symphony", true)),
		ok(playing("Study PC", "Kill Your Heroes", "Megalithic Symphony", false)),
		ok(playing("Someone's iPhone", "Kill Your Heroes", "Megalithic Symphony", true)),
	)

	states := r.sink.states()
	if len(states) != 4 {
		t.Fatalf("published %d states, want 4: %+v", len(states), states)
	}
	want := []contract.PlayState{contract.Playing, contract.Playing, contract.Paused, contract.Stopped}
	for i, s := range states {
		if s.State != want[i] {
			t.Fatalf("state %d is %s, want %s", i, s.State, want[i])
		}
		if i > 0 && !s.ChangedAt.After(states[i-1].ChangedAt) {
			t.Fatalf("state %d did not move the time of change", i)
		}
	}
	if states[1].Track.Title != "Kill Your Heroes" {
		t.Fatalf("second state is %q", states[1].Track.Title)
	}
}

// Spotify is reported without its cover (ADR-0016). Its answers carry image
// URLs all the same; the driver must not follow one, and must not pass one on.
// ADR-0031: the album's picture is the cover, fetched once for each album,
// and published with the state that names it.
func TestTheAlbumsPictureIsItsCoverFetchedOnceForEachAlbum(t *testing.T) {
	r := follow(t, 4, nil,
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
		ok(playing("Study PC", "Kill Your Heroes", "Megalithic Symphony", true)),
		ok(playing("Study PC", "Bloodshot", "Bloodshot", true)),
		ok(playing("Study PC", "Bloodshot", "Bloodshot", false)),
	)
	want := []string{"/image/Megalithic-Symphony-640", "/image/Bloodshot-640"}
	if got := r.api.pictures; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pictures asked for: %v, want %v", got, want)
	}
	if r.api.withBearer != 0 {
		t.Errorf("the account's token was sent to the pictures' host %d times", r.api.withBearer)
	}

	updates := r.sink.updates
	if len(updates) != 4 {
		t.Fatalf("published %d updates, want 4", len(updates))
	}
	// The cover goes out when it changes, and every state names the one it has.
	if string(updates[0].Cover) != string(pictureOf(want[0])) || updates[1].Cover != nil ||
		string(updates[2].Cover) != string(pictureOf(want[1])) || updates[3].Cover != nil {
		t.Errorf("covers went out with the wrong updates")
	}
	first, third := updates[0].State.Art, updates[2].State.Art
	if first == nil || third == nil || first.SHA256 == third.SHA256 || first.MIME != "image/png" {
		t.Fatalf("art = %+v then %+v", first, third)
	}
	if got := updates[1].State.Art; got == nil || got.SHA256 != first.SHA256 {
		t.Errorf("the second track of the album names %+v", got)
	}
	for _, request := range r.api.requests() {
		if request != "GET /v1/me/player" {
			t.Fatalf("the driver asked the API something else: %s", request)
		}
	}
}

// A picture that cannot be had is no cover: nowplayd will look one up.
func TestACoverThatCannotBeHadIsNoCover(t *testing.T) {
	gone := &api{noPictures: true}
	server := httptest.NewServer(gone)
	defer server.Close()
	d := &Driver{Source: "spotify", API: server.URL, HTTP: server.Client(), Logger: quiet}
	if art, cover := d.cover(context.Background(), server.URL+"/image/x"); art != nil || cover != nil {
		t.Errorf("a picture that was not there became a cover: %+v", art)
	}
	if art, cover := d.cover(context.Background(), "ftp://example.invalid/x"); art != nil || cover != nil {
		t.Errorf("a cover was fetched over something that is not HTTP")
	}
}

func TestPickTakesTheLargestThatIsNotTooLarge(t *testing.T) {
	for _, c := range []struct {
		images []image
		want   string
	}{
		{nil, ""},
		{[]image{{"a", 640}, {"b", 300}, {"c", 64}}, "a"},
		{[]image{{"c", 64}, {"a", 640}, {"b", 300}}, "a"},
		{[]image{{"big", 2000}, {"b", 300}}, "b"},
		{[]image{{"big", 2000}, {"bigger", 3000}}, "big"},
		{[]image{{"first", 0}, {"second", 0}}, "first"},
		{[]image{{"", 640}, {"b", 300}}, "b"},
	} {
		if got := pick(c.images); got != c.want {
			t.Errorf("pick(%v) = %q, want %q", c.images, got, c.want)
		}
	}
}

// --- when Spotify does not simply answer ---------------------------------------------

func TestARefusedTokenIsRefreshedAndTheQuestionAskedAgain(t *testing.T) {
	r := follow(t, 1, nil,
		answer{status: http.StatusUnauthorized},
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
	)
	if got := r.api.bearers; len(got) != 2 || got[0] != "access-1" || got[1] != "access-2" {
		t.Fatalf("tokens presented: %v", got)
	}
	if len(r.sink.states()) != 1 || len(r.waits) != 1 {
		t.Fatalf("want one state within one poll, got %d states over %d polls", len(r.sink.states()), len(r.waits))
	}
}

func TestBeingToldToWaitIsObeyedAndIsNotAnOutage(t *testing.T) {
	r := follow(t, 3, nil,
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
		answer{status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "45"}},
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)),
	)
	if r.waits[0] != 10*time.Second || r.waits[1] != 45*time.Second || r.waits[2] != 10*time.Second {
		t.Fatalf("waited %v, want 10s 45s 10s", r.waits)
	}
	if got := r.sink.presence(); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("presence %v: a rate limit must not read as offline", got)
	}
	if n := len(r.sink.states()); n != 1 {
		t.Fatalf("published %d states, want 1", n)
	}
}

func TestAShortRetryAfterDoesNotSpeedPollingUp(t *testing.T) {
	r := follow(t, 1, nil, answer{status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "1"}})
	if r.waits[0] != 10*time.Second {
		t.Fatalf("waited %s, want the usual 10s", r.waits[0])
	}
}

func TestOneFailureIsToleratedSeveralAreAnOutage(t *testing.T) {
	down := answer{status: http.StatusBadGateway}
	up := ok(playing("Study PC", "Sail", "Megalithic Symphony", true))

	r := follow(t, 3, nil, up, down, up)
	if got := r.sink.presence(); len(got) != 2 {
		t.Fatalf("presence %v: one failed request must not read as offline", got)
	}

	r = follow(t, 5, nil, up, down, down, down, up)
	got := r.sink.presence()
	if len(got) != 4 || !got[0] || got[1] || !got[2] || got[3] {
		t.Fatalf("presence %v, want online, offline, online, and offline at the end", got)
	}
	// Coming back with the same track is not news.
	if n := len(r.sink.states()); n != 1 {
		t.Fatalf("published %d states, want 1", n)
	}
}

func TestBeingRefusedOutrightIsOfflineAndNotHurried(t *testing.T) {
	refused := answer{status: http.StatusForbidden,
		body: `{"error": {"status": 403, "message": "Active premium subscription required for the owner of the app."}}`}
	up := ok(playing("Study PC", "Sail", "Megalithic Symphony", true))

	var log strings.Builder
	r := follow(t, 3, func(d *Driver) { d.Logger = slog.New(slog.NewTextHandler(&log, nil)) }, up, refused, up)

	if r.waits[1] != forbiddenWait {
		t.Fatalf("waited %s after a refusal, want %s", r.waits[1], forbiddenWait)
	}
	got := r.sink.presence()
	if len(got) != 4 || !got[0] || got[1] || !got[2] {
		t.Fatalf("presence %v, want online, offline, online, and offline at the end", got)
	}
	// Spotify's own reason is the only thing that says what to do about it.
	if !strings.Contains(log.String(), "premium subscription required") {
		t.Fatalf("the reason was not passed on: %s", log.String())
	}
}

func TestAnEndedAuthorizationStopsTheDriver(t *testing.T) {
	r := follow(t, 99, func(d *Driver) {
		d.Tokens.(*tokens).err = fmt.Errorf("refresh: %w", spotifyauth.ErrReauthorize)
	}, ok(playing("Study PC", "Sail", "Megalithic Symphony", true)))

	if !errors.Is(r.err, spotifyauth.ErrReauthorize) {
		t.Fatalf("Run returned %v, want ErrReauthorize", r.err)
	}
	if len(r.api.requests()) != 0 || len(r.waits) != 0 {
		t.Fatalf("it kept going: %d requests, %d waits", len(r.api.requests()), len(r.waits))
	}
	if got := r.sink.presence(); len(got) != 1 || got[0] {
		t.Fatalf("presence %v, want offline only", got)
	}
}

func TestWithNoDevicesNamedTheDriverRefusesToStart(t *testing.T) {
	r := follow(t, 99, func(d *Driver) { d.Devices = []string{" ", ""} },
		ok(playing("Study PC", "Sail", "Megalithic Symphony", true)))
	if r.err == nil || !strings.Contains(r.err.Error(), "NP_SPOTIFY_DEVICES") {
		t.Fatalf("Run returned %v", r.err)
	}
	if len(r.api.requests()) != 0 {
		t.Fatal("Spotify was asked anyway")
	}
}

func TestAnAuthorizationAboutToEndIsWarnedOf(t *testing.T) {
	var log strings.Builder
	follow(t, 1, func(d *Driver) {
		d.Logger = slog.New(slog.NewTextHandler(&log, nil))
		d.Tokens.(*tokens).authorized = time.Now().Add(-spotifyauth.Lifetime + 5*24*time.Hour)
	}, answer{status: http.StatusNoContent})
	if !strings.Contains(log.String(), "authorize again") {
		t.Fatalf("no warning in: %s", log.String())
	}

	log.Reset()
	follow(t, 1, func(d *Driver) { d.Logger = slog.New(slog.NewTextHandler(&log, nil)) },
		answer{status: http.StatusNoContent})
	if strings.Contains(log.String(), "authorize again") {
		t.Fatalf("warned with months left: %s", log.String())
	}
}

func TestListDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/player/devices" || r.Header.Get("Authorization") != "Bearer access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"devices": [
		  {"name": "Study PC", "type": "Computer", "is_active": true},
		  {"name": "Someone's iPhone", "type": "Smartphone", "is_active": false}]}`)
	}))
	defer server.Close()

	devices, err := ListDevices(context.Background(), server.Client(), server.URL, "access")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].Name != "Study PC" || !devices[0].Active || devices[1].Type != "Smartphone" {
		t.Fatalf("got %+v", devices)
	}
}
