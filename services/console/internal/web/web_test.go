package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cdeever/eds/services/console/internal/view"
)

const playing = `{"v":1,"source":"itunes","player":"itunes","state":"playing",
  "track":{"title":"House Party","artist":"The J. Geils Band","album":"Bloodshot"}}`

// A real, tiny PNG.
var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
	"\x00\x00\x00\rIDATx\x9cc\xf8\xcf\xc0\xf0\x1f\x00\x05\x00\x01\xff\x89\x99=\x1d\x00\x00\x00\x00IEND\xaeB`\x82")

func site(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := &Server{View: view.New("eds"), Version: "test", Heartbeat: time.Hour,
		Links: Links{Dashboard: "https://grafana.example/d/x", Docs: "javascript:alert(1)"}}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestThePageAndItsFilesAreServed(t *testing.T) {
	_, ts := site(t)
	for path, want := range map[string]string{
		"/":           "<title>EdS</title>",
		"/app.js":     "EventSource",
		"/effects.js": "BREATHE_FLOOR",
		"/app.css":    "--accent",
		"/icon.svg":   "<svg",
		"/healthz":    "ok",
	} {
		resp, body := get(t, ts.URL+path)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("%s: %d, missing %q", path, resp.StatusCode, want)
		}
	}
	// The effects' tests are beside the page, not part of it.
	if resp, _ := get(t, ts.URL+"/effects.test.mjs"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a test file is being served: %d", resp.StatusCode)
	}
}

func TestEveryAnswerCarriesThePolicy(t *testing.T) {
	_, ts := site(t)
	for _, path := range []string{"/", "/app.js", "/api/state", "/nothing-here"} {
		resp, _ := get(t, ts.URL+path)
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'self'") ||
			strings.Contains(csp, "unsafe") {
			t.Errorf("%s: policy %q", path, csp)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", path)
		}
	}
}

// The page is its own script and style and no one else's: the policy only
// holds if the page does not need what it forbids.
func TestThePageNeedsNothingThePolicyForbids(t *testing.T) {
	_, ts := site(t)
	_, html := get(t, ts.URL+"/")
	for _, forbidden := range []string{"<style", "style=\"", "onclick", "onload", "javascript:", "http://", "https://"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("the page contains %q", forbidden)
		}
	}
	_, js := get(t, ts.URL+"/app.js")
	for _, forbidden := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval("} {
		if strings.Contains(js, forbidden) {
			t.Errorf("the script uses %s", forbidden)
		}
	}
}

func TestTheConsoleOnlyLooks(t *testing.T) {
	_, ts := site(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, path := range []string{"/", "/api/state", "/api/cover/abc"} {
			req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader("{}"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: %d", method, path, resp.StatusCode)
			}
		}
	}
}

func TestStateIsThePictureAndOnlySafeLinks(t *testing.T) {
	s, ts := site(t)
	s.View.Apply("eds/nowplaying/current", []byte(playing))

	resp, body := get(t, ts.URL+"/api/state")
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type %q", ct)
	}
	var got struct {
		Current struct {
			Track struct{ Title string }
		}
		Links   map[string]string
		Version string
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.Current.Track.Title != "House Party" || got.Version != "test" {
		t.Fatalf("got %+v", got)
	}
	// A link that is not a web address is not handed to the page at all.
	if got.Links["dashboard"] != "https://grafana.example/d/x" || got.Links["docs"] != "" {
		t.Fatalf("links %v", got.Links)
	}
}

func TestTheCoverIsServedOnlyUnderItsOwnHash(t *testing.T) {
	s, ts := site(t)
	if !s.View.Apply("eds/nowplaying/current/art", png) {
		t.Fatal("the test picture was not taken as a cover")
	}
	sha := s.View.Snapshot().LitCover

	resp, body := get(t, ts.URL+"/api/cover/"+sha)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || body != string(png) {
		t.Fatalf("cover: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("cache-control %q", resp.Header.Get("Cache-Control"))
	}
	if resp, _ := get(t, ts.URL+"/api/cover/0000"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("another hash: %d", resp.StatusCode)
	}
}

// read returns the next data line of an event stream.
func read(t *testing.T, lines *bufio.Reader) string {
	t.Helper()
	for {
		line, err := lines.ReadString('\n')
		if err != nil {
			t.Fatalf("the stream ended: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
}

func TestAViewerIsSentThePictureAndThenEachChange(t *testing.T) {
	s, ts := site(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	lines := bufio.NewReader(resp.Body)

	if first := read(t, lines); strings.Contains(first, "House Party") || !strings.Contains(first, `"sources":[]`) {
		t.Fatalf("first event: %s", first)
	}

	// A title with a line break in it must not end the event early.
	s.View.Apply("eds/nowplaying/current", []byte(strings.Replace(playing, "House Party", `House\nParty`, 1)))
	s.Changed()
	second := read(t, lines)
	var got struct {
		Current struct {
			Track struct{ Title string }
		}
	}
	if err := json.Unmarshal([]byte(second), &got); err != nil || got.Current.Track.Title != "House\nParty" {
		t.Fatalf("second event: %v %s", err, second)
	}
}

func TestViewersAreCountedOutAsWellAsIn(t *testing.T) {
	s, ts := site(t)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	read(t, bufio.NewReader(resp.Body))

	count := func() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.viewers) }
	if count() != 1 {
		t.Fatalf("%d viewers, want 1", count())
	}
	cancel()
	resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for count() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if count() != 0 {
		t.Fatal("a viewer who left is still being held")
	}
	// Telling nobody is not a problem.
	s.Changed()
}

func TestTooManyViewersAreTurnedAway(t *testing.T) {
	s, ts := site(t)
	s.mu.Lock()
	s.viewers = map[chan struct{}]struct{}{}
	for i := 0; i < MaxViewers; i++ {
		s.viewers[make(chan struct{}, 1)] = struct{}{}
	}
	s.mu.Unlock()
	if resp, _ := get(t, ts.URL+"/api/events"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d", resp.StatusCode)
	}
}
