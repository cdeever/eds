package turntable

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

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

// line is a capture a test plays into. Each Open is a new cable; opens says
// how many times the driver has come looking for one.
type line struct {
	mu    sync.Mutex
	next  []io.ReadCloser
	opens int
}

func (l *line) Open(context.Context) (io.ReadCloser, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.opens++
	if len(l.next) == 0 {
		return nil, errors.New("no such device")
	}
	stream := l.next[0]
	l.next = l.next[1:]
	return stream, nil
}

func (l *line) plug() *io.PipeWriter {
	r, w := io.Pipe()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next = append(l.next, r)
	return w
}

func play(t *testing.T, w io.Writer, sound func() []int16, seconds int) {
	t.Helper()
	raw := make([]byte, blockSamples*2)
	for i, s := range sound() {
		binary.LittleEndian.PutUint16(raw[2*i:], uint16(s))
	}
	for i := 0; i < seconds*second; i++ {
		if _, err := w.Write(raw); err != nil {
			t.Fatalf("the driver stopped listening: %v", err)
		}
	}
}

type knows struct{ m Match }

func (k knows) Recognize(context.Context, []int16) (Match, error) { return k.m, nil }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never happened: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestARecordPlayedAndTheCablePulled(t *testing.T) {
	cable := &line{}
	first := cable.plug()
	out := &sink{}
	retry := make(chan time.Time)
	at := time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC)

	d := &Driver{
		Source:     "turntable",
		Capture:    cable,
		Recognizer: knows{intro},
		Sink:       out,
		Logger:     quiet,
		Now:        func() time.Time { return at },
		after:      func(time.Duration) <-chan time.Time { return retry },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Listening is being online, and the first thing said is that nothing
	// is playing: whatever the broker kept from last time is not known to
	// be true any more.
	play(t, first, hush, 1)
	eventually(t, "stopped is reported", func() bool { return len(out.states()) == 1 })
	if got := out.states()[0]; got.State != contract.Stopped || got.Source != "turntable" || got.Player != "turntable" {
		t.Fatalf("got %+v", got)
	}

	play(t, first, tone, ClipSeconds+1)
	eventually(t, "the track is reported", func() bool { return len(out.states()) == 2 })
	got := out.states()[1]
	if got.State != contract.Playing || got.Track == nil || got.Track.Title != "Introduction" ||
		got.Track.Artist != "Chicago" || got.Art != nil || !got.ChangedAt.Equal(at) {
		t.Fatalf("got %+v track %+v", got, got.Track)
	}

	// The cable pulled: offline, and the driver goes on looking for it.
	first.Close()
	eventually(t, "offline is reported", func() bool {
		p := out.presence()
		return len(p) == 2 && p[0] && !p[1]
	})

	// Put back. The track is not still playing just because it was when the
	// cable came out.
	second := cable.plug()
	retry <- time.Now()
	play(t, second, hush, 1)
	eventually(t, "online and stopped again", func() bool {
		s := out.states()
		return len(s) == 3 && s[2].State == contract.Stopped && len(out.presence()) == 3
	})

	cancel()
	second.Close()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}

func TestAMissingDeviceIsSaidOnce(t *testing.T) {
	cable := &line{}
	out := &sink{}
	retry := make(chan time.Time)

	d := &Driver{
		Source: "turntable", Capture: cable, Recognizer: knows{intro}, Sink: out, Logger: quiet,
		after: func(time.Duration) <-chan time.Time { return retry },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	for i := 0; i < 5; i++ {
		retry <- time.Now()
	}
	cancel()
	<-done

	cable.mu.Lock()
	opens := cable.opens
	cable.mu.Unlock()
	if opens < 5 {
		t.Fatalf("looked for the device %d times", opens)
	}
	// Once when it was first missed, and once on the way out.
	if got := out.presence(); len(got) != 2 || got[0] || got[1] {
		t.Fatalf("presence %v", got)
	}
}

func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recognize")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheRecognizerIsAProgram(t *testing.T) {
	clip := make([]int16, Rate)
	for i := range clip {
		clip[i] = int16(i)
	}
	ctx := context.Background()

	// It is handed the clip as a WAV file, whole.
	saved := filepath.Join(t.TempDir(), "clip.wav")
	path := script(t, `cat > "`+saved+`"; echo '{"artist": " Chicago ", "title": "Introduction", "album": "CTA", "isrc": "X"}'`)
	m, err := Command{Path: path}.Recognize(ctx, clip)
	if err != nil {
		t.Fatal(err)
	}
	if m != (Match{Artist: "Chicago", Title: "Introduction", Album: "CTA"}) {
		t.Fatalf("got %+v", m)
	}
	wav, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if len(wav) != 44+2*len(clip) || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " ||
		le.Uint32(wav[4:]) != uint32(len(wav)-8) || le.Uint16(wav[22:]) != 1 || le.Uint32(wav[24:]) != Rate ||
		le.Uint16(wav[34:]) != 16 || le.Uint32(wav[40:]) != uint32(2*len(clip)) ||
		int16(le.Uint16(wav[44+2*1000:])) != 1000 {
		t.Fatalf("not the clip as a WAV file: %d bytes, header % x", len(wav), wav[:44])
	}

	// Not knowing is an answer.
	if _, err := (Command{Path: script(t, `cat >/dev/null; echo '{}'`)}).Recognize(ctx, clip); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want ErrNoMatch", err)
	}

	// Failing is not, and says why.
	_, err = Command{Path: script(t, `cat >/dev/null; echo "no route to host" >&2; exit 3`)}.Recognize(ctx, clip)
	if err == nil || errors.Is(err, ErrNoMatch) || !strings.Contains(err.Error(), "no route to host") {
		t.Fatalf("got %v", err)
	}
	if _, err := (Command{Path: script(t, `cat >/dev/null; echo 'Traceback'`)}).Recognize(ctx, clip); err == nil || errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v for an answer that is not JSON", err)
	}

	// Nor is taking for ever.
	started := time.Now()
	_, err = Command{Path: script(t, `sleep 30`), Timeout: 50 * time.Millisecond}.Recognize(ctx, clip)
	if err == nil || time.Since(started) > 10*time.Second {
		t.Fatalf("got %v after %s", err, time.Since(started))
	}
}
