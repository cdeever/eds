// Package server is lightd's HTTP surface.
//
// POST /v1/cover is the interface the album cover resolver will eventually
// call. Building it now rather than a throwaway means nothing here is
// discarded when the resolver arrives - it just gains a second caller.
package server

import (
	"context"
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
	Logger       *slog.Logger
}

// Server publishes scenes in response to HTTP requests.
type Server struct {
	opts Options

	mu   sync.RWMutex
	last map[string]scene.Scene
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

// handleCover is the whole vertical in one request: image in, lights change.
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

	extracted, err := s.opts.Extractor.Extract(r.Context(), image, swatches)
	if err != nil {
		// An image the extractor rejected is the caller's mistake; an
		// extractor that is down is not. Reporting both as 502 would send
		// someone hunting a healthy service over a corrupt JPEG.
		var upstream *palette.StatusError
		if errors.As(err, &upstream) && upstream.ClientFault() {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}

	opts := s.opts.Scene
	if effect := r.URL.Query().Get("effect"); effect != "" {
		opts.Effect = effect
	}

	built, err := scene.Build(extracted.Swatches, opts)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	stand := s.stand(r)
	if err := s.publish(stand, built); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	s.opts.Logger.Info("published scene",
		"stand", stand,
		"effect", built.Effect,
		"colors", len(built.Palette),
		"background_dropped", extracted.BackgroundDropped,
		"fallback", extracted.Fallback,
	)

	writeJSON(w, http.StatusOK, publishResult{
		Stand: stand,
		Topic: broker.SceneTopic(s.opts.TopicPrefix, stand),
		Scene: built,
		Palette: &diagnostics{
			SwatchesExtracted: len(extracted.Swatches),
			BackgroundDropped: extracted.BackgroundDropped,
			Fallback:          extracted.Fallback,
			Source:            extracted.Source,
		},
	})
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
		writeError(w, http.StatusBadRequest, errors.New("scene must carry at least one colour"))
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
// "why is the stand that colour" without a broker subscription.
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
