package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/cdeever/eds/services/lightd/internal/scene"
)

var (
	prism  = []byte("\xff\xd8\xff\xe0 the dark side of the moon")
	sleeve = []byte("\xff\xd8\xff\xe0 some other record")
)

func shaOf(image []byte) string {
	sum := sha256.Sum256(image)
	return hex.EncodeToString(sum[:])
}

func lookFor(image []byte, name string) []byte {
	raw, _ := json.Marshal(map[string]any{"v": 1, "look": name, "art": shaOf(image)})
	return raw
}

func sceneAt(t *testing.T, pub *fakePublisher, i int) scene.Scene {
	t.Helper()
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if i < 0 {
		i += len(pub.payloads)
	}
	if i < 0 || i >= len(pub.payloads) {
		t.Fatalf("no scene %d of %d", i, len(pub.payloads))
	}
	var sc scene.Scene
	if err := json.Unmarshal(pub.payloads[i], &sc); err != nil {
		t.Fatal(err)
	}
	return sc
}

func isSpectrum(sc scene.Scene) bool {
	return len(sc.Palette) == 6 && sc.Palette[0].RGB == [3]uint8{255, 0, 0}
}

// The look before its cover: the room is lit once, as the spectrum, and the
// extractor is never asked what colors a black sleeve has.
func TestACoverWithALookIsLitAsTheLook(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()

	if err := s.ApplyLook(ctx, lookFor(prism, "spectrum")); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 0 {
		t.Fatal("a look with no cover lit the room")
	}
	if err := s.ApplyCover(ctx, prism); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 {
		t.Fatalf("lit the room %d times, want once", pub.count())
	}
	sc := sceneAt(t, pub, 0)
	if !isSpectrum(sc) || sc.Effect != "sweep" || sc.V != scene.ContractVersion {
		t.Fatalf("scene %+v", sc)
	}
	if ex.gotImage != nil {
		t.Error("the extractor was asked about a cover that has a look")
	}
}

// The cover before its look, which is the order two retained messages may
// arrive in at start-up: lit from its own colors, then corrected.
func TestALookThatArrivesAfterItsCoverRelightsTheRoom(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()

	if err := s.ApplyCover(ctx, prism); err != nil {
		t.Fatal(err)
	}
	if isSpectrum(sceneAt(t, pub, 0)) {
		t.Fatal("a cover with no look yet was lit as the spectrum")
	}
	if err := s.ApplyLook(ctx, lookFor(prism, "spectrum")); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 2 || !isSpectrum(sceneAt(t, pub, 1)) {
		t.Fatalf("after the look: %d scenes", pub.count())
	}
	// Said again, as a retained message is on every reconnect: nothing.
	s.ApplyLook(ctx, lookFor(prism, "spectrum"))
	s.ApplyCover(ctx, prism)
	if pub.count() != 2 {
		t.Fatalf("a repeat lit the room again: %d scenes", pub.count())
	}
}

// A look belongs to one cover. The record played next must not inherit it,
// whichever of its two messages comes first.
func TestALookDoesNotLandOnAnotherCover(t *testing.T) {
	for _, order := range []string{"look first", "cover first"} {
		t.Run(order, func(t *testing.T) {
			ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
			s := newServer(t, ex, pub)
			ctx := context.Background()
			s.ApplyLook(ctx, lookFor(prism, "spectrum"))
			s.ApplyCover(ctx, prism)
			before := pub.count()

			if order == "look first" {
				s.ApplyLook(ctx, lookFor(sleeve, ""))
				if pub.count() != before {
					t.Fatal("a look for a cover not yet here relit the room")
				}
				s.ApplyCover(ctx, sleeve)
			} else {
				s.ApplyCover(ctx, sleeve)
				s.ApplyLook(ctx, lookFor(sleeve, ""))
			}
			if pub.count() != before+1 {
				t.Fatalf("the next record lit the room %d times, want once", pub.count()-before)
			}
			if isSpectrum(sceneAt(t, pub, -1)) {
				t.Fatal("the next record was lit as the spectrum")
			}
		})
	}
}

// A late look for a record that has gone changes nothing.
func TestALookForACoverThatHasGoneIsIgnored(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	s.ApplyCover(ctx, sleeve)
	if err := s.ApplyLook(ctx, lookFor(prism, "spectrum")); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 || isSpectrum(sceneAt(t, pub, 0)) {
		t.Fatalf("%d scenes", pub.count())
	}
}

// A name this does not know is not an error: the cover is lit from its own
// colors, as a stand shows an effect it does not know as solid.
func TestALookThisDoesNotKnowLeavesTheCoverItsOwnColors(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	if err := s.ApplyLook(ctx, lookFor(prism, "holographic")); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyCover(ctx, prism); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 || isSpectrum(sceneAt(t, pub, 0)) || ex.gotImage == nil {
		t.Fatalf("%d scenes; extractor asked: %v", pub.count(), ex.gotImage != nil)
	}
}

