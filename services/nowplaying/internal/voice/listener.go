package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// UtteranceFilter is every device's requests: a WAV, never retained
// (ADR-0042).
func UtteranceFilter(prefix string) string { return prefix + "/voice/+/utterance" }

// HeardTopic is where a device is told what was made of its request.
func HeardTopic(prefix, device string) string { return prefix + "/voice/" + device + "/heard" }

// StatusTopic is voiced's own presence.
func StatusTopic(prefix string) string { return prefix + "/voiced/status" }

// Heard is what was made of a request, said back to the device that sent it.
// Understood is false when nothing was asked for, and Reason says why.
type Heard struct {
	V          int    `json:"v"`
	Text       string `json:"text"`
	Understood bool   `json:"understood"`
	Artist     string `json:"artist,omitempty"`
	Album      string `json:"album,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Transcriber writes a recording down.
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte) (string, error)
}

// Counter knows an artist's albums in order (ADR-0041).
type Counter interface {
	Discography(ctx context.Context, artist string) (resolve.Discography, error)
}

// Listener makes each request heard into a command.
type Listener struct {
	Prefix  string
	Scribe  Transcriber
	Albums  Counter
	Publish func(topic string, payload []byte, retained bool) error
	Logger  *slog.Logger

	// Timeout bounds one request, from its words to its album: a request
	// answered a minute late is not what anyone meant. Zero is 30 seconds.
	Timeout time.Duration
}

// Handle takes one message from the utterance filter.
func (l *Listener) Handle(msg bus.Message) {
	device, ok := deviceOf(l.Prefix, msg.Topic)
	if !ok {
		return
	}
	if msg.Retained {
		// Someone else's mistake, and played again on every reconnect.
		l.logger().Warn("dropped a retained request; a request is never retained", "device", device)
		return
	}
	timeout := l.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	heard := l.hear(ctx, device, msg.Payload)
	payload, _ := json.Marshal(heard)
	if err := l.Publish(HeardTopic(l.Prefix, device), payload, false); err != nil {
		l.logger().Warn("could not say what was heard", "device", device, "error", err)
	}
}

func (l *Listener) hear(ctx context.Context, device string, wav []byte) Heard {
	started := time.Now()
	text, err := l.Scribe.Transcribe(ctx, wav)
	if err != nil {
		l.logger().Warn("a request could not be written down", "device", device, "bytes", len(wav), "error", err)
		return Heard{V: contract.Version, Reason: "it could not be written down"}
	}
	heard := Heard{V: contract.Version, Text: text}
	log := l.logger().With("device", device, "text", text, "written_ms", time.Since(started).Milliseconds())

	r, err := Parse(text)
	if err != nil {
		log.Info("not a request")
		heard.Reason = "not a request for an album"
		return heard
	}
	artist, album, err := Album(ctx, l.Albums, r)
	if err != nil {
		log.Info("no album for the request", "artist", r.Artist, "nth", r.Nth, "error", err)
		heard.Reason = "no such album: " + reason(err)
		return heard
	}

	c, err := contract.EncodeCommand(contract.Command{Command: contract.PlayAlbum, From: "voiced", Artist: artist, Album: album})
	if err == nil {
		err = l.Publish(contract.CommandTopic(l.Prefix), c, false)
	}
	if err != nil {
		log.Warn("could not ask for the album", "artist", artist, "album", album, "error", err)
		heard.Reason = "the album could not be asked for"
		return heard
	}
	log.Info("asked for an album", "artist", artist, "album", album, "total_ms", time.Since(started).Milliseconds())
	heard.Understood, heard.Artist, heard.Album = true, artist, album
	return heard
}

// Album names the album a request asks for: as said, or counted among the
// artist's studio albums. An artist heard without an apostrophe ("Van
// Halens") is asked for again without its s when no artist has the name.
func Album(ctx context.Context, albums Counter, r Request) (artist, album string, err error) {
	if r.Album != "" {
		return r.Artist, r.Album, nil
	}
	d, err := albums.Discography(ctx, r.Artist)
	if errors.Is(err, resolve.ErrNotFound) {
		if bare, ok := strings.CutSuffix(r.Artist, "s"); ok && strings.TrimSpace(bare) != "" {
			d, err = albums.Discography(ctx, bare)
		}
	}
	if err != nil {
		return "", "", err
	}
	a, err := d.Nth(r.Nth)
	if err != nil {
		return "", "", err
	}
	return d.Artist, a.Title, nil
}

func reason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && errors.Is(err, resolve.ErrNotFound) {
		return msg[i+2:]
	}
	return msg
}

func deviceOf(prefix, topic string) (string, bool) {
	rest, ok := strings.CutPrefix(topic, prefix+"/voice/")
	if !ok {
		return "", false
	}
	device, ok := strings.CutSuffix(rest, "/utterance")
	if !ok || device == "" || strings.Contains(device, "/") {
		return "", false
	}
	return device, true
}

func (l *Listener) logger() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return slog.Default()
}

// Scribe asks scribe to write a recording down (ADR-0042).
type Scribe struct {
	URL  string // http://127.0.0.1:8732
	HTTP *http.Client
}

// Transcribe sends a WAV and returns its words.
func (s Scribe) Transcribe(ctx context.Context, wav []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.URL, "/")+"/v1/transcribe", bytes.NewReader(wav))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scribe answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var t struct {
		V    int    `json:"v"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return "", fmt.Errorf("scribe: %w", err)
	}
	if t.V != 1 {
		return "", fmt.Errorf("scribe speaks version %d, this program 1", t.V)
	}
	return t.Text, nil
}
