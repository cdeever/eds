package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type words string

// A transcript that begins with "?" is one scribe is not sure of.
func (w words) Transcribe(context.Context, []byte) (Words, error) {
	if w == "!" {
		return Words{}, errors.New("scribe is down")
	}
	if text, unsure := strings.CutPrefix(string(w), "?"); unsure {
		return Words{Text: text}, nil
	}
	return Words{Text: string(w), Confident: true}, nil
}

// discographies are MusicBrainz's as ADR-0041 counts them, cut down.
type discographies struct {
	mu    sync.Mutex
	asked []string
}

func (d *discographies) Discography(_ context.Context, artist string) (resolve.Discography, error) {
	d.mu.Lock()
	d.asked = append(d.asked, artist)
	d.mu.Unlock()
	switch resolve.Simplify(artist) {
	case "vanhalen":
		return resolve.Discography{Artist: "Van Halen", Albums: []resolve.StudioAlbum{{Title: "Van Halen"}, {Title: "Van Halen II"}}}, nil
	case "gunsnroses":
		return resolve.Discography{Artist: "Guns N' Roses", Albums: []resolve.StudioAlbum{{Title: "Appetite for Destruction"}}}, nil
	}
	return resolve.Discography{}, fmt.Errorf("%w: MusicBrainz knows no artist %q", resolve.ErrNotFound, artist)
}

type published struct {
	topic, payload string
	retained       bool
}

func listen(t *testing.T, said words) (*Listener, *[]published, *discographies) {
	t.Helper()
	var out []published
	d := &discographies{}
	l := &Listener{Prefix: "eds", Scribe: said, Albums: d, Logger: quiet,
		Publish: func(topic string, payload []byte, retained bool) error {
			out = append(out, published{topic, string(payload), retained})
			return nil
		}}
	return l, &out, d
}

func utterance(device string) bus.Message {
	return bus.Message{Topic: "eds/voice/" + device + "/utterance", Payload: []byte("RIFF....WAVE")}
}

func heard(t *testing.T, p published) Heard {
	t.Helper()
	var h Heard
	if err := json.Unmarshal([]byte(p.payload), &h); err != nil {
		t.Fatalf("%s: %v", p.payload, err)
	}
	return h
}

func TestARequestBecomesACommandAndIsSaidBack(t *testing.T) {
	l, out, _ := listen(t, "Play Van Halen's first album.")
	l.Handle(utterance("respeaker-01"))

	if len(*out) != 3 {
		t.Fatalf("published %+v", *out)
	}
	cmd := (*out)[0]
	if cmd.topic != "eds/nowplaying/command" || cmd.retained {
		t.Errorf("command on %s, retained %v", cmd.topic, cmd.retained)
	}
	c, err := contract.DecodeCommand([]byte(cmd.payload))
	if err != nil || c.Command != contract.PlayAlbum || c.Artist != "Van Halen" || c.Album != "Van Halen" || c.From != "voiced" {
		t.Errorf("command %+v, %v", c, err)
	}
	said := (*out)[1]
	if said.topic != "eds/voice/respeaker-01/heard" || said.retained {
		t.Errorf("heard on %s, retained %v", said.topic, said.retained)
	}
	if h := heard(t, said); !h.Understood || h.Text != "Play Van Halen's first album." || h.Album != "Van Halen" || h.Reason != "" {
		t.Errorf("heard %+v", h)
	}
}

// The artist is MusicBrainz's, as it writes the name.
func TestAnAlbumAskedForByItsPlaceIsNamedAsMusicBrainzNamesIt(t *testing.T) {
	l, out, _ := listen(t, "play the first album by guns n roses")
	l.Handle(utterance("respeaker-01"))
	c, _ := contract.DecodeCommand([]byte((*out)[0].payload))
	if c.Artist != "Guns N' Roses" || c.Album != "Appetite for Destruction" {
		t.Errorf("command %+v", c)
	}
}

func TestAnAlbumAskedForByNameIsAskedForAsSaid(t *testing.T) {
	l, out, d := listen(t, "Play Abbey Road by the Beatles.")
	l.Handle(utterance("respeaker-01"))
	c, _ := contract.DecodeCommand([]byte((*out)[0].payload))
	if c.Artist != "the Beatles" || c.Album != "Abbey Road" {
		t.Errorf("command %+v", c)
	}
	if len(d.asked) != 0 {
		t.Errorf("counted albums for a named one: %q", d.asked)
	}
}

func TestAPossessiveWithoutItsApostropheIsAskedForWithoutItsS(t *testing.T) {
	l, out, d := listen(t, "Play Van Halens second album")
	l.Handle(utterance("respeaker-01"))
	c, _ := contract.DecodeCommand([]byte((*out)[0].payload))
	if c.Album != "Van Halen II" || fmt.Sprint(d.asked) != "[Van Halens Van Halen]" {
		t.Errorf("command %+v after asking for %q", c, d.asked)
	}
}