func TestTheSpectrumMovesAsConfigured(t *testing.T) {
	pub := &fakePublisher{}
	s := New(Options{Extractor: &fakeExtractor{resp: goodPalette()}, Publisher: pub, TopicPrefix: "eds",
		DefaultStand: "lp-stand-01", Swatches: 6, Scene: scene.DefaultOptions(),
		Looks: scene.LookOptions{SpectrumEffect: "morph"}})
	ctx := context.Background()
	s.ApplyLook(ctx, lookFor(prism, "spectrum"))
	s.ApplyCover(ctx, prism)
	if sc := sceneAt(t, pub, 0); sc.Effect != "morph" || !isSpectrum(sc) {
		t.Fatalf("scene %+v", sc)
	}
}

// A look needs nothing from the extractor, so the extractor being down does
// not keep the spectrum off the stand.
func TestALookDoesNotNeedTheExtractor(t *testing.T) {
	ex, pub := &fakeExtractor{err: errors.New("connection refused")}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	s.ApplyLook(ctx, lookFor(prism, "spectrum"))
	if err := s.ApplyCover(ctx, prism); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 || !isSpectrum(sceneAt(t, pub, 0)) {
		t.Fatalf("%d scenes", pub.count())
	}
}

// A cover that could not be lit is tried again; one that was is not.
func TestACoverThatFailedIsStillOwedAScene(t *testing.T) {
	ex, pub := &fakeExtractor{err: errors.New("connection refused")}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	if err := s.ApplyCover(ctx, sleeve); err == nil {
		t.Fatal("a failed extraction was reported as lit")
	}
	ex.err, ex.resp = nil, goodPalette()
	if err := s.ApplyCover(ctx, sleeve); err != nil || pub.count() != 1 {
		t.Fatalf("the retry: %v, %d scenes", err, pub.count())
	}
}

func TestALookThatCannotBeReadIsRefused(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	s.ApplyCover(ctx, prism)

	for name, payload := range map[string]string{
		"not JSON":        "spectrum",
		"another version": fmt.Sprintf(`{"v":2,"look":"spectrum","art":%q}`, shaOf(prism)),
		"no version":      fmt.Sprintf(`{"look":"spectrum","art":%q}`, shaOf(prism)),
	} {
		if err := s.ApplyLook(ctx, []byte(payload)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if pub.count() != 1 || isSpectrum(sceneAt(t, pub, 0)) {
		t.Fatalf("a look that could not be read changed the room: %d scenes", pub.count())
	}
}

// A look that names no cover is not a cover's look: it is the look of there
// being none (ADR-0028), and the cover held does not take it.
func TestALookForNoCoverIsNotTheHeldCovers(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	s.ApplyCover(ctx, prism)

	if err := s.ApplyLook(ctx, []byte(`{"v":1,"look":"spectrum"}`)); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 || isSpectrum(sceneAt(t, pub, 0)) {
		t.Fatalf("a look for no cover changed a room that has one: %d scenes", pub.count())
	}
}

// ADR-0028: a cover named dark lights nothing. The stand is sent dark, the
// extractor is not asked, and the next cover is lit as usual.
func TestACoverNamedDarkLightsNothing(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()

	look := fmt.Sprintf(`{"v":1,"look":%q,"art":%q}`, scene.LookDark, shaOf(prism))
	if err := s.ApplyLook(ctx, []byte(look)); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyCover(ctx, prism); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 1 || sceneAt(t, pub, 0).Brightness != 0 {
		t.Fatalf("%d scenes, first %+v", pub.count(), sceneAt(t, pub, 0))
	}

	if err := s.ApplyCover(ctx, []byte("ANOTHER")); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 2 || sceneAt(t, pub, 1).Brightness == 0 {
		t.Fatalf("the cover after was not lit: %d scenes", pub.count())
	}
}

// The scanner: a look with a speed and brightness of its own, and one that
// a cover can take up and put down as its tracks change.
func TestTheScannerComesAndGoesOnOneCover(t *testing.T) {
	ex, pub := &fakeExtractor{resp: goodPalette()}, &fakePublisher{}
	s := newServer(t, ex, pub)
	ctx := context.Background()
	s.ApplyCover(ctx, prism)
	if got := sceneAt(t, pub, 0); got.Effect == "scanner" {
		t.Fatalf("a cover with no look is the scanner: %+v", got)
	}

	s.ApplyLook(ctx, lookFor(prism, scene.LookScanner))
	got := sceneAt(t, pub, 1)
	if got.Effect != "scanner" || got.Brightness != 1 || len(got.Palette) != 1 || got.Palette[0].RGB != [3]uint8{255, 0, 0} {
		t.Fatalf("scanner scene = %+v", got)
	}

	s.ApplyLook(ctx, lookFor(prism, scene.LookDark))
	if pub.count() != 3 || sceneAt(t, pub, 2).Brightness != 0 {
		t.Fatalf("after the scanner's track: %d scenes", pub.count())
	}
}
