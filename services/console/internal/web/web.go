// Package web serves the console's one page and keeps it current.
//
// The page is static files compiled into the binary, plus three things the
// server answers: the picture as JSON, the same picture pushed whenever it
// changes, and the cover the stand is lit from. It looks (ADR-0018), and
// changes one thing: which sources the stand is lit for (ADR-0039).
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/console/internal/view"
)

//go:embed static
var static embed.FS

// DefaultStripLEDs is the firmware's own default, CONFIG_LP_LED_COUNT: the
// stand has room for eighteen across its front.
const DefaultStripLEDs = 18

// MaxViewers bounds how many browsers are held open at once. Each is a
// request that never ends; this is a household's page, and a number far
// above a household is something else holding connections.
const MaxViewers = 64

// Links are the places the page points to. Each is used only if it is an
// http or https address.
type Links struct {
	Dashboard string `json:"dashboard,omitempty"`
	Docs      string `json:"docs,omitempty"`
}

// Server serves the page for one View.
type Server struct {
	View    *view.View
	Links   Links
	Version string
	Logger  *slog.Logger

	// Heartbeat is how often an idle viewer is sent a `beat` event. It keeps
	// anything in between from deciding the request is dead, and it is how
	// the page knows the stream is alive: one that misses a few reconnects.
	Heartbeat time.Duration

	// StripLEDs is how many LEDs a stand's strip is drawn with. The stand
	// does not report how many it has, so this is told what was built:
	// CONFIG_LP_LED_COUNT in the firmware. Zero is DefaultStripLEDs.
	StripLEDs int

	// Now is the clock. Nil is the wall clock.
	Now func() time.Time

	// Secure is set when this handler is served over TLS. A browser that has
	// reached the page that way is then told to keep doing so.
	Secure bool

	// Switch puts the stand's setting on the bus (ADR-0039): view.StandRecords
	// or view.StandAll. Nil, the page cannot switch it and the route is not
	// served.
	Switch func(sources string) error

	mu      sync.Mutex
	viewers map[chan struct{}]struct{}
}

// page is what the browser is given.
type page struct {
	view.Snapshot
	Links   Links  `json:"links"`
	Version string `json:"version,omitempty"`
	// StripLEDs is how many LEDs the strip is drawn with.
	StripLEDs int `json:"strip_leds"`
	// Now is the server's clock when this was sent. The page counts a
	// playing track on from it, so the viewer's own clock is never compared
	// with the one that stamped the track.
	Now time.Time `json:"now"`
}

func (s *Server) page() page {
	links := Links{}
	if safeLink(s.Links.Dashboard) {
		links.Dashboard = s.Links.Dashboard
	}
	if safeLink(s.Links.Docs) {
		links.Docs = s.Links.Docs
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	leds := s.StripLEDs
	if leds <= 0 {
		leds = DefaultStripLEDs
	}
	return page{Snapshot: s.View.Snapshot(), Links: links, Version: s.Version, StripLEDs: leds, Now: now().UTC()}
}

func safeLink(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")
}

// Changed tells every viewer the picture is new. It never waits: a viewer
// that has not yet taken the last change is told once, not twice.
func (s *Server) Changed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for viewer := range s.viewers {
		select {
		case viewer <- struct{}{}:
		default:
		}
	}
}

func (s *Server) join() (chan struct{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.viewers) >= MaxViewers {
		return nil, false
	}
	if s.viewers == nil {
		s.viewers = map[chan struct{}]struct{}{}
	}
	viewer := make(chan struct{}, 1)
	s.viewers[viewer] = struct{}{}
	return viewer, true
}

func (s *Server) leave(viewer chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.viewers, viewer)
}

