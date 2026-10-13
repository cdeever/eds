// Package calls counts what EdS asks of other people's services, so that
// how often it asks can be seen (ADR-0027).
//
// None of these services is EdS's, and one of them was never offered at all.
// Each has a limit, published or not. A Meter keeps a tally for each service
// and hands it over once a minute as one line: how many calls, how many were
// refused for coming too often, how many failed, how long the slowest took.
// A minute with no calls says nothing.
//
// It counts and does nothing else. It does not slow a caller down or stop
// one: each client already keeps to its service's rules, and this is how to
// see that it does.
package calls

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event is the name the summaries are reported under.
const Event = "api.calls"

// Every is how often the tallies are handed over.
const Every = time.Minute

// Outcome is how one call went, as far as the service's patience goes.
type Outcome int

const (
	// OK is an answer, whatever it said. "Not found" is an answer.
	OK Outcome = iota
	// Limited is the service saying it was asked too often.
	Limited
	// Failed is no answer, or one that was the service's failure or a
	// refusal.
	Failed
)

// Summary is what was asked of one service since the last one.
type Summary struct {
	API       string
	Calls     int
	Limited   int
	Failed    int
	SlowestMS int64
	MeanMS    int64
	// Remaining is the least the service said was left of its allowance,
	// for a service that says. Nil for one that does not.
	Remaining *int
}

// Attrs is the summary as the fields of a log line. The numbers are fields
// the log store can add up.
func (s Summary) Attrs() []any {
	attrs := []any{
		"api", s.API,
		"calls", s.Calls,
		"limited", s.Limited,
		"failed", s.Failed,
		"slowest_ms", s.SlowestMS,
		"mean_ms", s.MeanMS,
	}
	if s.Remaining != nil {
		attrs = append(attrs, "remaining", *s.Remaining)
	}
	return attrs
}

type tally struct {
	calls, limited, failed int
	slowest, total         time.Duration
	remaining              *int
}

// Meter keeps the tallies. The zero value is not ready; use New.
type Meter struct {
	mu      sync.Mutex
	tallies map[string]*tally
}

func New() *Meter { return &Meter{tallies: map[string]*tally{}} }

// Record counts one call.
func (m *Meter) Record(api string, outcome Outcome, took time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tally(api)
	t.calls++
	switch outcome {
	case Limited:
		t.limited++
	case Failed:
		t.failed++
	}
	t.total += took
	if took > t.slowest {
		t.slowest = took
	}
}

// Remaining notes what a service said was left of its allowance. The least
// heard in a minute is the one kept: it is how close the minute came.
func (m *Meter) Remaining(api string, left int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tally(api)
	if t.remaining == nil || left < *t.remaining {
		t.remaining = &left
	}
}

func (m *Meter) tally(api string) *tally {
	t := m.tallies[api]
	if t == nil {
		t = &tally{}
		m.tallies[api] = t
	}
	return t
}

// Take hands over the tallies, in order of name, and starts them again.
func (m *Meter) Take() []Summary {
	m.mu.Lock()
	tallies := m.tallies
	m.tallies = map[string]*tally{}
	m.mu.Unlock()

	out := make([]Summary, 0, len(tallies))
	for api, t := range tallies {
		if t.calls == 0 {
			continue
		}
		out = append(out, Summary{
			API:       api,
			Calls:     t.calls,
			Limited:   t.limited,
			Failed:    t.failed,
			SlowestMS: t.slowest.Milliseconds(),
			MeanMS:    (t.total / time.Duration(t.calls)).Milliseconds(),
			Remaining: t.remaining,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].API < out[j].API })
	return out
}

// Run hands the tallies to emit every so often until ctx ends, and once
// more then, so that the last minute is not the one never seen.
func (m *Meter) Run(ctx context.Context, every time.Duration, emit func(Summary)) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			for _, s := range m.Take() {
				emit(s)
			}
			return
		}
		for _, s := range m.Take() {
			emit(s)
		}
	}
}

// Client is an HTTP client whose every request is counted, redirects
// included, under the name Name gives its host.
func (m *Meter) Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &Transport{Meter: m}}
}

// Transport counts the requests that pass through it.
type Transport struct {
	Meter *Meter
	// Base does the asking. Nil is http.DefaultTransport.
	Base http.RoundTripper
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	api := Name(req.URL.Hostname())
	started := time.Now()
	resp, err := base.RoundTrip(req)
	took := time.Since(started)

	if err != nil {
		t.Meter.Record(api, Failed, took)
		return resp, err
	}
	t.Meter.Record(api, outcome(resp.StatusCode), took)
	for _, header := range remainingHeaders {
		if left, err := strconv.Atoi(resp.Header.Get(header)); err == nil {
			t.Meter.Remaining(api, left)
			break
		}
	}
	return resp, nil
}

// remainingHeaders are where a service says what is left of an allowance.
var remainingHeaders = []string{"X-Discogs-Ratelimit-Remaining", "X-RateLimit-Remaining"}

func outcome(status int) Outcome {
	switch {
	case status == http.StatusTooManyRequests:
		return Limited
	case status == http.StatusNotFound:
		// "There is no such record" is an answer, and the commonest one a
		// cover archive gives.
		return OK
	case status >= 400:
		return Failed
	}
	return OK
}

// names says whose a host is. A service's pictures come from other hosts
// than its API does and are counted apart: the allowance is the API's, and
// a count swollen with pictures would say nothing about it.
var names = []struct{ suffix, api string }{
	{"api.discogs.com", "discogs"},
	{"discogs.com", "discogs-images"},
	{"api.deezer.com", "deezer"},
	{"dzcdn.net", "deezer-images"},
	{"itunes.apple.com", "itunes"},
	{"mzstatic.com", "itunes-images"},
	{"musicbrainz.org", "musicbrainz"},
	{"coverartarchive.org", "coverartarchive"},
	{"archive.org", "coverartarchive"}, // where the archive's pictures are kept
	{"spotify.com", "spotify"},
	{"scdn.co", "spotify-images"},
}

// Name is what a host's calls are counted under: the service it belongs to,
// or the host itself when nobody here knows it.
func Name(host string) string {
	host = strings.ToLower(host)
	for _, n := range names {
		if host == n.suffix || strings.HasSuffix(host, "."+n.suffix) {
			return n.api
		}
	}
	return host
}
