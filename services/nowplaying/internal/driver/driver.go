// Package driver is what every driver has in common: the shape of what it
// reports, and who it reports to.
//
// A driver knows one player. It reports what the player says and looks
// nothing up; completing a track is nowplayd's job, done once for everyone
// (ADR-0006).
package driver

import "github.com/cdeever/eds/services/nowplaying/internal/contract"

// Update is one thing for the agent to publish.
type Update struct {
	State contract.State
	// Cover is set when the cover changed with this state, and is the image
	// the state's Art names. It is published before the state.
	Cover []byte
}

// Sink receives what a driver sees.
type Sink interface {
	// Online reports whether the driver can reach its player. A driver is
	// online when it has a working session with its player, not merely when
	// it is running.
	Online(online bool)
	Publish(Update)
}
