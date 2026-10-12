package turntable

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"
)

// ALSA captures with arecord. A program and not a library, because the
// library is C and nothing in this module is (ADR-0007): the agent is built
// on a desk for a Pi, and stays that simple.
type ALSA struct {
	// Device is ALSA's name for the input, e.g. plughw:UA4FX. A plughw
	// device, so that ALSA does the converting: an interface offers the
	// rates and channels it likes, and they are seldom these.
	Device string
}

func (a ALSA) Open(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, "arecord", "-q",
		"-D", a.Device, "-t", "raw", "-f", "S16_LE", "-c", "1", "-r", strconv.Itoa(Rate))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("arecord could not be started: %w", err)
	}
	return &recording{cmd: cmd, out: out, stderr: &stderr}, nil
}

type recording struct {
	cmd    *exec.Cmd
	out    io.ReadCloser
	stderr *bytes.Buffer
}

// Read is the audio. When arecord stops, the error is what it said on the
// way out, which is the only place "no such device" is to be found.
func (r *recording) Read(p []byte) (int, error) {
	n, err := r.out.Read(p)
	if err == io.EOF && n == 0 {
		if said := lastLine(r.stderr.String()); said != "" {
			return 0, fmt.Errorf("arecord: %s", said)
		}
	}
	return n, err
}

func (r *recording) Close() error {
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	return r.cmd.Wait()
}
