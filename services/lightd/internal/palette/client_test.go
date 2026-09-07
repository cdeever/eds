package palette

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validBody = `{
  "v": 1,
  "swatches": [{"rgb":[225,82,26],"hex":"#e1521a","oklab":[0.6264,0.147,0.1184],
                "weight":0.37,"chroma":0.1888,"lightness":0.6264}],
  "background_dropped": 0.88, "fallback": false,
  "source": {"width":600,"height":600,"format":"jpeg"}
}`

func TestExtractParsesTheContract(t *testing.T) {
	var gotPath, gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotContentType = r.Header.Get("Content-Type")
		gotBody = make([]byte, r.ContentLength)
		r.Body.Read(gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(validBody))
	}))
	defer srv.Close()

	got, err := New(srv.URL).Extract(context.Background(), []byte("JPEGBYTES"), 5)
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/v1/palette?n=5" {
		t.Errorf("path = %q", gotPath)
	}
	// Raw body, not multipart: there is no filename worth sending.
	if gotContentType != "application/octet-stream" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if string(gotBody) != "JPEGBYTES" {
		t.Errorf("body = %q", gotBody)
	}
	if len(got.Swatches) != 1 || got.Swatches[0].Chroma != 0.1888 {
		t.Errorf("swatches = %+v", got.Swatches)
	}
	if got.BackgroundDropped != 0.88 || got.Source.Format != "jpeg" {
		t.Errorf("diagnostics lost: %+v", got)
	}
}

// The contract is versioned for a reason; guessing at a changed shape would be
// worse than refusing.
func TestExtractRejectsUnknownContractVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Replace(validBody, `"v": 1`, `"v": 2`, 1)))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Extract(context.Background(), []byte("IMG"), 5)
	if err == nil || !strings.Contains(err.Error(), "contract v2") {
		t.Errorf("err = %v, want a contract version complaint", err)
	}
}

func TestStatusErrorClassifiesFault(t *testing.T) {
	client := &StatusError{StatusCode: 400}
	server := &StatusError{StatusCode: 503}

	if !client.ClientFault() {
		t.Error("400 should be a client fault")
	}
	if server.ClientFault() {
		t.Error("503 should not be a client fault")
	}
}

func TestExtractSurfacesServiceErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"detail":"could not decode image"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Extract(context.Background(), []byte("nope"), 5)
	if err == nil || !strings.Contains(err.Error(), "could not decode image") {
		t.Errorf("err = %v, want the service's own message", err)
	}

	var status *StatusError
	if !errors.As(err, &status) {
		t.Fatalf("err is %T, want *StatusError so callers can classify it", err)
	}
	if !status.ClientFault() {
		t.Error("a 400 from the palette service should read as a client fault")
	}
}

func TestExtractRejectsEmptySwatchList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"v":1,"swatches":[],"background_dropped":0,"fallback":false,
		                  "source":{"width":1,"height":1,"format":"png"}}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Extract(context.Background(), []byte("IMG"), 5); err == nil {
		t.Error("expected an error for an empty palette")
	}
}

func TestExtractRejectsEmptyImage(t *testing.T) {
	if _, err := New("http://127.0.0.1:1").Extract(context.Background(), nil, 5); err == nil {
		t.Error("expected an error for no image")
	}
}

func TestExtractSurfacesTransportFailure(t *testing.T) {
	// Port 1 is reserved and never listening.
	_, err := New("http://127.0.0.1:1").Extract(context.Background(), []byte("IMG"), 5)
	if err == nil {
		t.Error("expected a transport error")
	}
}

func TestExtractHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(validBody))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Extract(ctx, []byte("IMG"), 5); err == nil {
		t.Error("expected cancellation to surface")
	}
}
