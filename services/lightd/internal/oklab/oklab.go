// Package oklab converts OKLab colours to sRGB and adjusts their chroma.
//
// The palette service already carries OKLab coordinates in its response, so
// lightd never has to convert forwards. It only needs the inverse transform
// and a way to make a colour more saturated without moving its hue, which is
// the one aesthetic correction that belongs on this side of the boundary.
package oklab

import "math"

// Lab is an OKLab colour: perceptual lightness, then the two opponent axes.
type Lab struct {
	L float64
	A float64
	B float64
}

// RGB is an 8-bit sRGB colour, ready to put on the wire.
type RGB struct {
	R uint8
	G uint8
	B uint8
}

// Chroma is the distance from the neutral axis: how colourful, independent of
// lightness. A swatch with near-zero chroma is a grey and will not read as a
// colour on a strip no matter how much of the sleeve it covers.
func (l Lab) Chroma() float64 {
	return math.Sqrt(l.A*l.A + l.B*l.B)
}

// ScaleChroma multiplies the opponent axes, leaving lightness and hue alone.
// Scaling in OKLab rather than HSV is what keeps a boosted colour recognisably
// the same colour.
func (l Lab) ScaleChroma(k float64) Lab {
	return Lab{L: l.L, A: l.A * k, B: l.B * k}
}

// linearRGB is the un-gamma-encoded triple, possibly outside 0..1.
type linearRGB struct{ r, g, b float64 }

func (l Lab) toLinear() linearRGB {
	lms := [3]float64{
		l.L + 0.3963377774*l.A + 0.2158037573*l.B,
		l.L - 0.1055613458*l.A - 0.0638541728*l.B,
		l.L - 0.0894841775*l.A - 1.2914855480*l.B,
	}
	for i := range lms {
		lms[i] = lms[i] * lms[i] * lms[i]
	}
	return linearRGB{
		r: 4.0767416621*lms[0] - 3.3077115913*lms[1] + 0.2309699292*lms[2],
		g: -1.2684380046*lms[0] + 2.6097574011*lms[1] - 0.3413193965*lms[2],
		b: -0.0041960863*lms[0] - 0.7034186147*lms[1] + 1.7076147010*lms[2],
	}
}

func (c linearRGB) inGamut() bool {
	const eps = 1e-9
	return c.r >= -eps && c.r <= 1+eps &&
		c.g >= -eps && c.g <= 1+eps &&
		c.b >= -eps && c.b <= 1+eps
}

func encode(v float64) uint8 {
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(clamp01(v), 1.0/2.4) - 0.055
	}
	return uint8(clamp01(v)*255 + 0.5)
}

// ToSRGB converts to 8-bit sRGB, pulling the colour back into gamut by
// reducing chroma rather than clipping channels.
//
// Per-channel clipping is the obvious approach and the wrong one: clipping a
// blown red channel while leaving green and blue alone shifts the hue, so a
// boosted orange arrives as a different colour than the one on the sleeve.
// Walking chroma down holds hue and lightness fixed and gives up only
// saturation, which is the property that was artificially raised anyway.
func (l Lab) ToSRGB() RGB {
	if l.toLinear().inGamut() {
		return l.encodeDirect()
	}

	// Binary search the largest chroma scale that stays in gamut. Twenty
	// iterations resolves far below one 8-bit step.
	lo, hi := 0.0, 1.0
	for i := 0; i < 20; i++ {
		mid := (lo + hi) / 2
		if l.ScaleChroma(mid).toLinear().inGamut() {
			lo = mid
		} else {
			hi = mid
		}
	}
	return l.ScaleChroma(lo).encodeDirect()
}

func (l Lab) encodeDirect() RGB {
	c := l.toLinear()
	return RGB{R: encode(c.r), G: encode(c.g), B: encode(c.b)}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
