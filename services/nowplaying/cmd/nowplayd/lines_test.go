package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/logstore"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// An event said is sent to the store as one line, its attributes as fields.
func TestALineSaidReachesTheStore(t *testing.T) {
	got := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var line map[string]any
		if err := json.NewDecoder(r.Body).Decode(&line); err != nil {
			t.Error(err)
		}
		got <- line
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &logstore.Writer{Endpoint: server.URL, Token: "t", App: "nowplayd", HTTP: server.Client()}
	said := newLines(ctx, store, quiet)
	said.say("the collection was listed", "event", "collection.listed", "records", 412)

	select {
	case line := <-got:
		if line["event"] != "collection.listed" || line["records"] != float64(412) || line["_msg"] != "the collection was listed" || line["app"] != "nowplayd" {
			t.Errorf("%v", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing reached the store")
	}
}

// A store that does not answer never holds up whoever says a line.
func TestAStoreThatHangsHoldsNobodyUp(t *testing.T) {
	hang := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-hang }))
	defer server.Close()
	defer close(hang)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	said := newLines(ctx, &logstore.Writer{Endpoint: server.URL, HTTP: server.Client()}, quiet)
	done := make(chan struct{})
	go func() {
		for range linesQueued * 2 {
			said.say("a line", "event", "cover.lookup")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("saying a line waited on the store")
	}
}

// Without a store, a line is only logged.
func TestWithoutAStoreALineIsOnlyLogged(t *testing.T) {
	newLines(context.Background(), nil, quiet).say("a line", "event", "cover.lookup")
}