// Nothing is asked for when the words are not a request, the artist is
// unknown, the place is past the last album, or scribe cannot be reached;
// each is said back, with why.
func TestWhatIsNotARequestIsSaidBackAndNothingIsAskedFor(t *testing.T) {
	for said, reason := range map[words]string{
		"Thank you.":                     "not a request for an album",
		"Play Arrowsmith's first album.": `no such album: MusicBrainz knows no artist "Arrowsmith"`,
		"Play Van Halen's fifth album.":  "no such album: Van Halen has 2 studio albums, not a 5",
		"!":                              "it could not be written down",
		// The prompt, written back for what could not be made out: a
		// request in words, and nothing anyone said.
		"?Play Abbey Road by the Beatles.": "not heard clearly",
	} {
		l, out, _ := listen(t, said)
		l.Handle(utterance("respeaker-01"))
		if len(*out) != 2 || (*out)[0].topic != "eds/voice/respeaker-01/heard" || !strings.Contains((*out)[1].payload, `"unduck"`) {
			t.Errorf("%q: published %+v", said, *out)
			continue
		}
		if h := heard(t, (*out)[0]); h.Understood || h.Reason != reason {
			t.Errorf("%q: heard %+v; want reason %q", said, h, reason)
		}
	}
}

func TestARetainedRequestOrAnotherTopicIsIgnored(t *testing.T) {
	l, out, _ := listen(t, "Play Van Halen's first album.")
	retained := utterance("respeaker-01")
	retained.Retained = true
	l.Handle(retained)
	l.Handle(bus.Message{Topic: "eds/voice/respeaker-01/heard", Payload: []byte("{}")})
	l.Handle(bus.Message{Topic: "eds/voice/a/b/utterance", Payload: []byte("RIFF")})
	if len(*out) != 0 {
		t.Errorf("published %+v", *out)
	}
}

func TestScribeIsAskedAsItsContractSays(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/transcribe" || r.Header.Get("Content-Type") != "audio/wav" {
			http.Error(w, "wrong", http.StatusBadRequest)
			return
		}
		got, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"v":1,"text":"Play Rocks by Aerosmith.","confident":true,"seconds":1.8,"took_ms":400}`))
	}))
	defer srv.Close()

	heard, err := Scribe{URL: srv.URL + "/"}.Transcribe(context.Background(), []byte("RIFF-wav"))
	if err != nil || heard.Text != "Play Rocks by Aerosmith." || !heard.Confident || string(got) != "RIFF-wav" {
		t.Errorf("heard %+v, %v; sent %q", heard, err, got)
	}
}

// A scribe that does not say it is sure, or says it is not, is not.
func TestWordsScribeIsNotSureOfAreNotConfident(t *testing.T) {
	for _, answer := range []string{
		`{"v":1,"text":"Play Abbey Road by the Beatles.","confident":false}`,
		`{"v":1,"text":"Play Abbey Road by the Beatles."}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(answer))
		}))
		heard, err := Scribe{URL: srv.URL}.Transcribe(context.Background(), []byte("x"))
		srv.Close()
		if err != nil || heard.Confident {
			t.Errorf("%s: heard %+v, %v", answer, heard, err)
		}
	}
}

func TestScribeRefusingIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"44100 Hz"}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()
	if _, err := (Scribe{URL: srv.URL}).Transcribe(context.Background(), []byte("x")); err == nil {
		t.Error("a refusal was taken for words")
	}
}

// --- ducking (ADR-0044) --------------------------------------------------------

func TestAWakeTurnsTheMusicDownAndTheAnswerBringsItBack(t *testing.T) {
	l, out, _ := listen(t, "Play Van Halen's first album.")
	l.Handle(bus.Message{Topic: "eds/voice/respeaker-01/woken", Payload: []byte(`{"v":1}`)})
	l.Handle(utterance("respeaker-01"))

	var commands []string
	for _, p := range *out {
		if p.topic == "eds/nowplaying/command" {
			c, err := contract.DecodeCommand([]byte(p.payload))
			if err != nil || c.From != "voiced" || p.retained {
				t.Errorf("command %s: %v", p.payload, err)
			}
			commands = append(commands, c.Command)
		}
	}
	if fmt.Sprint(commands) != "[duck play_album unduck]" {
		t.Errorf("commands %v", commands)
	}
}

func TestARetainedWakeDucksNothing(t *testing.T) {
	l, out, _ := listen(t, "")
	l.Handle(bus.Message{Topic: "eds/voice/respeaker-01/woken", Payload: []byte(`{"v":1}`), Retained: true})
	if len(*out) != 0 {
		t.Errorf("published %+v", *out)
	}
}
