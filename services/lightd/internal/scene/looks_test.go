package scene

import (
	"math"
	"testing"
)

func TestTheSpectrumIsSixColorsInEqualShares(t *testing.T) {
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
	// A stand takes eight colors at most.
	if len(sc.Palette) > 8 {
		t.Errorf("%d colors is more than a stand takes", len(sc.Palette))
	}
}

func TestTheGrenadeIsTheSleevesRedsAndThePinkOfTheHand(t *testing.T) {
	opts := DefaultOptions()
	opts.Effect, opts.Speed, opts.Brightness = "chase", 0.4, 0.8
	sc, ok := Look(LookGrenade, opts, LookOptions{})
	if !ok {
		t.Fatal("the grenade is not a known look")
	}
	if sc.V != ContractVersion || len(sc.Palette) != 4 {
		t.Fatalf("scene %+v", sc)
	}
	// The stand's own way of showing a cover.
	if sc.Effect != "chase" || sc.Speed != 0.4 || sc.Brightness != 0.8 {
		t.Errorf("effect %q, speed %v, brightness %v", sc.Effect, sc.Speed, sc.Brightness)
	}
	// A red ground, the most of it, and a pale pink among the accents.
	ground := sc.Palette[0]
	if ground.RGB[0] < 200 || ground.RGB[1] > 40 || ground.Weight < 0.4 {
		t.Errorf("ground %+v", ground)
	}
	pink := false
	var total float64
	for _, c := range sc.Palette {
		total += c.Weight
		if c.RGB[0] > 220 && c.RGB[1] > 150 && c.RGB[2] > 150 {
			pink = true
		}
	}
	if !pink {
		t.Error("no pale pink in the palette")
	}
	if math.Abs(total-1) > 0.01 {
		t.Errorf("shares add to %v", total)
	}
	// The preset is not handed out to be changed.
	sc.Palette[0].RGB = [3]uint8{0, 0, 0}
	if again, _ := Look(LookGrenade, opts, LookOptions{}); again.Palette[0].RGB == [3]uint8{0, 0, 0} {
		t.Error("a scene's palette is the preset itself")
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
