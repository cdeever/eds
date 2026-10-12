package turntable

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Match is a track a recognizer named.
type Match struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Album  string `json:"album"`
}

// same compares what tells one track from the next. The album is left out:
// the recognizer does not name it the same way twice.
func (m Match) same(other Match) bool {
	return plain(m.Artist) == plain(other.Artist) && plain(m.Title) == plain(other.Title)
}

// ErrNoMatch is a recognizer that answered, and did not know.
var ErrNoMatch = errors.New("turntable: the recognizer does not know this")

// Recognizer names the track in a clip of mono audio at Rate. It is the seam
// ADR-0021 keeps: the recognizer in use is not one anybody promised would
// last, and its replacement is a second one of these, not a second driver.
type Recognizer interface {
	Recognize(ctx context.Context, clip []int16) (Match, error)
}

// Command is a recognizer that is a program: the clip goes to its standard
// input as a WAV file, and it prints one JSON object - artist, title, album -
// or an empty one when it does not know. Anything else is a failure.
type Command struct {
	Path string
	// Timeout is how long it may take. Default thirty seconds.
	Timeout time.Duration
}

func (c Command) Recognize(ctx context.Context, clip []int16) (Match, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path)
	cmd.Stdin = bytes.NewReader(WAV(clip))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Killed on a timeout, and not waited on for ever if it left a child
	// holding its output open.
	cmd.WaitDelay = time.Second

	if err := cmd.Run(); err != nil {
		if said := lastLine(stderr.String()); said != "" {
			return Match{}, fmt.Errorf("%s: %w: %s", c.Path, err, said)
		}
		return Match{}, fmt.Errorf("%s: %w", c.Path, err)
	}

	var m Match
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &m); err != nil {
		return Match{}, fmt.Errorf("%s: its answer could not be read: %w", c.Path, err)
	}
	m.Artist, m.Title, m.Album = strings.TrimSpace(m.Artist), strings.TrimSpace(m.Title), strings.TrimSpace(m.Album)
	if m.Title == "" {
		return Match{}, ErrNoMatch
	}
	return m, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

// WAV is a clip as a file: mono, 16-bit, at Rate.
func WAV(clip []int16) []byte {
	const header = 44
	data := len(clip) * 2
	out := make([]byte, header+data)
	le := binary.LittleEndian

	copy(out[0:], "RIFF")
	le.PutUint32(out[4:], uint32(header-8+data))
	copy(out[8:], "WAVEfmt ")
	le.PutUint32(out[16:], 16)     // size of the format chunk
	le.PutUint16(out[20:], 1)      // PCM
	le.PutUint16(out[22:], 1)      // channels
	le.PutUint32(out[24:], Rate)   // samples a second
	le.PutUint32(out[28:], Rate*2) // bytes a second
	le.PutUint16(out[32:], 2)      // bytes a sample
	le.PutUint16(out[34:], 16)     // bits a sample
	copy(out[36:], "data")
	le.PutUint32(out[40:], uint32(data))
	for i, s := range clip {
		le.PutUint16(out[header+2*i:], uint16(s))
	}
	return out
}
