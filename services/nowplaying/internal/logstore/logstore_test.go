package logstore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestALineIsPostedTheWayTheStoreKeepsIt(t *testing.T) {
	var (
		path, auth, kind string
		line             map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth, kind = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &line); err != nil {
			t.Errorf("the body is not one JSON line: %q", body)
		}
	}))
	defer server.Close()

	w, err := New(server.URL+"/", "s3cret", "nowplayd", "")
	if err != nil {
		t.Fatal(err)
	}
	err = w.Write(context.Background(), "3 calls to discogs", map[string]any{"event": "api.calls", "calls": 3, "app": "somebody else"})
	if err != nil {
		t.Fatal(err)
	}

	if path != "/insert/jsonline" || auth != "Bearer s3cret" || kind != "application/stream+json" {
		t.Fatalf("posted to %q as %q with %q", path, kind, auth)
	}
	if line["_msg"] != "3 calls to discogs" || line["app"] != "nowplayd" || line["event"] != "api.calls" || line["calls"] != float64(3) {
		t.Fatalf("line = %v", line)
	}
}

func TestARefusalIsAnErrorThatDoesNotNameTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such token", http.StatusUnauthorized)
	}))
	defer server.Close()

	w, _ := New(server.URL, "s3cret", "nowplayd", "")
	err := w.Write(context.Background(), "x", nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want the store's refusal", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("the error names the token: %v", err)
	}

	server.Close()
	if err := w.Write(context.Background(), "x", nil); err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v", err)
	}
}

func TestACertificateAuthorityThatIsNotOne(t *testing.T) {
	if _, err := New("https://logs.example", "t", "nowplayd", "/nonexistent/ca.pem"); err == nil {
		t.Fatal("a missing CA file was accepted")
	}
}
