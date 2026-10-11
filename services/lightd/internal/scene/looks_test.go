package scene

import (
	"math"
	"testing"
)

func TestTheSpectrumIsSixColoursInEqualShares(t *testing.T) {
	opts := DefaultOptions()
	sc, ok := Look(LookSpectrum, opts, LookOptions{})
	if !ok {
		t.Fatal("the spectrum is not a known look")
	}
	if sc.V != ContractVersion || len(sc.Palette) != 6 {
		t.Fatalf("scene %+v", sc)
	}
	var total float64
	for _, c := range sc.Palette {
		if math.Abs(c.Weight-1.0/6) > 0.001 {
			t.Errorf("a share of %v, want a sixth", c.Weight)
		}
		total += c.Weight
	}
	if math.Abs(total-1) > 0.01 {
		t.Errorf("shares add to %v", total)
	}
	// Red first and violet last: the order a prism throws them in.
	first, last := sc.Palette[0].RGB, sc.Palette[5].RGB
	if first[0] < 200 || first[2] > 50 || last[2] < 200 || last[1] > 50 {
		t.Errorf("first %v, last %v", first, last)
	}
	// A stand takes eight colours at most.
	if len(sc.Palette) > 8 {
		t.Errorf("%d colours is more than a stand takes", len(sc.Palette))
	}
}

// A look changes what is shown, not how fast or how bright.
func TestALookKeepsTheStandsSpeedAndBrightness(t *testing.T) {
	opts := DefaultOptions()
	opts.Speed, opts.Brightness = 0.7, 0.35
	sc, _ := Look(LookSpectrum, opts, LookOptions{})
	if sc.Speed != 0.7 || sc.Brightness != 0.35 {
		t.Errorf("speed %v, brightness %v", sc.Speed, sc.Brightness)
	}
}

func TestTheSpectrumMovesTheWayItIsToldTo(t *testing.T) {
	opts := DefaultOptions()
	if sc, _ := Look(LookSpectrum, opts, LookOptions{}); sc.Effect != "sweep" {
		t.Errorf("by default the spectrum is shown with %q", sc.Effect)
	}
	if sc, _ := Look(LookSpectrum, opts, LookOptions{SpectrumEffect: "morph"}); sc.Effect != "morph" {
		t.Errorf("asked for morph, got %q", sc.Effect)
	}
	if ValidSpectrumEffect("sweep") != nil || ValidSpectrumEffect("morph") != nil {
		t.Error("a known effect was refused")
	}
	for _, bad := range []string{"", "breathe", "solid", "disco"} {
		if ValidSpectrumEffect(bad) == nil {
			t.Errorf("%q was accepted for the spectrum", bad)
		}
	}
}

func TestANameThatIsNotALookIsNotOne(t *testing.T) {
	for _, name := range []string{"", "Spectrum", "rainbow", "prism"} {
		if _, ok := Look(name, DefaultOptions(), LookOptions{}); ok {
			t.Errorf("%q is a look", name)
		}
	}
}
