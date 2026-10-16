package send

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/tools/ota/internal/image"
)

// device behaves as the firmware's receiver does (app/ota, network/ota): it
// takes only the piece that starts at the next byte it wants, and answers
// every piece with that byte.
type device struct {
	mu      sync.Mutex
	project string
	slot    int
	running string // the ELF it runs

	receiving bool
	size      int
	next      int
	written   []byte
	offered   image.Desc

	answers chan []byte

	// Faults, by the number of the piece received (from 1).
	pieces   int
	lose     map[int]bool // the piece never arrives
	silent   map[int]bool // it arrives, and the answer is lost
	twice    map[int]bool // it arrives twice
	undo     bool         // the new build does not reach the broker
	noAnswer bool         // nothing is ever answered
}

func newDevice(project string) *device {
	return &device{project: project, slot: 1 << 20, running: "0000000000000000", answers: make(chan []byte, 64),
		lose: map[int]bool{}, silent: map[int]bool{}, twice: map[int]bool{}}
}

func (d *device) Answers() <-chan []byte { return d.answers }

func (d *device) say(a Answer) {
	if d.noAnswer {
		return
	}
	a.V = ContractVersion
	b, _ := json.Marshal(a)
	d.answers <- b
}

func at(n int) *int64 { v := int64(n); return &v }

func (d *device) Publish(topic string, payload []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case strings.HasSuffix(topic, "/ota"):
		var o Offer
		if json.Unmarshal(payload, &o) != nil || o.V != ContractVersion {
			d.say(Answer{State: "refused", Why: "it is not an offer this firmware reads"})
			return nil
		}
		if o.Project != d.project {
			d.say(Answer{State: "refused", Why: fmt.Sprintf("it is %s's, and this is %s", o.Project, d.project)})
			return nil
		}
		if o.Size < 1 || o.Size > d.slot {
			d.say(Answer{State: "refused", Why: "it is larger than the slot"})
			return nil
		}
		d.receiving, d.size, d.next, d.written = true, o.Size, 0, make([]byte, o.Size)
		d.offered = image.Desc{Project: o.Project, Version: o.Version, ELF: o.ELF}
		d.say(Answer{State: "ready", Next: at(0)})
	case strings.HasSuffix(topic, "/ota/chunk"):
		d.pieces++
		n := d.pieces
		if d.lose[n] {
			return nil
		}
		times := 1
		if d.twice[n] {
			times = 2
		}
		for range times {
			d.take(payload, d.silent[n])
		}
	}
	return nil
}

func (d *device) take(msg []byte, silent bool) {
	if !d.receiving || len(msg) <= 4 {
		return
	}
	off := int(binary.BigEndian.Uint32(msg))
	body := msg[4:]
	if off == d.next {
		if d.next+len(body) > d.size {
			d.receiving = false
			d.say(Answer{State: "refused", Why: "it is longer than was offered"})
			return
		}
		copy(d.written[d.next:], body)
		d.next += len(body)
	}
	if d.next == d.size {
		d.receiving = false
		d.say(Answer{State: "rebooting", Version: d.offered.Version, ELF: d.offered.ELF})
		if d.undo {
			d.say(Answer{State: "running", Version: "old", ELF: d.running})
		} else {
			d.running = d.offered.ELF
			d.say(Answer{State: "running", Version: d.offered.Version, ELF: d.running})
		}
		return
	}
	if !silent {
		d.say(Answer{State: "receiving", Next: at(d.next)})
	}
}

func build(n int) ([]byte, image.Desc) {
	img := make([]byte, n)
	for i := range img {
		img[i] = byte(i*31 + i>>8)
	}
	return img, image.Desc{Project: "lp-stand", Version: "abc1234", ELF: "0123456789abcdef"}
}

var quick = Waits{Ready: 200 * time.Millisecond, Piece: 50 * time.Millisecond, Tries: 4, Restart: 200 * time.Millisecond}

var topics = TopicsFor("eds", "lightstand", "lp-stand-01")

func TestTopicsAreTheDevicesOwn(t *testing.T) {
	want := Topics{"eds/lightstand/lp-stand-01/ota", "eds/lightstand/lp-stand-01/ota/chunk", "eds/lightstand/lp-stand-01/ota/state"}
	if topics != want {
		t.Errorf("TopicsFor = %+v", topics)
	}
}

