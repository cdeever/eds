package turntable

import (
	"errors"
	"math"
	"testing"
	"time"
)

// A second of audio is ten blocks; these keep the tests in seconds.
const second = 10

func tone() []int16 {
	block := make([]int16, blockSamples)
	for i := range block {
		block[i] = int16(3000 * math.Sin(float64(i)/8))
	}
	return block
}

func hush() []int16 { return make([]int16, blockSamples) }

// deck is a follower with a record on it: it plays so much music or silence,
// and answers each clip with whatever the test says the recogniser knows.
type deck struct {
	t     *testing.T
	f     *follower
	knows func() (Match, error)
	clips int
	// sent is how many blocks into the last stretch each clip was asked for.
	sent []int
}

func newDeck(t *testing.T) *deck {
	return &deck{t: t, f: newFollower(20*time.Second, 20*time.Second, DefaultSilenceDB)}
}

func (d *deck) hears(m Match) { d.knows = func() (Match, error) { return m, nil } }
func (d *deck) hearsNothing() { d.knows = func() (Match, error) { return Match{}, ErrNoMatch } }
func (d *deck) cannotAsk() {
	d.knows = func() (Match, error) { return Match{}, errors.New("no network") }
}
func (d *deck) music(blocks int) { d.run(tone, blocks) }
func (d *deck) quiet(blocks int) { d.run(hush, blocks) }

func (d *deck) run(sound func() []int16, blocks int) {
	d.sent = nil
	for i := 1; i <= blocks; i++ {
		if clip := d.f.hear(sound()); clip != nil {
			if len(clip) != ClipSeconds*Rate {
				d.t.Fatalf("a clip of %d samples, want %d", len(clip), ClipSeconds*Rate)
			}
			d.clips++
			d.sent = append(d.sent, i)
			d.f.answer(d.knows())
		}
	}
}

func (d *deck) want(playing bool, title, album string) {
	d.t.Helper()
	gotPlaying, track := d.f.state()
	gotTitle, gotAlbum := "", ""
	if track != nil {
		gotTitle, gotAlbum = track.Title, track.Album
	}
	if gotPlaying != playing || gotTitle != title || gotAlbum != album {
		d.t.Fatalf("playing=%v %q [%s], want playing=%v %q [%s]", gotPlaying, gotTitle, gotAlbum, playing, title, album)
	}
}

var (
	intro = Match{Artist: "Chicago", Title: "Introduction", Album: "Original Album Series"}
	time_ = Match{Artist: "Chicago", Title: "Does Anybody Really Know What Time It Is?", Album: "The Chicago Transit Authority"}
	home  = Match{Artist: "Phil Collins", Title: "Take Me Home", Album: "The Singles"}
)

func TestTheNeedleComingDown(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)

	d.quiet(5 * second)
	d.want(false, "", "")

	// Nothing is said until there is a whole clip to ask about.
	d.music(ClipSeconds*second - 1)
	if d.clips != 0 {
		t.Fatal("asked before a whole clip had been heard")
	}
	d.want(false, "", "")

	// The first answer after silence is believed at once.
	d.music(1)
	if d.clips != 1 {
		t.Fatalf("%d clips, want 1", d.clips)
	}
	d.want(true, "Introduction", "Original Album Series")
}

func TestClipsAreSentAtTheInterval(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(60 * second)
	if len(d.sent) != 3 || d.sent[0] != 12*second || d.sent[1] != 32*second || d.sent[2] != 52*second {
		t.Fatalf("clips sent at blocks %v", d.sent)
	}
}

func TestATrackIsNotReplacedOnOneAnswer(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)

	// One answer that disagrees, and then the old one again: a recogniser
	// that was wrong for a moment. The room is not repainted for it.
	d.hears(home)
	d.music(20 * second)
	d.want(true, "Introduction", "Original Album Series")
	d.hears(intro)
	d.music(20 * second)
	d.want(true, "Introduction", "Original Album Series")
}

