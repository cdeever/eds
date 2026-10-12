// Package smallcover makes a cover small.
//
// A cover arrives as the player or a catalog had it: up to 600 pixels a
// side, a JPEG of either kind or a PNG, tens or hundreds of kilobytes. That
// suits a program with memory to spare. It does not suit a subscriber with a
// hundred kilobytes for everything, which cannot hold the image, let alone
// decode a progressive one. So nowplayd publishes the cover a second time in
// a form anything can take (ADR-0020): a baseline JPEG that fits a small
// square, in a few kilobytes.
//
// Nothing here knows who wants it. The size is "small", chosen once, not the
// size of some screen.
package smallcover

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // a cover may be any of these
	"image/jpeg"
	_ "image/png"
)

const (
	// Side is the longer side of a small cover, in pixels. The other is
	// whatever keeps the picture's shape. A cover smaller than this is
	// scaled up to it, so a subscriber is always handed the same size.
	Side = 240

	// MaxBytes is the most a small cover weighs. Quality is given up to stay
	// under it, and a picture that cannot be brought under it has no small
	// form at all: a subscriber sizes a buffer by this number.
	MaxBytes = 32 << 10

	// maxPixels bounds what one cover can make this allocate. A cover is
	// some hundreds of pixels a side; the header of a hostile image can
	// claim anything.
	maxPixels = 4096 * 4096
)

// The qualities tried, best first. Eighty is where a cover stops looking
// like a JPEG at this size; below thirty it looks like nothing else.
var qualities = []int{80, 70, 60, 50, 40, 30}

// ErrTooLarge means the picture could not be brought under MaxBytes.
var ErrTooLarge = errors.New("smallcover: the picture will not fit the size allowed")

// Make returns data as a small cover: a baseline JPEG, three components of
// eight bits, whose longer side is Side.
func Make(data []byte) ([]byte, error) {
	conf, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("smallcover: not a picture this can read: %w", err)
	}
	if conf.Width < 1 || conf.Height < 1 || conf.Width*conf.Height > maxPixels {
		return nil, fmt.Errorf("smallcover: a picture of %dx%d is not a cover", conf.Width, conf.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("smallcover: %w", err)
	}

	small := Scale(src, Side)

	// Encoded from RGBA and never from a gray image, even for a cover with
	// no color in it: a gray image is written with one component, and the
	// promise is three. The encoder writes baseline, which is the other half
	// of the promise - it has no progressive mode to choose by mistake.
	for _, quality := range qualities {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, small, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("smallcover: %w", err)
		}
		if out.Len() <= MaxBytes {
			return out.Bytes(), nil
		}
	}
	return nil, ErrTooLarge
}

// Scale returns src with its longer side made side pixels, and its shape
// kept. Anything transparent is put on black.
func Scale(src image.Image, side int) *image.RGBA {
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()

	dw, dh := side, side
	switch {
	case sw > sh:
		dh = max(1, (sh*side+sw/2)/sw)
	case sh > sw:
		dw = max(1, (sw*side+sh/2)/sh)
	}

	// One known layout to read from, whatever the picture was decoded as.
	// The alpha it leaves is premultiplied, which is what "on black" means.
	flat := image.NewRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(flat, flat.Bounds(), src, bounds.Min, draw.Src)

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xs, ys := spans(sw, dw), spans(sh, dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var r, g, b, total float64
			for _, row := range ys[y] {
				for _, col := range xs[x] {
					weight := row.weight * col.weight
					at := flat.PixOffset(col.at, row.at)
					r += weight * float64(flat.Pix[at])
					g += weight * float64(flat.Pix[at+1])
					b += weight * float64(flat.Pix[at+2])
					total += weight
				}
			}
			at := dst.PixOffset(x, y)
			dst.Pix[at] = uint8(r/total + 0.5)
			dst.Pix[at+1] = uint8(g/total + 0.5)
			dst.Pix[at+2] = uint8(b/total + 0.5)
			dst.Pix[at+3] = 0xff
		}
	}
	return dst
}

// A span is one source pixel's share of a destination pixel.
type span struct {
	at     int
	weight float64
}

// spans says, for each of to destination pixels along one axis, which of the
// from source pixels it is made of and how much of each.
//
// Scaling down, a destination pixel is the average of the stretch of source
// it covers, with the pixels at either end counted by how far in they reach.
// That is what keeps a cover's fine print from turning to sparkle, which
// picking one source pixel in every two or three does. Scaling up, it is a
// blend of the two source pixels it falls between.
func spans(from, to int) [][]span {
	out := make([][]span, to)
	if to >= from {
		for i := range out {
			// Where the center of this pixel falls among the source's.
			pos := (float64(i)+0.5)*float64(from)/float64(to) - 0.5
			lo := int(pos)
			if pos < 0 {
				lo, pos = 0, 0
			}
			frac := pos - float64(lo)
			hi := min(lo+1, from-1)
			out[i] = []span{{lo, 1 - frac}, {hi, frac}}
		}
		return out
	}

	ratio := float64(from) / float64(to)
	for i := range out {
		start, end := float64(i)*ratio, float64(i+1)*ratio
		for at := int(start); at < from && float64(at) < end; at++ {
			weight := min(end, float64(at+1)) - max(start, float64(at))
			if weight > 0 {
				out[i] = append(out[i], span{at, weight})
			}
		}
	}
	return out
}
