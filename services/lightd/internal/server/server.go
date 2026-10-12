// Package server is lightd's HTTP surface, and the one path from a cover to a
// scene.
//
// POST /v1/cover was built as the interface an album cover resolver would
// call, so that nothing here would be discarded when one arrived. It has
// arrived as a topic rather than a caller (ADR-0009): ApplyCover takes the
// same image bytes from MQTT and runs them down the same path.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/lightd/internal/broker"
	"github.com/cdeever/eds/services/lightd/internal/palette"
	"github.com/cdeever/eds/services/lightd/internal/scene"
)

const maxUpload = 16 << 20 // 16 MiB; cover art is never close to this.

// Extractor is the palette service, as far as this package cares.
type Extractor interface {
	Extract(ctx context.Context, image []byte, swatches int) (*palette.Response, error)
}

// Options configure a Server.
type Options struct {
	Extractor    Extractor
	Publisher    broker.Publisher
	TopicPrefix  string
	DefaultStand string
	Swatches     int
	Scene        scene.Options
	// Looks are the choices a look leaves open (ADR-0019).
	Looks  scene.LookOptions
	Logger *slog.Logger
}

// Server publishes scenes in response to HTTP requests.
type Server struct {
	opts Options

	mu   sync.RWMutex
	last map[string]scene.Scene

	// The cover from the topic, and the look said to belong to a cover. They
	// arrive separately and in either order, so each is kept, and the room is
	// lit from the pair (ADR-0019).
	held heldCover
	// early is a look that arrived before its cover. It is kept until that
	// cover comes, and never applied to the one held meanwhile.
	early coverLook
	// idle says the last thing sent to the default stand from the topic was
	// the idle scene, so being told again that there is no cover sends
	// nothing.
	idle   bool
	render sync.Mutex // one lighting of the room at a time
}

// heldCover is the cover the stand is, or is about to be, lit from.
type heldCover struct {
	image []byte
	sha   string
	// look is the look named for this cover, or "" for none.
	look string
	// lit says a scene for this cover has gone out; shown is the look it went
	// out with, or "" for the cover's own colors.
	lit   bool
	shown string
}

// coverLook is the last word on whether a cover has a look of its own.
type coverLook struct {
	name string
	art  string // the SHA-256 of the cover it is for
}

// New builds a Server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{opts: opts, last: make(map[string]scene.Scene)}
}

// Routes returns the mux, so main stays a wiring function.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/cover", s.handleCover)
	mux.HandleFunc("POST /v1/scene", s.handleScene)
	mux.HandleFunc("GET /v1/scene", s.handleLastScene)
	return mux
}

type diagnostics struct {
	SwatchesExtracted int            `json:"swatches_extracted"`
	BackgroundDropped float64        `json:"background_dropped"`
	Fallback          bool           `json:"fallback"`
	Source            palette.Source `json:"source"`
}

