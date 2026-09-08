// Package scene turns a palette into something a light stand can render.
//
// This is where the decisions that need to know about hardware live. The
// palette service deliberately returns raw perceptual data - ranked swatches
// with weights, no roles, no correction - because it has no idea what is on
// the other end. lightd does.
//
// What it does not do is gamma. Gamma is a property of the specific LED driver,
// so it belongs in firmware, next to the hardware it corrects. Keeping it out
// of the payload means `mosquitto_sub` shows the colours actually intended,
// which matters the first time something looks wrong.
package scene

import (
	"fmt"
	"sort"

	"github.com/cdeever/eds/services/lightd/internal/oklab"
	"github.com/cdeever/eds/services/lightd/internal/palette"
)

// ContractVersion is the `v` stamped on every published scene.
const ContractVersion = 1

// Color is one entry of a scene's palette, already corrected for the strip.
type Color struct {
	RGB    [3]uint8 `json:"rgb"`
	Weight float64  `json:"weight"`
}

// Scene is the payload published to a stand. It is a description of what to
// render, never a frame: the device animates locally so that WiFi latency
// never reaches the light, and so a broker outage leaves the stand doing
// something sensible instead of frozen mid-sweep.
type Scene struct {
	V          int     `json:"v"`
	Effect     string  `json:"effect"`
	Speed      float64 `json:"speed"`
	Brightness float64 `json:"brightness"`
	Palette    []Color `json:"palette"`
}

// Options are the knobs that map art onto a strip.
type Options struct {
	Effect     string
	Speed      float64
	Brightness float64

	// MinChroma rejects swatches too close to neutral to read as a colour.
	// Weight alone is not enough: asking the extractor for few swatches from
	// busy art forces clusters to merge, and a merged centroid lands near the
	// average of what it merged - a high-weight grey that would light the
	// stand with nothing.
	MinChroma float64

	// Saturation multiplies chroma. A palette that looks right on a monitor
	// reads washed out on a strip, so the aesthetic correction happens here.
	Saturation float64

	// MaxColors caps the palette. A stand can only show so many colours
	// before an effect turns to soup.
	MaxColors int
}

// DefaultOptions are tuned for an addressable strip behind an LP jacket.
func DefaultOptions() Options {
	return Options{
		Effect:     "breathe",
		Speed:      0.4,
		Brightness: 0.8,
		MinChroma:  0.05,
		Saturation: 1.25,
		MaxColors:  4,
	}
}

// Build maps ranked swatches onto a scene.
func Build(swatches []palette.Swatch, opts Options) (Scene, error) {
	if len(swatches) == 0 {
		return Scene{}, fmt.Errorf("scene: no swatches to map")
	}
	if opts.Effect == "" {
		return Scene{}, fmt.Errorf("scene: no effect named")
	}

	usable := filterByChroma(swatches, opts.MinChroma)

	ranked := make([]palette.Swatch, len(usable))
	copy(ranked, usable)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Weight > ranked[j].Weight })

	if opts.MaxColors > 0 && len(ranked) > opts.MaxColors {
		ranked = ranked[:opts.MaxColors]
	}

	// Weights are renormalised after filtering and truncation, so they describe
	// the palette that was actually sent rather than the one extracted.
	var total float64
	for _, s := range ranked {
		total += s.Weight
	}
	if total <= 0 {
		return Scene{}, fmt.Errorf("scene: swatches carry no weight")
	}

	colors := make([]Color, 0, len(ranked))
	for _, s := range ranked {
		lab := oklab.Lab{L: s.OKLab[0], A: s.OKLab[1], B: s.OKLab[2]}
		rgb := lab.ScaleChroma(opts.Saturation).ToSRGB()
		colors = append(colors, Color{
			RGB:    [3]uint8{rgb.R, rgb.G, rgb.B},
			Weight: round4(s.Weight / total),
		})
	}

	return Scene{
		V:          ContractVersion,
		Effect:     opts.Effect,
		Speed:      opts.Speed,
		Brightness: opts.Brightness,
		Palette:    colors,
	}, nil
}

// filterByChroma drops neutrals, but never returns nothing: genuinely
// monochrome art still has to light the stand, so the most colourful swatch
// available survives even when it is below the floor.
func filterByChroma(swatches []palette.Swatch, minChroma float64) []palette.Swatch {
	kept := make([]palette.Swatch, 0, len(swatches))
	for _, s := range swatches {
		if s.Chroma >= minChroma {
			kept = append(kept, s)
		}
	}
	if len(kept) > 0 {
		return kept
	}

	best := swatches[0]
	for _, s := range swatches[1:] {
		if s.Chroma > best.Chroma {
			best = s
		}
	}
	return []palette.Swatch{best}
}

func round4(v float64) float64 {
	return float64(int(v*10000+0.5)) / 10000
}