// Handler is the whole site.
func (s *Server) Handler() http.Handler {
	files, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the directory is compiled in; this cannot fail at run time
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/cover/{sha}", s.cover)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// The page loads its own script, style and pictures and nothing
		// else, and nothing may frame it. Track names come from players and
		// catalogs; they are put on the page as text, and this is what
		// holds if that is ever got wrong.
		h.Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; "+
				"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if s.Secure {
			// A day, and this name only. Long enough that a browser does not
			// try plain HTTP again tomorrow; short enough that a certificate
			// left to expire does not lock the page away for a year.
			h.Set("Strict-Transport-Security", "max-age=86400")
		}

		if r.URL.Path == standPath && s.Switch != nil {
			h.Set("Cache-Control", "no-store")
			if r.Method != http.MethodPost {
				h.Set("Allow", "POST")
				http.Error(w, "the stand's setting is changed with a POST", http.StatusMethodNotAllowed)
				return
			}
			s.stand(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "the console only looks, but for the stand's setting", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/cover/") {
			// The picture changes; only a cover, named by its own hash, is
			// worth a browser keeping.
			h.Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

// standPath is the one route that changes anything (ADR-0039).
const standPath = "/api/stand"

// stand switches which sources the stand is lit for. The page sends JSON
// from its own origin; anything else is refused, so that another site open in
// the same browser cannot switch the stand by posting a form here. There is
// no login: anyone who can open the console can do this, and this is all
// they can do.
//
// It answers with what it published. Whether nowplayd heard it is said by
// the setting coming back on the bus, and the page shows that, not this.
func (s *Server) stand(w http.ResponseWriter, r *http.Request) {
	scheme := "http://"
	if s.Secure {
		scheme = "https://"
	}
	if origin := r.Header.Get("Origin"); origin != scheme+r.Host {
		http.Error(w, "the stand is switched from the console's own page", http.StatusForbidden)
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		http.Error(w, "the stand's setting is sent as JSON", http.StatusUnsupportedMediaType)
		return
	}
	var body struct {
		Sources string `json:"sources"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "not a setting", http.StatusBadRequest)
		return
	}
	if body.Sources != view.StandRecords && body.Sources != view.StandAll {
		http.Error(w, fmt.Sprintf("the stand is lit for %q or %q", view.StandRecords, view.StandAll), http.StatusBadRequest)
		return
	}
	if err := s.Switch(body.Sources); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("the stand's setting could not be published", "sources", body.Sources, "error", err)
		}
		http.Error(w, "the setting could not be put on the bus", http.StatusBadGateway)
		return
	}
	if s.Logger != nil {
		s.Logger.Info("the stand's sources were switched from the page", "sources", body.Sources, "from", r.RemoteAddr)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"v": view.ContractVersion, "sources": body.Sources})
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.page())
}

// events holds the request open and writes the picture each time it changes,
// starting with how it is now.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this connection cannot be held open", http.StatusInternalServerError)
		return
	}
	viewer, ok := s.join()
	if !ok {
		http.Error(w, "too many viewers", http.StatusServiceUnavailable)
		return
	}
	defer s.leave(viewer)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func() bool {
		data, err := json.Marshal(s.page())
		if err != nil {
			return false
		}
		// One line: JSON escapes a newline inside a string, so the event
		// cannot be ended early by a track's title.
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}

	heartbeat := s.Heartbeat
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-viewer:
			if !send() {
				return
			}
		case <-tick.C:
			// An event, not a comment: a page cannot see a comment, and it
			// is by hearing these that it knows the stream is not dead.
			if _, err := fmt.Fprint(w, "event: beat\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	image, mime, ok := s.View.Cover(r.PathValue("sha"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	// Named by the hash of its own bytes: it can never be a different image.
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	_, _ = w.Write(image)
}

// ToHTTPS is what the plain port answers once the console is served over
// TLS: every request is sent to the same path at https://name.
//
// The name is the one the certificate is for, not the one the request used.
// A browser sent by its own Host header would follow anything that pointed a
// name at this address, and would arrive at a certificate that does not match
// it anyway.
//
// /healthz still answers here, so a check that knows nothing of certificates
// can tell the process is up.
func ToHTTPS(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "the console only looks", http.StatusMethodNotAllowed)
			return
		}
		target := "https://" + name + r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}
