package turntable

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// ClipSeconds is how much audio a recogniser is given. Twelve seconds is what
// the recogniser in use is built around: less is padded, more is ignored.
const ClipSeconds = 12

// DefaultSilenceDB is quieter than the quietest music tried on the bench
// (-46 dBFS, a piano alone) and louder than an input with nothing playing.
const DefaultSilenceDB = -55.0

// missesBeforeUnknown is how many clips in a row may go unnamed before the
// track is let go. One miss is a quiet passage. Several are a different track
// the recogniser does not know, or the needle in the run-out groove.
const missesBeforeUnknown = 4

// forgetAfter is how much silence ends an album. Turning a record over is
// silence too, and must not: so it is long.
const forgetAfter = 10 * time.Minute

// follower turns what is heard, and what the recogniser says about it, into
// what is playing. It has no clock but the audio and does nothing but decide,
// which is what lets a test put it through a whole side in no time.
type follower struct {
	clip      int // blocks in a clip
	interval  int // blocks between clips
	stopAfter int // blocks of silence that mean stopped
	forget    int // blocks of silence that end an album
	silence   float64

	ring  []int16 // the last clip's worth of audio
	loud  []bool  // which of the ring's blocks were music
	quiet int     // blocks of silence in a row
	since int     // blocks since a clip was last sent
	busy  bool    // a clip is with the recogniser

	hearing  bool // music is playing
	answered bool // the recogniser has answered since it started
	track    *Match
	pending  *Match // a different track, heard once
	misses   int

	// album is kept from the first track named on a side and given to the
	// tracks after it. The recogniser names an album per track, and for two
	// tracks of one record it has named two: the album, and a box set the
	// album is in. Left alone, that is a new cover between tracks.
	album, albumArtist string
}

func newFollower(interval, stopAfter time.Duration, silenceDB float64) *follower {
	blocks := func(d time.Duration) int { return int(d / (time.Second / 10)) }
	return &follower{
		clip:      ClipSeconds * 10,
		interval:  blocks(interval),
		stopAfter: blocks(stopAfter),
		forget:    blocks(forgetAfter),
		silence:   silenceDB,
		quiet:     blocks(stopAfter), // nothing has been heard yet
	}
}

// hear takes the next tenth of a second. It returns a clip when it is time
// for the recogniser to be asked, and nil otherwise.
func (f *follower) hear(block []int16) []int16 {
	music := Level(block) >= f.silence
	if music {
		if !f.hearing {
			// The first sound after silence. What is in the ring is the
			// silence, and a clip should be all music.
			f.hearing = true
			f.ring, f.loud = f.ring[:0], f.loud[:0]
			f.since = f.interval
		}
		f.quiet = 0
	} else if f.quiet < f.forget {
		f.quiet++
	}

	if f.quiet >= f.stopAfter {
		f.hearing, f.answered = false, false
		f.track, f.pending, f.misses = nil, nil, 0
		if f.quiet >= f.forget {
			f.album, f.albumArtist = "", ""
		}
		return nil
	}
	if !f.hearing {
		return nil
	}

	f.ring = append(f.ring, block...)
	f.loud = append(f.loud, music)
	if over := len(f.loud) - f.clip; over > 0 {
		f.ring = f.ring[over*len(block):]
		f.loud = f.loud[over:]
	}
	f.since++

	// A different track heard once is asked about again as soon as there is
	// a clip that shares nothing with the last one; otherwise a change of
	// track would wait out two whole intervals.
	due := f.interval
	if f.pending != nil && f.clip < due {
		due = f.clip
	}
	if f.busy || len(f.loud) < f.clip || f.since < due || !f.mostlyMusic() {
		return nil
	}
	f.busy, f.since = true, 0
	return append([]int16(nil), f.ring...)
}

// mostlyMusic keeps a click on an idle input, or the last note before the
// gap, from being sent off as a clip.
func (f *follower) mostlyMusic() bool {
	n := 0
	for _, l := range f.loud {
		if l {
			n++
		}
	}
	return 2*n >= len(f.loud)
}

// answer takes what the recogniser said about the last clip.
func (f *follower) answer(m Match, err error) {
	f.busy = false
	if !f.hearing {
		// The needle was lifted while it was thinking.
		return
	}
	switch {
	case errors.Is(err, ErrNoMatch):
		f.answered = true
		f.pending = nil
		if f.track != nil {
			if f.misses++; f.misses >= missesBeforeUnknown {
				f.track, f.misses = nil, 0
			}
		}
	case err != nil:
		// Not an answer. Nothing is learned from a recogniser that could
		// not be reached, least of all that the music is unknown.
	default:
		f.answered = true
		f.misses = 0
		switch {
		case f.track == nil:
			f.adopt(m)
		case f.track.same(m):
			f.pending = nil
		case f.pending != nil && f.pending.same(m):
			f.adopt(m)
		default:
			f.pending = &m
		}
	}
}

func (f *follower) adopt(m Match) {
	if f.album != "" && plain(m.Artist) == plain(f.albumArtist) {
		m.Album = f.album
	} else {
		f.album, f.albumArtist = m.Album, m.Artist
	}
	f.track, f.pending = &m, nil
}

// state is what the follower believes. Music is not reported until the
// recogniser has had its say once: a needle that has just come down is
// followed by a track a few seconds later, and "playing, nobody knows what"
// in between would only blank the display.
func (f *follower) state() (playing bool, track *contract.Track) {
	if !f.hearing || !f.answered {
		return false, nil
	}
	if f.track == nil {
		return true, nil
	}
	return true, &contract.Track{Title: f.track.Title, Artist: f.track.Artist, Album: f.track.Album}
}

// Level is how loud a stretch of audio is, in dBFS. The mean is taken out
// first: a cheap input sits a constant distance from zero, and that distance
// is not sound.
func Level(samples []int16) float64 {
	if len(samples) == 0 {
		return math.Inf(-1)
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s)
	}
	mean := sum / float64(len(samples))

	var squares float64
	for _, s := range samples {
		v := float64(s) - mean
		squares += v * v
	}
	rms := math.Sqrt(squares / float64(len(samples)))
	if rms < 1 {
		rms = 1
	}
	return 20 * math.Log10(rms/32768)
}

func plain(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