func TestTheNextTrack(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)

	d.hears(time_)
	d.quiet(3 * second) // the gap between two tracks is not a stop
	d.want(true, "Introduction", "Original Album Series")

	// Heard once at the next interval, and asked about again a clip later,
	// not an interval later.
	d.music(40 * second)
	if len(d.sent) < 2 || d.sent[1]-d.sent[0] != ClipSeconds*second {
		t.Fatalf("the second opinion was asked for at blocks %v", d.sent)
	}
	// It keeps the album the side began with: the recogniser names the
	// record two ways, and one record has one cover.
	d.want(true, "Does Anybody Really Know What Time It Is?", "Original Album Series")
}

func TestMusicNobodyKnows(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)

	// A miss or two is a quiet passage.
	d.hearsNothing()
	d.music(20 * second * (missesBeforeUnknown - 1))
	d.want(true, "Introduction", "Original Album Series")

	// Several are a track it does not know. Still playing: it can be heard.
	d.music(20 * second)
	d.want(true, "", "")
}

func TestARecordNobodyKnows(t *testing.T) {
	d := newDeck(t)
	d.hearsNothing()
	d.music(ClipSeconds * second)
	d.want(true, "", "")
}

func TestTheNeedleLifted(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)

	d.quiet(20*second - 1)
	d.want(true, "Introduction", "Original Album Series")
	d.quiet(1)
	d.want(false, "", "")
}

func TestTurningTheRecordOverKeepsTheAlbum(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)
	d.quiet(90 * second)
	d.want(false, "", "")

	d.hears(time_)
	d.music(ClipSeconds * second)
	d.want(true, "Does Anybody Really Know What Time It Is?", "Original Album Series")
}

func TestAnotherRecord(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	d.music(ClipSeconds * second)

	// Another artist is another record, however soon.
	d.quiet(30 * second)
	d.hears(home)
	d.music(ClipSeconds * second)
	d.want(true, "Take Me Home", "The Singles")

	// And so is the same artist after a long silence.
	d.quiet(int(forgetAfter/time.Second) * second)
	d.hears(Match{Artist: "Phil Collins", Title: "Sussudio", Album: "No Jacket Required"})
	d.music(ClipSeconds * second)
	d.want(true, "Sussudio", "No Jacket Required")
}

func TestAClickIsNotMusic(t *testing.T) {
	d := newDeck(t)
	d.hears(intro)
	for i := 0; i < 5; i++ {
		d.music(2)
		d.quiet(15 * second)
	}
	if d.clips != 0 {
		t.Fatalf("%d clips sent for an input that only clicked", d.clips)
	}
	d.want(false, "", "")
}

func TestARecogniserThatCannotBeAskedTeachesNothing(t *testing.T) {
	d := newDeck(t)
	d.cannotAsk()
	d.music(60 * second)
	// Not "playing, unknown": nobody was asked.
	d.want(false, "", "")
	if d.clips != 3 {
		t.Fatalf("%d clips, want it to keep asking", d.clips)
	}

	d.hears(intro)
	d.music(20 * second)
	d.cannotAsk()
	d.music(5 * 20 * second)
	d.want(true, "Introduction", "Original Album Series")
}

func TestLevel(t *testing.T) {
	if got := Level(hush()); got > -90 {
		t.Fatalf("silence is %.1f dBFS", got)
	}

	// A constant is not a sound, however far from zero it sits.
	offset := make([]int16, blockSamples)
	for i := range offset {
		offset[i] = 265
	}
	if got := Level(offset); got > -90 {
		t.Fatalf("a constant offset is %.1f dBFS", got)
	}

	square := make([]int16, blockSamples)
	for i := range square {
		square[i] = 32767
		if i%2 == 1 {
			square[i] = -32767
		}
	}
	if got := Level(square); math.Abs(got) > 0.01 {
		t.Fatalf("a full-scale square wave is %.2f dBFS, want 0", got)
	}

	if got := Level(tone()); got < DefaultSilenceDB {
		t.Fatalf("the test tone is %.1f dBFS, below the silence line", got)
	}
}
