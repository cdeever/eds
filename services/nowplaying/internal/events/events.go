// Package events reports what the agent does to the tenant's log store, the
// way a stand does (ADR-0005, ADR-0017): a few JSON lines on
// <tenant>/log/<device>, which the substrate's bridge files under the device.
//
// It is not the agent's log. The journal has everything; this carries the
// handful of lines worth seeing beside the stand's on a dashboard - a track
// starting, a driver going offline, Spotify refusing.
//
// A line is reported by giving it an "event" attribute where it is logged.
// Nothing else has to know this package exists.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"
)

// Key marks a log line as an event, and names it: "track.playing".
const Key = "event"

// Topic is where a device's events go. The shape is the substrate's, not
// EdS's: it is the one topic under log/ a device's account may publish to.
func Topic(prefix, device string) string { return prefix + "/log/" + device }

// queueSize is how many events wait while the broker is away. Past that the
// oldest claim on attention is the newest event, so new ones are dropped and
// counted, and the next to get through says how many were lost.
const queueSize = 32

// Reporter sends events without ever making their author wait.
type Reporter struct {
	publish func(payload []byte) error
	queue   chan map[string]any
	dropped atomic.Uint32
	started time.Time
	now     func() time.Time
}

// New makes a Reporter that sends each event with publish. Run must be
// called for anything to be sent.
func New(publish func(payload []byte) error) *Reporter {
	return &Reporter{
		publish: publish,
		queue:   make(chan map[string]any, queueSize),
		started: time.Now(),
		now:     time.Now,
	}
}

// Run sends queued events until ctx ends, then makes one last attempt at
// whatever is still waiting, so that "stopping" is not always the line lost.
func (r *Reporter) Run(ctx context.Context) {
	for {
		select {
		case e := <-r.queue:
			r.send(e)
		case <-ctx.Done():
			for {
				select {
				case e := <-r.queue:
					r.send(e)
				default:
					return
				}
			}
		}
	}
}

func (r *Reporter) send(e map[string]any) {
	lost := r.dropped.Swap(0)
	if lost > 0 {
		e["dropped"] = lost
	}
	payload, err := json.Marshal(e)
	if err == nil {
		err = r.publish(payload)
	}
	if err != nil {
		// Not logged: a line about a lost line would be the next one lost.
		r.dropped.Add(lost + 1)
	}
}

func (r *Reporter) enqueue(e map[string]any) {
	select {
	case r.queue <- e:
	default:
		r.dropped.Add(1)
	}
}

// Handler wraps a log handler. Every line still goes to next; a line with an
// "event" attribute is reported as well.
func (r *Reporter) Handler(next slog.Handler) slog.Handler {
	return &handler{reporter: r, next: next}
}

type handler struct {
	reporter *Reporter
	next     slog.Handler
	attrs    []slog.Attr
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	kept := append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &handler{reporter: h.reporter, next: h.next.WithAttrs(attrs), attrs: kept}
}

// Groups are passed on and not otherwise honoured: the agent uses none, and
// an event is a flat line.
func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{reporter: h.reporter, next: h.next.WithGroup(name), attrs: h.attrs}
}

func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	fields := map[string]any{}
	add := func(a slog.Attr) bool {
		fields[a.Key] = plain(a.Value)
		return true
	}
	for _, a := range h.attrs {
		add(a)
	}
	record.Attrs(add)

	if name, ok := fields[Key].(string); ok && name != "" {
		// The store's own names win over an attribute that happens to share
		// one. The store dates each event on arrival; uptime orders the ones
		// that queued.
		fields["level"] = levelName(record.Level)
		fields["msg"] = record.Message
		fields["uptime_ms"] = h.reporter.now().Sub(h.reporter.started).Milliseconds()
		h.reporter.enqueue(fields)
	}
	return h.next.Handle(ctx, record)
}

// plain is a value as the store can file it: a string, a number or a
// boolean. Anything else is written out as text.
func plain(v slog.Value) any {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339)
	}
	if err, ok := v.Any().(error); ok {
		return err.Error()
	}
	return v.String()
}

// levelName is the level as the stand names it, so one filter finds both.
func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	}
	return "info"
}
