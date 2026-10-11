package events

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type sent struct {
	mu       sync.Mutex
	payloads [][]byte
	fail     bool
}

func (s *sent) publish(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("broker away")
	}
	s.payloads = append(s.payloads, p)
	return nil
}

func (s *sent) events(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, p := range s.payloads {
		var e map[string]any
		if err := json.Unmarshal(p, &e); err != nil {
			t.Fatalf("not JSON: %s", p)
		}
		out = append(out, e)
	}
	return out
}

// drain runs the reporter over whatever is queued and stops it.
func drain(r *Reporter) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Run(ctx)
}

func TestOnlyLinesMarkedAsEventsAreReported(t *testing.T) {
	s := &sent{}
	r := New(s.publish)
	var journal strings.Builder
	logger := slog.New(r.Handler(slog.NewTextHandler(&journal, nil)))

	logger.Info("connected to broker", "url", "tls://broker:8883")
	logger.Info("published", Key, "track.playing", "artist", "The J. Geils Band", "title", "House Party")
	drain(r)

	got := s.events(t)
	if len(got) != 1 {
		t.Fatalf("reported %d events, want 1", len(got))
	}
	e := got[0]
	if e["event"] != "track.playing" || e["level"] != "info" || e["msg"] != "published" ||
		e["artist"] != "The J. Geils Band" || e["title"] != "House Party" {
		t.Fatalf("got %v", e)
	}
	if _, ok := e["uptime_ms"].(float64); !ok {
		t.Fatalf("no uptime: %v", e)
	}
	// Reporting a line does not take it out of the journal.
	if !strings.Contains(journal.String(), "connected to broker") || !strings.Contains(journal.String(), "House Party") {
		t.Fatalf("the journal lost a line: %s", journal.String())
	}
}

func TestWhatALoggerCarriesGoesWithTheEvent(t *testing.T) {
	s := &sent{}
	r := New(s.publish)
	logger := slog.New(r.Handler(slog.NewTextHandler(io.Discard, nil))).With("driver", "spotify")

	logger.Warn("spotify refuses", Key, "spotify.refused", "error", errors.New("premium required"), "again_in", 5*time.Minute, "failures", 3)
	drain(r)

	e := s.events(t)[0]
	if e["driver"] != "spotify" || e["level"] != "warn" || e["error"] != "premium required" ||
		e["again_in"] != "5m0s" || e["failures"] != float64(3) {
		t.Fatalf("got %v", e)
	}
}

func TestAnAttributeCannotPassItselfOffAsTheLevel(t *testing.T) {
	s := &sent{}
	r := New(s.publish)
	logger := slog.New(r.Handler(slog.NewTextHandler(io.Discard, nil)))
	logger.Error("driver stopped", Key, "driver.stopped", "level", "info", "msg", "all is well")
	drain(r)
	if e := s.events(t)[0]; e["level"] != "error" || e["msg"] != "driver stopped" {
		t.Fatalf("got %v", e)
	}
}

func TestAFullQueueDropsAndNeverBlocks(t *testing.T) {
	s := &sent{}
	r := New(s.publish)
	logger := slog.New(r.Handler(slog.NewTextHandler(io.Discard, nil)))

	done := make(chan struct{})
	go func() {
		for i := 0; i < queueSize+5; i++ {
			logger.Info("published", Key, "track.playing", "n", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("logging blocked with nothing draining the queue")
	}

	drain(r)
	got := s.events(t)
	if len(got) != queueSize {
		t.Fatalf("sent %d, want the %d that fitted", len(got), queueSize)
	}
	// The loss is said once, on the first event to get through.
	if got[0]["dropped"] != float64(5) {
		t.Fatalf("first event says dropped=%v, want 5", got[0]["dropped"])
	}
	if _, again := got[1]["dropped"]; again {
		t.Fatal("the loss was reported twice")
	}
}

func TestAnEventThatCannotBeSentIsCounted(t *testing.T) {
	s := &sent{fail: true}
	r := New(s.publish)
	logger := slog.New(r.Handler(slog.NewTextHandler(io.Discard, nil)))
	logger.Info("one", Key, "a")
	logger.Info("two", Key, "b")
	drain(r)

	s.mu.Lock()
	s.fail = false
	s.mu.Unlock()
	logger.Info("three", Key, "c")
	drain(r)

	got := s.events(t)
	if len(got) != 1 || got[0]["event"] != "c" || got[0]["dropped"] != float64(2) {
		t.Fatalf("got %v", got)
	}
}

func TestTopic(t *testing.T) {
	if got := Topic("eds", "np-relay"); got != "eds/log/np-relay" {
		t.Fatal(got)
	}
}
