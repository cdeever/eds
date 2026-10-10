package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cdeever/eds/services/lightd/internal/palette"
	"github.com/cdeever/eds/services/lightd/internal/scene"
)

type fakeExtractor struct {
	resp *palette.Response
	err  error

	mu       sync.Mutex
	gotImage []byte
	gotN     int
}

func (f *fakeExtractor) Extract(_ context.Context, image []byte, n int) (*palette.Response, error) {
	f.mu.Lock()
	f.gotImage, f.gotN = image, n
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type fakePublisher struct {
	mu       sync.Mutex
	err      error
	stand    string
	payloads [][]byte
}

func (f *fakePublisher) PublishScene(standID string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.stand = standID
	f.payloads = append(f.payloads, payload)
	return nil
}

func (f *fakePublisher) Close() {}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.payloads)
}

func goodPalette() *palette.Response {
	return &palette.Response{
		V: 1,
		Swatches: []palette.Swatch{
			{OKLab: [3]float64{0.625, 0.147, 0.118}, Weight: 0.6, Chroma: 0.188, Lightness: 0.625},
			{OKLab: [3]float64{0.449, -0.036, -0.116}, Weight: 0.4, Chroma: 0.122, Lightness: 0.449},
		},
		BackgroundDropped: 0.88,
		Source:            palette.Source{Width: 600, Height: 600, Format: "jpeg"},
	}
}

func newTestServer(t *testing.T, ex Extractor, pub *fakePublisher) http.Handler {
	t.Helper()
	return newServer(t, ex, pub).Routes()
}

func newServer(t *testing.T, ex Extractor, pub *fakePublisher) *Server {
	t.Helper()
	return New(Options{
		Extractor:    ex,
		Publisher:    pub,
		TopicPrefix:  "eds",
		DefaultStand: "lp-stand-01",
		Swatches:     6,
		Scene:        scene.DefaultOptions(),
	})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCoverRawBodyPublishesRetainedScene(t *testing.T) {
	ex := &fakeExtractor{resp: goodPalette()}
	pub := &fakePublisher{}

	req := httptest.NewRequest(http.MethodPost, "/v1/cover", bytes.NewReader([]byte("JPEGBYTES")))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, ex, pub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if pub.count() != 1 {
		t.Fatalf("published %d scenes, want 1", pub.count())
	}
	if pub.stand != "lp-stand-01" {
		t.Errorf("stand = %q", pub.stand)
	}
	if string(ex.gotImage) != "JPEGBYTES" {
		t.Errorf("extractor received %q", ex.gotImage)
	}

	var body publishResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Topic != "eds/lightstand/lp-stand-01/scene" {
		t.Errorf("topic = %q", body.Topic)
	}
	if body.Scene.Effect != "breathe" || len(body.Scene.Palette) != 2 {
		t.Errorf("unexpected scene: %+v", body.Scene)
	}
	// Diagnostics are what make a surprising result debuggable from one curl.
	if body.Palette == nil || body.Palette.BackgroundDropped != 0.88 {
		t.Errorf("diagnostics missing or wrong: %+v", body.Palette)
	}
}

func TestCoverMultipartUpload(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("image", "jacket.jpg")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("JPEGBYTES"))
	writer.Close()

	pub := &fakePublisher{}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, pub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if pub.count() != 1 {
		t.Error("multipart upload did not publish")
	}
}

func TestCoverHonoursStandAndEffectAndN(t *testing.T) {
	ex := &fakeExtractor{resp: goodPalette()}
	pub := &fakePublisher{}

	req := httptest.NewRequest(http.MethodPost,
		"/v1/cover?stand=lp-stand-02&effect=sweep&n=3", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, ex, pub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if pub.stand != "lp-stand-02" {
		t.Errorf("stand = %q", pub.stand)
	}
	if ex.gotN != 3 {
		t.Errorf("n = %d, want 3", ex.gotN)
	}

	var body publishResult
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Scene.Effect != "sweep" {
		t.Errorf("effect = %q", body.Scene.Effect)
	}
}

func TestCoverRejectsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader(""))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCoverRejectsBadSwatchCount(t *testing.T) {
	for _, n := range []string{"0", "99", "abc"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/cover?n="+n, strings.NewReader("IMG"))
		req.Header.Set("Content-Type", "image/jpeg")
		rec := httptest.NewRecorder()
		newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("n=%s: status = %d, want 400", n, rec.Code)
		}
	}
}

