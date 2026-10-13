package scene

import "fmt"

// A look is a scene that is not derived from a cover: a preset, known by
// name (ADR-0019). Something that knows what is playing may say that a cover
// has one; what the name looks like is decided here, with the other choices
// about color and hardware, and nothing here knows what an album is.
//
// Looks are few. Each is for a record whose look is as well known as its
// title.

// LookSpectrum is the colors of the rainbow, red to violet.
const LookSpectrum = "spectrum"

// spectrum is six colors in equal shares: the bands a prism throws, as a
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
// rainbow moving around the strip, or the strip turning through each color in
// succession. Both are effects every stand already has.
var SpectrumEffects = []string{"sweep", "morph"}

// LookOptions are the choices a look leaves open.
type LookOptions struct {
	// SpectrumEffect is how the spectrum moves: one of SpectrumEffects.
	SpectrumEffect string
}

// Look returns the scene for a named look, and whether the name is known. A
// name that is not known is not an error: the cover is lit from its own
// colors, the way a stand shows an effect it does not know as solid.
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
	case LookScanner:
		// Its own speed and its own brightness, where the spectrum takes the
		// stand's: it is nothing if it is slow, and one point of light has
		// to carry the whole strip. There and back in a second and a half.
		return Scene{
			V:          ContractVersion,
			Effect:     "scanner",
			Speed:      0.9,
			Brightness: 1,
			Palette:    []Color{{RGB: [3]uint8{255, 0, 0}, Weight: 1}},
		}, true
	case LookDark:
		return Dark(), true
	}
	return Scene{}, false
}

// LookScanner is one red point going from end to end of the strip and back,
// with a tail behind it.
const LookScanner = "scanner"

// LookDark is a cover the stand is not to be lit for (ADR-0028). Whose
// cover that is, and why, is whoever names it's to know.
const LookDark = "dark"

// LookStandby is the look of no cover at all, where whoever says there is
// none says also that the room is waiting for one (ADR-0028).
const LookStandby = "standby"

// NoCover is what a stand shows when there is no cover to light it from, for
// the look that was named for that, if one was: waiting is Idle, and
// anything else is Dark. A name that is not known is no name.
func NoCover(look string) Scene {
	if look == LookStandby {
		return Idle()
	}
	return Dark()
}

// Dark is a stand with nothing to show and nobody said to be waiting: every
// LED off (ADR-0028). It is a scene like any other, sent and retained, so a
// stand that restarts goes dark again once it has heard it.
//
// The color is Idle's, and is not seen. A scene has to carry one.
func Dark() Scene {
	dark := Idle()
	dark.Brightness = 0
	return dark
}

// Idle is what a stand shows when there is no cover to light it from, and
// the room is waiting for one (ADR-0024, ADR-0028): a steady, dim, warm
// white. It was once also what the firmware showed before it had heard
// anything. The firmware now starts dark (ADR-0029), with this color at
// brightness 0.
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