func TestABuildIsSentAndKept(t *testing.T) {
	img, desc := build(3*Chunk + 100)
	d := newDevice("lp-stand")
	var asked []int
	res, err := Send(context.Background(), d, topics, img, desc, quick, func(next, size int) { asked = append(asked, next) })
	if err != nil {
		t.Fatal(err)
	}
	if !res.Kept || res.Running.ELF != desc.ELF {
		t.Errorf("result %+v, want kept", res)
	}
	if !bytes.Equal(d.written, img) {
		t.Error("what the device wrote is not the image")
	}
	if res.Sent != len(img) {
		t.Errorf("sent %d bytes for an image of %d", res.Sent, len(img))
	}
	if want := []int{0, Chunk, 2 * Chunk, 3 * Chunk, len(img)}; fmt.Sprint(asked) != fmt.Sprint(want) {
		t.Errorf("progress %v, want %v", asked, want)
	}
}

func TestALostPieceIsSentAgain(t *testing.T) {
	img, desc := build(4 * Chunk)
	d := newDevice("lp-stand")
	d.lose[2] = true
	d.silent[3] = true
	res, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Kept || !bytes.Equal(d.written, img) {
		t.Errorf("after a lost piece and a lost answer: kept=%v, image whole=%v", res.Kept, bytes.Equal(d.written, img))
	}
}

func TestAPieceDeliveredTwiceDoesNotRunAway(t *testing.T) {
	img, desc := build(6 * Chunk)
	d := newDevice("lp-stand")
	d.twice[1] = true
	d.twice[3] = true
	res, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Kept || !bytes.Equal(d.written, img) {
		t.Fatal("not kept after a duplicate")
	}
	// Each duplicate costs at most one piece sent again, not one per piece
	// after it.
	if res.Sent > len(img)+2*Chunk {
		t.Errorf("sent %d bytes for %d: duplicates ran away", res.Sent, len(img))
	}
}

func TestARefusalIsTheDevicesWords(t *testing.T) {
	img, desc := build(Chunk)
	d := newDevice("cover-screen")
	_, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "it is lp-stand's, and this is cover-screen") {
		t.Errorf("err = %v", err)
	}
	if d.pieces != 0 {
		t.Errorf("%d pieces sent after a refusal", d.pieces)
	}
}

func TestADeviceThatDoesNotAnswerIsSaidSo(t *testing.T) {
	img, desc := build(Chunk)
	d := newDevice("lp-stand")
	d.noAnswer = true
	_, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if err == nil || !strings.Contains(err.Error(), "did not answer the offer") {
		t.Errorf("err = %v", err)
	}
}

func TestADeviceThatStopsAnsweringIsGivenUp(t *testing.T) {
	img, desc := build(4 * Chunk)
	d := newDevice("lp-stand")
	for i := 2; i < 100; i++ {
		d.lose[i] = true
	}
	_, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if err == nil || !strings.Contains(err.Error(), "stopped answering at byte 8192") {
		t.Errorf("err = %v", err)
	}
}

func TestAnUndoneBuildIsReportedAsUndone(t *testing.T) {
	img, desc := build(2 * Chunk)
	d := newDevice("lp-stand")
	d.undo = true
	res, err := Send(context.Background(), d, topics, img, desc, quick, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kept || res.Running.ELF != "0000000000000000" {
		t.Errorf("result %+v, want undone", res)
	}
}

func TestAnswersFromBeforeTheOfferAreIgnored(t *testing.T) {
	img, desc := build(2 * Chunk)
	d := newDevice("lp-stand")
	d.answers <- []byte(`{"v":1,"state":"refused","why":"an old one"}`)
	d.answers <- []byte(`not json`)
	if _, err := Send(context.Background(), d, topics, img, desc, quick, nil); err != nil {
		t.Errorf("an answer from before the offer was read as this one's: %v", err)
	}
}

func TestPiecesCarryTheirOffset(t *testing.T) {
	img, _ := build(Chunk + 10)
	p := piece(img, Chunk)
	if binary.BigEndian.Uint32(p) != Chunk || len(p) != 4+10 || !bytes.Equal(p[4:], img[Chunk:]) {
		t.Errorf("piece at %d is wrong: % x", Chunk, p[:8])
	}
}
