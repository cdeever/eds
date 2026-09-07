package scene

import (
	"math"
	"testing"

	"github.com/cdeever/eds/services/lightd/internal/oklab"
	"github.com/cdeever/eds/services/lightd/internal/palette"
)

func swatch(l, a, b, weight float64) palette.Swatch {
	lab := oklab.Lab{L: l, A: a, B: b}
	return palette.Swatch{
		OKLab:     [3]float64{l, a, b},
		Weight:    weight,
		Chroma:    lab.Chroma(),
		Lightness: l,
	}
}

var (
	orange  = swatch(0.625, 0.147, 0.118, 0.40)
	blue    = swatch(0.449, -0.036, -0.116, 0.30)
	yellow  = swatch(0.804, 0.017, 0.161, 0.20)
	green   = swatch(0.541, -0.104, 0.035, 0.07)
	neutral = swatch(0.536, 0.0015, 0.021, 0.03) // chroma ~0.021, a grey
)

func TestBuildRanksAndNormalises(t *testing.T) {
	got, err := Build([]palette.Swatch{orange, blue, yellow, green}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	if got.V != ContractVersion {
		t.Errorf("v = %d, want %d", got.V, ContractVersion)
	}

	var total float64
	for i, c := range got.Palette {
		total += c.Weight
		if i > 0 && c.Weight > got.Palette[i-1].Weight {
			t.Error("palette is not ranked by weight")
		}
	}
	if math.Abs(total-1) > 1e-3 {
		t.Errorf("weights sum to %v, want 1", total)
	}
}

// The finding that motivated MinChroma: a high-weight grey would otherwise
// light the stand with nothing at all.
func TestDropsNeutralSwatchesEvenWhenHeavy(t *testing.T) {
	heavyGrey := neutral
	heavyGrey.Weight = 0.9

	got, err := Build([]palette.Swatch{heavyGrey, orange}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Palette) != 1 {
		t.Fatalf("expected the grey to be dropped, got %d colours", len(got.Palette))
	}
	if got.Palette[0].Weight != 1.0 {
		t.Errorf("surviving weight = %v, want renormalised to 1", got.Palette[0].Weight)
	}
	c := got.Palette[0]
	if !(c.RGB[0] > c.RGB[2]) {
		t.Errorf("surviving colour is not the orange: %v", c.RGB)
	}
}

// Monochrome art still has to light the stand.
func TestNeverReturnsAnEmptyPalette(t *testing.T) {
	got, err := Build([]palette.Swatch{neutral, swatch(0.4, 0.001, 0.002, 0.5)}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Palette) != 1 {
		t.Fatalf("want exactly the most colourful swatch, got %d", len(got.Palette))
	}
	if got.Palette[0].Weight != 1.0 {
		t.Errorf("weight = %v, want 1", got.Palette[0].Weight)
	}
}

func TestTruncatesToMaxColors(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxColors = 2

	got, err := Build([]palette.Swatch{orange, blue, yellow, green}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Palette) != 2 {
		t.Fatalf("got %d colours, want 2", len(got.Palette))
	}

	var total float64
	for _, c := range got.Palette {
		total += c.Weight
	}
	if math.Abs(total-1) > 1e-3 {
		t.Errorf("weights not renormalised after truncation: sum %v", total)
	}
}

func TestSaturationBoostIncreasesChroma(t *testing.T) {
	plain := DefaultOptions()
	plain.Saturation = 1.0
	boosted := DefaultOptions()
	boosted.Saturation = 1.6

	a, err := Build([]palette.Swatch{orange}, plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build([]palette.Swatch{orange}, boosted)
	if err != nil {
		t.Fatal(err)
	}

	spread := func(c Color) int {
		lo, hi := int(c.RGB[0]), int(c.RGB[0])
		for _, v := range c.RGB {
			if int(v) < lo {
				lo = int(v)
			}
			if int(v) > hi {
				hi = int(v)
			}
		}
		return hi - lo
	}

	if spread(b.Palette[0]) <= spread(a.Palette[0]) {
		t.Errorf("boosting did not widen the channel spread: %v vs %v",
			a.Palette[0].RGB, b.Palette[0].RGB)
	}
}

func TestEffectOverride(t *testing.T) {
	opts := DefaultOptions()
	opts.Effect = "sweep"

	got, err := Build([]palette.Swatch{orange}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effect != "sweep" {
		t.Errorf("effect = %q, want sweep", got.Effect)
	}
}

func TestRejectsEmptyInput(t *testing.T) {
	if _, err := Build(nil, DefaultOptions()); err == nil {
		t.Error("expected an error for no swatches")
	}
}

func TestRejectsMissingEffect(t *testing.T) {
	opts := DefaultOptions()
	opts.Effect = ""
	if _, err := Build([]palette.Swatch{orange}, opts); err == nil {
		t.Error("expected an error for an unnamed effect")
	}
}

func TestRejectsWeightlessSwatches(t *testing.T) {
	zero := orange
	zero.Weight = 0
	if _, err := Build([]palette.Swatch{zero}, DefaultOptions()); err == nil {
		t.Error("expected an error when swatches carry no weight")
	}
}