type publishResult struct {
	Stand   string       `json:"stand"`
	Topic   string       `json:"topic"`
	Scene   scene.Scene  `json:"scene"`
	Palette *diagnostics `json:"palette,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// describe renders a scene's palette for a log line: each color as hex with
// its share of the strip, largest first as the scene has them.
func describe(sc scene.Scene) string {
	parts := make([]string, 0, len(sc.Palette))
	for _, c := range sc.Palette {
		parts = append(parts, fmt.Sprintf("#%02x%02x%02x %.0f%%", c.RGB[0], c.RGB[1], c.RGB[2], c.Weight*100))
	}
	return strings.Join(parts, ", ")
}

// coverError says which kind of failure a cover met, in the terms HTTP has
// for it. The topic input has no caller to tell, but logs the same words.
type coverError struct {
	status int
	err    error
}

func (e *coverError) Error() string { return e.err.Error() }
func (e *coverError) Unwrap() error { return e.err }

// cover is the whole vertical: image in, lights change. Both inputs end here,
// so a cover behaves the same however it arrived.
func (s *Server) cover(ctx context.Context, image []byte, stand string, swatches int, effect, via string) (*publishResult, *coverError) {
	extracted, err := s.opts.Extractor.Extract(ctx, image, swatches)
	if err != nil {
		// An image the extractor rejected is the sender's mistake; an
		// extractor that is down is not. Reporting both as 502 would send
		// someone hunting a healthy service over a corrupt JPEG.
		var upstream *palette.StatusError
		if errors.As(err, &upstream) && upstream.ClientFault() {
			return nil, &coverError{http.StatusBadRequest, err}
		}
		return nil, &coverError{http.StatusBadGateway, err}
	}

	opts := s.opts.Scene
	if effect != "" {
		opts.Effect = effect
	}

	built, err := scene.Build(extracted.Swatches, opts)
	if err != nil {
		return nil, &coverError{http.StatusUnprocessableEntity, err}
	}

	if err := s.publish(stand, built); err != nil {
		return nil, &coverError{http.StatusBadGateway, err}
	}

	s.opts.Logger.Info("published scene",
		"via", via,
		"stand", stand,
		"effect", built.Effect,
		"colors", len(built.Palette),
		// Which colors, not only how many. Only the latest scene is kept on
		// the broker, so this line is the one record of what a cover produced
		// - and "what did it pick for that sleeve?" is the tuning question.
		"palette", describe(built),
		"background_dropped", extracted.BackgroundDropped,
		"fallback", extracted.Fallback,
	)

	return &publishResult{
		Stand: stand,
		Topic: broker.SceneTopic(s.opts.TopicPrefix, stand),
		Scene: built,
		Palette: &diagnostics{
			SwatchesExtracted: len(extracted.Swatches),
			BackgroundDropped: extracted.BackgroundDropped,
			Fallback:          extracted.Fallback,
			Source:            extracted.Source,
		},
	}, nil
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	image, err := readImage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	swatches := s.opts.Swatches
	if raw := r.URL.Query().Get("n"); raw != "" {
		parsed, convErr := strconv.Atoi(raw)
		if convErr != nil || parsed < 1 || parsed > 16 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("n must be an integer 1-16"))
			return
		}
		swatches = parsed
	}

	result, failed := s.cover(r.Context(), image, s.stand(r), swatches, r.URL.Query().Get("effect"), "http")
	if failed != nil {
		writeError(w, failed.status, failed.err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ApplyCover takes a cover that arrived on the cover topic and lights the
// default stand from it, with the configured swatch count and effect.
//
// It is given image bytes and nothing else. What the image is a cover *of* is
// not lightd's to know: the topic is a setting, and the music stays on the
// other side of it.
//
// A cover that cannot be used is reported and otherwise ignored, so the stand
// keeps showing the last good scene - the same answer the firmware gives a
// scene it cannot render.
func (s *Server) ApplyCover(ctx context.Context, image []byte) error {
	if len(image) == 0 {
		return errors.New("no image supplied")
	}
	if len(image) > maxUpload {
		return fmt.Errorf("cover is %d bytes; the most accepted is %d", len(image), maxUpload)
	}

	sum := sha256.Sum256(image)
	sha := hex.EncodeToString(sum[:])

	s.mu.Lock()
	if s.held.sha != sha {
		s.held = heldCover{image: image, sha: sha}
		if s.early.art == sha {
			s.held.look = s.early.name
		}
	}
	s.mu.Unlock()
	return s.light(ctx)
}

// ApplyLook takes the word on whether a cover has a look of its own
// (ADR-0019): a name, or none, and the hash of the cover it is for.
//
// The look and its cover come on two topics and in either order. So this only
// records what was said, and then lights the room from whatever pair it now
// holds. A look for a cover that is not the one held changes nothing in the
// room: it is early, and is kept for its cover, or late, and its cover has
// gone. The cover held keeps the look it has until its own is said again.
func (s *Server) ApplyLook(ctx context.Context, payload []byte) error {
	var said struct {
		V    int    `json:"v"`
		Look string `json:"look"`
		Art  string `json:"art"`
	}
	if err := json.Unmarshal(payload, &said); err != nil {
		return fmt.Errorf("a look that cannot be read: %w", err)
	}
	switch {
	case said.V != scene.ContractVersion:
		return fmt.Errorf("a look of version %d; this reads version %d", said.V, scene.ContractVersion)
	case said.Art == "":
		return errors.New("a look that names no cover")
	}

	s.mu.Lock()
	if s.held.sha == said.Art {
		s.held.look = said.Look
	} else {
		s.early = coverLook{name: said.Look, art: said.Art}
	}
	s.mu.Unlock()
	return s.light(ctx)
}

// ClearCover takes the word that there is no cover: the cover topic was
// emptied, which is how whoever publishes it says nothing is playing
// (ADR-0024). The default stand is sent the idle scene, and the cover held
// is let go, so the same cover coming back lights the room again.
//
// What stopped playing, and why, is not lightd's to know. It is told there
// is no image, and that is all.
func (s *Server) ClearCover(ctx context.Context) error {
	s.render.Lock()
	defer s.render.Unlock()

	s.mu.Lock()
	already := s.idle
	s.held = heldCover{}
	s.mu.Unlock()
	if already {
		return nil
	}

	idle := scene.Idle()
	if err := s.publish(s.opts.DefaultStand, idle); err != nil {
		return &coverError{http.StatusBadGateway, err}
	}
	s.mu.Lock()
	s.idle = true
	s.mu.Unlock()
	s.opts.Logger.Info("published scene",
		"via", "topic",
		"stand", s.opts.DefaultStand,
		"idle", true,
		"effect", idle.Effect,
		"brightness", idle.Brightness,
	)
	return nil
}

// light makes the stand show what the held cover should look like now: its
// look, if one was named for it and is known here, else its own colors. It
// does nothing when that is already what was sent, so the look and the cover
// arriving in either order light the room once where they can, and a look
// repeated lights it not at all.
func (s *Server) light(ctx context.Context) error {
	s.render.Lock()
	defer s.render.Unlock()

	s.mu.RLock()
	held := s.held
	s.mu.RUnlock()
	if held.image == nil {
		return nil
	}

	want := ""
	var preset scene.Scene
	if held.look != "" {
		if sc, known := scene.Look(held.look, s.opts.Scene, s.opts.Looks); known {
			want, preset = held.look, sc
		} else {
			// Said once per cover: this runs again when the cover is retried.
			if !held.lit {
				s.opts.Logger.Warn("a cover came with a look this does not know; lighting it from its own colors",
					"look", held.look)
			}
		}
	}
	if held.lit && held.shown == want {
		return nil
	}

	if want != "" {
		// A look needs nothing from the cover but its arrival. The extractor
		// is not asked, and being down does not matter.
		if err := s.publish(s.opts.DefaultStand, preset); err != nil {
			return &coverError{http.StatusBadGateway, err}
		}
		s.opts.Logger.Info("published scene",
			"via", "topic",
			"stand", s.opts.DefaultStand,
			"look", want,
			"effect", preset.Effect,
			"colors", len(preset.Palette),
			"palette", describe(preset),
		)
	} else if _, failed := s.cover(ctx, held.image, s.opts.DefaultStand, s.opts.Swatches, "", "topic"); failed != nil {
		return failed
	}

	s.mu.Lock()
	// Only if it is still the cover held: a newer one may have come meanwhile.
	if s.held.sha == held.sha {
		s.held.lit, s.held.shown = true, want
	}
	s.mu.Unlock()
	return nil
}

// Temporary reports whether a cover failed for a reason that may pass: palette
// or the broker not answering. An image that palette rejected, or one with
// nothing in it to light a room from, will fail the same way every time.
func Temporary(err error) bool {
	var failed *coverError
	return errors.As(err, &failed) && failed.status == http.StatusBadGateway
}

// ApplyCoverRetrying is ApplyCover for a cover that will not be sent again.
//
// Over HTTP a caller who gets a 502 can try again. A cover from the topic has
// no caller: it is retained, it arrives once when lightd subscribes, and if it
// is dropped the stand shows the wrong record until the album changes. And
// the moment lightd subscribes is the moment it has just started - which, on
// a workload where everything restarts together, is the moment palette is
// least likely to be answering yet.
//
// So a temporary failure is tried again, up to attempts times, wait apart. A
// failure that is not temporary is returned at once.
func (s *Server) ApplyCoverRetrying(ctx context.Context, image []byte, attempts int, wait time.Duration) error {
	var err error
	for attempt := 1; ; attempt++ {
		if err = s.ApplyCover(ctx, image); err == nil || !Temporary(err) || attempt >= attempts {
			return err
		}
		s.opts.Logger.Info("cover not applied yet; trying again", "attempt", attempt, "of", attempts, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// handleScene publishes a scene supplied directly. This is how the stand gets
// exercised before any image path exists, and how a specific effect gets
// tested without hunting for cover art that produces it.
func (s *Server) handleScene(w http.ResponseWriter, r *http.Request) {
	var supplied scene.Scene
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&supplied); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid scene json: %w", err))
		return
	}
	if supplied.V == 0 {
		supplied.V = scene.ContractVersion
	}
	if supplied.V != scene.ContractVersion {
		writeError(w, http.StatusBadRequest,
			fmt.Errorf("scene contract v%d, want v%d", supplied.V, scene.ContractVersion))
		return
	}
	if supplied.Effect == "" {
		writeError(w, http.StatusBadRequest, errors.New("scene must name an effect"))
		return
	}
	if len(supplied.Palette) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("scene must carry at least one color"))
		return
	}

	stand := s.stand(r)
	if err := s.publish(stand, supplied); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, publishResult{
		Stand: stand,
		Topic: broker.SceneTopic(s.opts.TopicPrefix, stand),
		Scene: supplied,
	})
}

// handleLastScene reports what this process last published, which answers
// "why is the stand that color" without a broker subscription.
func (s *Server) handleLastScene(w http.ResponseWriter, r *http.Request) {
	stand := s.stand(r)

	s.mu.RLock()
	last, ok := s.last[stand]
	s.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no scene published for stand %q", stand))
		return
	}
	writeJSON(w, http.StatusOK, publishResult{
		Stand: stand,
		Topic: broker.SceneTopic(s.opts.TopicPrefix, stand),
		Scene: last,
	})
}

func (s *Server) publish(stand string, sc scene.Scene) error {
	payload, err := json.Marshal(sc)
	if err != nil {
		return fmt.Errorf("encode scene: %w", err)
	}
	if err := s.opts.Publisher.PublishScene(stand, payload); err != nil {
		return err
	}

	s.mu.Lock()
	s.last[stand] = sc
	// Whatever this was, it is the last thing the stand was sent. ClearCover
	// says so itself when it was the idle scene.
	if stand == s.opts.DefaultStand {
		s.idle = false
	}
	s.mu.Unlock()
	return nil
}

func (s *Server) stand(r *http.Request) string {
	if stand := r.URL.Query().Get("stand"); stand != "" {
		return stand
	}
	return s.opts.DefaultStand
}

// readImage accepts either a multipart `image` field or a raw body. Multipart
// is what a person types into curl; raw is what a service sends.
func readImage(r *http.Request) ([]byte, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			return nil, fmt.Errorf("invalid multipart body: %w", err)
		}
		file, _, err := r.FormFile("image")
		if err != nil {
			return nil, fmt.Errorf("multipart body has no `image` field: %w", err)
		}
		defer file.Close()
		return readLimited(file)
	}
	return readLimited(r.Body)
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxUpload))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("no image supplied")
	}
	return data, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
