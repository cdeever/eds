package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/logstore"
)

// openStore is the tenant's log store, or nil when nowplayd was not given
// the way there, as on a desk.
func openStore(cfg config.Nowplayd) (*logstore.Writer, error) {
	if cfg.LogEndpoint == "" || cfg.LogToken == "" {
		return nil, nil
	}
	return logstore.New(cfg.LogEndpoint, cfg.LogToken, "nowplayd", cfg.Broker.CAFile)
}

// linesQueued is how many lines wait for the store. A side of a record is a
// dozen; a full queue is a store that has stopped answering.
const linesQueued = 64

// lines are the events nowplayd says as it goes (ADR-0050): how a track was
// completed, and the collection being read. Each is logged here and, when
// there is a store, sent there in the order said. Saying one never waits on
// the store: a line that finds the queue full is dropped, and how many were
// is said when the store answers again.
type lines struct {
	logger *slog.Logger
	queue  chan line
}

type line struct {
	msg    string
	fields map[string]any
}

// newLines starts sending to store, until ctx ends. What is still queued
// then is not sent. A nil store sends nothing.
func newLines(ctx context.Context, store *logstore.Writer, logger *slog.Logger) *lines {
	l := &lines{logger: logger}
	if store == nil {
		return l
	}
	l.queue = make(chan line, linesQueued)
	go func() {
		failing := false
		for {
			select {
			case <-ctx.Done():
				return
			case ln := <-l.queue:
				sending, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := store.Write(sending, ln.msg, ln.fields)
				cancel()
				// Said once for a run of failures, and once when it mends.
				switch {
				case err != nil && !failing:
					failing = true
					logger.Warn("events could not be written to the log store", "reason", err)
				case err == nil && failing:
					failing = false
					logger.Info("events are reaching the log store again")
				}
			}
		}
	}()
	return l
}

// say logs a line, and queues it for the store. attrs are key and value
// pairs, an "event" among them.
func (l *lines) say(msg string, attrs ...any) {
	l.logger.Info(msg, attrs...)
	if l.queue == nil {
		return
	}
	fields := map[string]any{"level": "info"}
	for i := 0; i+1 < len(attrs); i += 2 {
		if k, ok := attrs[i].(string); ok {
			fields[k] = attrs[i+1]
		}
	}
	select {
	case l.queue <- line{msg: msg, fields: fields}:
	default:
		l.logger.Warn("an event was dropped: the log store is not keeping up", "event", fields["event"])
	}
}