// A palette service that is down is not the caller's mistake.
func TestCoverReportsPaletteOutageAsBadGateway(t *testing.T) {
	ex := &fakeExtractor{err: errors.New("connection refused")}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, ex, &fakePublisher{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

// But a corrupt JPEG is. Reporting it as 502 would send someone hunting a
// healthy service over a bad upload.
func TestCoverReportsRejectedImageAsBadRequest(t *testing.T) {
	ex := &fakeExtractor{err: &palette.StatusError{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       `{"detail":"could not decode image"}`,
	}}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("NOTANIMAGE"))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, ex, &fakePublisher{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "could not decode image") {
		t.Errorf("upstream detail lost: %s", rec.Body)
	}
}

// An upstream 5xx is still an outage, whatever shape it arrives in.
func TestCoverReportsUpstreamServerErrorAsBadGateway(t *testing.T) {
	ex := &fakeExtractor{err: &palette.StatusError{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
	}}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, ex, &fakePublisher{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestCoverReportsPublishFailure(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker unreachable")}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, pub).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

// Publishing a scene by hand is how the stand gets exercised before any image
// path exists.
func TestSceneEndpointPublishesDirectly(t *testing.T) {
	pub := &fakePublisher{}
	body := `{"effect":"solid","speed":0,"brightness":1,"palette":[{"rgb":[255,0,0],"weight":1}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/scene", strings.NewReader(body))
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, pub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if pub.count() != 1 {
		t.Fatal("scene was not published")
	}

	var published scene.Scene
	if err := json.Unmarshal(pub.payloads[0], &published); err != nil {
		t.Fatal(err)
	}
	if published.V != scene.ContractVersion {
		t.Errorf("v = %d, want %d stamped by default", published.V, scene.ContractVersion)
	}
	if published.Effect != "solid" {
		t.Errorf("effect = %q", published.Effect)
	}
}

func TestSceneEndpointValidates(t *testing.T) {
	cases := map[string]string{
		"not json":       `{`,
		"no effect":      `{"palette":[{"rgb":[1,2,3],"weight":1}]}`,
		"empty palette":  `{"effect":"solid","palette":[]}`,
		"wrong contract": `{"v":99,"effect":"solid","palette":[{"rgb":[1,2,3],"weight":1}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/scene", strings.NewReader(body))
			rec := httptest.NewRecorder()
			newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{}).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestLastSceneRoundTrip(t *testing.T) {
	handler := newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scene", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("before publishing, status = %d, want 404", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scene", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("after publishing, status = %d", rec.Code)
	}

	var body publishResult
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Scene.Effect != "breathe" {
		t.Errorf("last scene = %+v", body.Scene)
	}
}

func TestLastSceneIsPerStand(t *testing.T) {
	handler := newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{})

	req := httptest.NewRequest(http.MethodPost, "/v1/cover?stand=a&effect=solid", strings.NewReader("IMG"))
	req.Header.Set("Content-Type", "image/jpeg")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scene?stand=b", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("stand b status = %d, want 404 - stands must not share state", rec.Code)
	}
}

func TestConcurrentPublishesAreSafe(t *testing.T) {
	handler := newTestServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{})

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			url := fmt.Sprintf("/v1/cover?stand=stand-%d", i%3)
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader("IMG"))
			req.Header.Set("Content-Type", "image/jpeg")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()
}

// --- the cover topic ---------------------------------------------------------

// A cover from the topic and the same cover over HTTP must light the room the
// same way. That they share one path is the design; this is what holds it to
// that.
func TestApplyCoverPublishesWhatHTTPWould(t *testing.T) {
	viaHTTP := &fakePublisher{}
	req := httptest.NewRequest(http.MethodPost, "/v1/cover", strings.NewReader("COVER"))
	rec := httptest.NewRecorder()
	newTestServer(t, &fakeExtractor{resp: goodPalette()}, viaHTTP).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("http status = %d", rec.Code)
	}

	ex := &fakeExtractor{resp: goodPalette()}
	viaTopic := &fakePublisher{}
	if err := newServer(t, ex, viaTopic).ApplyCover(context.Background(), []byte("COVER")); err != nil {
		t.Fatal(err)
	}

	if viaTopic.count() != 1 {
		t.Fatalf("published %d scenes, want 1", viaTopic.count())
	}
	if viaTopic.stand != "lp-stand-01" {
		t.Errorf("stand = %q, want the default", viaTopic.stand)
	}
	if string(ex.gotImage) != "COVER" || ex.gotN != 6 {
		t.Errorf("extractor received %q with n=%d", ex.gotImage, ex.gotN)
	}
	if string(viaTopic.payloads[0]) != string(viaHTTP.payloads[0]) {
		t.Errorf("topic scene %s differs from http scene %s", viaTopic.payloads[0], viaHTTP.payloads[0])
	}
}

// What the topic delivered is what GET /v1/scene reports, so "why is the
// stand that colour" has one answer whichever input set it.
func TestApplyCoverIsVisibleAsLastScene(t *testing.T) {
	srv := newServer(t, &fakeExtractor{resp: goodPalette()}, &fakePublisher{})
	if err := srv.ApplyCover(context.Background(), []byte("COVER")); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scene", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
}

// A cover that cannot be used must leave the room alone. Nothing is published,
// so the stand keeps the last good scene.
func TestApplyCoverIgnoresWhatItCannotUse(t *testing.T) {
	cases := map[string]struct {
		image []byte
		ex    *fakeExtractor
		pub   *fakePublisher
	}{
		"empty": {
			image: nil,
			ex:    &fakeExtractor{resp: goodPalette()},
			pub:   &fakePublisher{},
		},
		"not an image": {
			image: []byte("NOTANIMAGE"),
			ex: &fakeExtractor{err: &palette.StatusError{
				StatusCode: http.StatusBadRequest, Status: "400 Bad Request",
			}},
			pub: &fakePublisher{},
		},
		"palette down": {
			image: []byte("COVER"),
			ex:    &fakeExtractor{err: errors.New("connection refused")},
			pub:   &fakePublisher{},
		},
		"larger than an upload may be": {
			image: make([]byte, maxUpload+1),
			ex:    &fakeExtractor{resp: goodPalette()},
			pub:   &fakePublisher{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := newServer(t, tc.ex, tc.pub).ApplyCover(context.Background(), tc.image)
			if err == nil {
				t.Error("expected an error")
			}
			if tc.pub.count() != 0 {
				t.Errorf("published %d scenes for a cover it could not use", tc.pub.count())
			}
		})
	}
}

func TestApplyCoverReportsPublishFailure(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker: timed out")}
	err := newServer(t, &fakeExtractor{resp: goodPalette()}, pub).ApplyCover(context.Background(), []byte("COVER"))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want the publish failure", err)
	}
}
