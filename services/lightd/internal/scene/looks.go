package scene

import "fmt"

// A look is a scene that is not derived from a cover: a preset, known by
// name (ADR-0019). Something that knows what is playing may say that a cover
// has one; what the name looks like is decided here, with the other choices
// about colour and hardware, and nothing here knows what an album is.
//
// Looks are few. Each is for a record whose look is as well known as its
// title.

// LookSpectrum is the colours of the rainbow, red to violet.
const LookSpectrum = "spectrum"

// spectrum is six colours in equal shares: the bands a prism throws, as a
// strip of LEDs makes them. They are chosen, not measured, and are sent
// before gamma like any others.
var spectrum = [][3]uint8{
	{255, 0, 0},   // red
	{255, 110, 0}, // orange
	{255, 215, 0}, // yellow
	{0, 200, 40},  // green
	{0, 80, 255},  // blue
	{140, 0, 255}, // violet
}

// SpectrumEffects are the motions the spectrum can be shown with: the whole
// rainbow moving round the strip, or the strip turning through each colour in
// succession. Both are effects every stand already has.
var SpectrumEffects = []string{"sweep", "morph"}

// LookOptions are the choices a look leaves open.
type LookOptions struct {
	// SpectrumEffect is how the spectrum moves: one of SpectrumEffects.
	SpectrumEffect string
}

// Look returns the scene for a named look, and whether the name is known. A
// name that is not known is not an error: the cover is lit from its own
// colours, the way a stand shows an effect it does not know as solid.
//
// Speed and brightness are the stand's usual ones. A look changes what is
// shown, not how bright the room is.
func Look(name string, opts Options, look LookOptions) (Scene, bool) {
	switch name {
	case LookSpectrum:
		effect := look.SpectrumEffect
		if effect == "" {
			effect = SpectrumEffects[0]
		}
		palette := make([]Color, 0, len(spectrum))
		for _, rgb := range spectrum {
			palette = append(palette, Color{RGB: rgb, Weight: round4(1 / float64(len(spectrum)))})
		}
		return Scene{
			V:          ContractVersion,
			Effect:     effect,
			Speed:      opts.Speed,
			Brightness: opts.Brightness,
			Palette:    palette,
		}, true
	}
	return Scene{}, false
}

// Idle is what a stand shows when there is no cover to light it from: nothing
// is playing (ADR-0024). It is the scene the firmware starts with before it
// has heard anything - a steady, dim, warm white - so a stand that is idle
// and one that has just been switched on look the same, and neither is dark.
// A dark stand cannot be told from a dead one.
//
// It is not the usual brightness turned down. The room is not being lit for
// a record, and how bright that is has its own answer.
func Idle() Scene {
	return Scene{
		V:          ContractVersion,
		Effect:     "solid",
		Speed:      0,
		Brightness: 0.15,
		Palette:    []Color{{RGB: [3]uint8{255, 170, 90}, Weight: 1}},
	}
}

// ValidSpectrumEffect reports whether effect is one the spectrum can be shown
// with.
func ValidSpectrumEffect(effect string) error {
	for _, known := range SpectrumEffects {
		if effect == known {
			return nil
		}
	}
	return fmt.Errorf("scene: the spectrum is shown with one of %v, not %q", SpectrumEffects, effect)
}
