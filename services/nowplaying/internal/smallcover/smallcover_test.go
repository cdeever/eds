package smallcover

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"testing"
)

// Covers cannot be committed, so the tests draw their own: quadrants of flat
// color, which is what scaling must keep, and noise, which is what a JPEG
// finds hardest to make small.

func quadrants(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{200, 30, 30, 255}
			switch {
			case x >= w/2 && y < h/2:
				c = color.RGBA{30, 200, 30, 255}
			case x < w/2 && y >= h/2:
				c = color.RGBA{30, 30, 200, 255}
			case x >= w/2 && y >= h/2:
				c = color.RGBA{220, 220, 40, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func noise(w, h int) *image.RGBA {
	rng := rand.New(rand.NewSource(1)) // the same noise every run
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255
	}
	return img
}

func asPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func asJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decoded checks a small cover is what the package promises, and returns it.
func decoded(t *testing.T, small []byte) image.Image {
	t.Helper()
	if len(small) > MaxBytes {
		t.Fatalf("%d bytes, more than the %d allowed", len(small), MaxBytes)
	}
	img, format, err := image.Decode(bytes.NewReader(small))
	if err != nil {
		t.Fatalf("the small cover does not decode: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("the small cover is a %s", format)
	}
	if _, ok := img.(*image.YCbCr); !ok {
		t.Fatalf("decoded as %T: not three components", img)
	}
	if b := img.Bounds(); b.Dx() > Side || b.Dy() > Side || (b.Dx() != Side && b.Dy() != Side) {
		t.Fatalf("%dx%d does not fit %d with its longer side at it", b.Dx(), b.Dy(), Side)
	}
	return img
}

// baseline reports whether a JPEG is the sequential kind a small decoder
// reads: a baseline frame header (SOF0) and no progressive one (SOF2).
func baseline(data []byte) bool {
	sof0 := false
	for i := 2; i+3 < len(data); {
		if data[i] != 0xff {
			return false
		}
		marker := data[i+1]
		switch marker {
		case 0xc0:
			sof0 = true
		case 0xc2:
			return false
		case 0xda: // the picture itself starts; the headers are over
			return sof0
		}
		i += 2 + int(data[i+2])<<8 + int(data[i+3])
	}
	return sof0
}

func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d > -30 && d < 30
}

func TestACoverIsMadeSmallAndKeepsItsColors(t *testing.T) {
	for name, cover := range map[string][]byte{
		"a jpeg, as a player sends": asJPEG(t, quadrants(600, 600)),
		"a png":                     asPNG(t, quadrants(600, 600)),
		"an odd size":               asPNG(t, quadrants(599, 601)),
	} {
		t.Run(name, func(t *testing.T) {
			small, err := Make(cover)
			if err != nil {
				t.Fatal(err)
			}
			if !baseline(small) {
				t.Error("not a baseline jpeg")
			}
			img := decoded(t, small)
			b := img.Bounds()
			for _, c := range []struct {
				x, y    int
				r, g, b uint8
			}{
				{b.Dx() / 4, b.Dy() / 4, 200, 30, 30},
				{3 * b.Dx() / 4, b.Dy() / 4, 30, 200, 30},
				{b.Dx() / 4, 3 * b.Dy() / 4, 30, 30, 200},
				{3 * b.Dx() / 4, 3 * b.Dy() / 4, 220, 220, 40},
			} {
				r, g, bl, _ := img.At(c.x, c.y).RGBA()
				if !near(uint8(r>>8), c.r) || !near(uint8(g>>8), c.g) || !near(uint8(bl>>8), c.b) {
					t.Errorf("at %d,%d: %d,%d,%d, want about %d,%d,%d", c.x, c.y, r>>8, g>>8, bl>>8, c.r, c.g, c.b)
				}
			}
		})
	}
}

func TestACoverThatIsNotSquareKeepsItsShape(t *testing.T) {
	for name, c := range map[string]struct{ w, h, wantW, wantH int }{
		"wide":       {600, 300, 240, 120},
		"tall":       {300, 600, 120, 240},
		"a gatefold": {1000, 333, 240, 80},
		"a sliver":   {2000, 2, 240, 1},
	} {
		t.Run(name, func(t *testing.T) {
			small, err := Make(asPNG(t, quadrants(c.w, c.h)))
			if err != nil {
				t.Fatal(err)
			}
			if b := decoded(t, small).Bounds(); b.Dx() != c.wantW || b.Dy() != c.wantH {
				t.Errorf("%dx%d became %dx%d, want %dx%d", c.w, c.h, b.Dx(), b.Dy(), c.wantW, c.wantH)
			}
		})
	}
}

// A subscriber is always handed the same size, so a cover that starts
// smaller is brought up to it.
func TestACoverSmallerThanSmallIsScaledUp(t *testing.T) {
	small, err := Make(asPNG(t, quadrants(100, 100)))
	if err != nil {
		t.Fatal(err)
	}
	if b := decoded(t, small).Bounds(); b.Dx() != Side || b.Dy() != Side {
		t.Errorf("100x100 became %dx%d", b.Dx(), b.Dy())
	}
}

// A progressive JPEG is the reason this package exists: a small decoder
// cannot read one, and players send them. Drawn by ImageMagick as a 48x32
// gradient, red to blue; it is the only fixture here that is not made in the
// test, because Go cannot write one.
const progressive = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAA0JCgsKCA0LCgsODg0PEyAVExISEyccHhcgLikxMC4pLSwzOko+MzZGNywtQFdBRkxO" +
	"UlNSMj5aYVpQYEpRUk//2wBDAQ4ODhMREyYVFSZPNS01T09PT09PT09PT09PT09PT09PT09PT09PT09PT09PT09PT09PT09PT09P" +
	"T09PT09PT0//wgARCAAgADADASIAAhEBAxEB/8QAFwABAQEBAAAAAAAAAAAAAAAAAAMCBf/EABYBAQEBAAAAAAAAAAAAAAAAAAAF" +
	"Af/aAAwDAQACEAMQAAAB5qbLVExRMYTWZNExRMf/xAAUEAEAAAAAAAAAAAAAAAAAAABA/9oACAEBAAEFAkf/xAAVEQEBAAAAAAAA" +
	"AAAAAAAAAAAAE//aAAgBAwEBPwGyyyyyz//EABURAQEAAAAAAAAAAAAAAAAAAAAT/9oACAECAQE/AYIIIIIP/8QAFBABAAAAAAAA" +
	"AAAAAAAAAAAAQP/aAAgBAQAGPwJH/8QAFRABAQAAAAAAAAAAAAAAAAAAABH/2gAIAQEAAT8hqqqqqqqqqqqqqqqq/9oADAMBAAIA" +
	"AwAAABD774MMP//EABQRAQAAAAAAAAAAAAAAAAAAACD/2gAIAQMBAT8QQH//xAAVEQEBAAAAAAAAAAAAAAAAAAAAcf/aAAgBAgEB" +
	"PxCEIQhD/8QAFBABAAAAAAAAAAAAAAAAAAAAQP/aAAgBAQABPxBH/wD/AP8A/9k="

func TestAProgressiveJPEGComesOutBaseline(t *testing.T) {
	cover, err := base64.StdEncoding.DecodeString(progressive)
	if err != nil {
		t.Fatal(err)
	}
	if baseline(cover) {
		t.Fatal("the fixture is not progressive, so this proves nothing")
	}

	small, err := Make(cover)
	if err != nil {
		t.Fatal(err)
	}
	if !baseline(small) {
		t.Error("still not baseline")
	}
	img := decoded(t, small)
	if b := img.Bounds(); b.Dx() != 240 || b.Dy() != 160 {
		t.Errorf("48x32 became %dx%d", b.Dx(), b.Dy())
	}
	// Red at the top, blue at the bottom.
	r, _, b, _ := img.At(120, 4).RGBA()
	if r>>8 < 180 || b>>8 > 80 {
		t.Errorf("the top is %d,_,%d: not red", r>>8, b>>8)
	}
	r, _, b, _ = img.At(120, 155).RGBA()
	if b>>8 < 180 || r>>8 > 80 {
		t.Errorf("the bottom is %d,_,%d: not blue", r>>8, b>>8)
	}
}

// A cover with no color in it is still three components. A gray image
// handed to the encoder as gray comes out with one, which is not what was
// promised.
func TestAGrayCoverStillHasThreeComponents(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 300, 300))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i % 251)
	}
	small, err := Make(asPNG(t, gray))
	if err != nil {
		t.Fatal(err)
	}
	decoded(t, small)
}

func TestAGIFIsACoverToo(t *testing.T) {
	paletted := image.NewPaletted(image.Rect(0, 0, 64, 64), color.Palette{color.Black, color.RGBA{200, 30, 30, 255}})
	for i := range paletted.Pix {
		paletted.Pix[i] = 1
	}
	var buf bytes.Buffer
	if err := gif.Encode(&buf, paletted, nil); err != nil {
		t.Fatal(err)
	}
	small, err := Make(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	r, g, _, _ := decoded(t, small).At(120, 120).RGBA()
	if !near(uint8(r>>8), 200) || !near(uint8(g>>8), 30) {
		t.Errorf("the middle is %d,%d", r>>8, g>>8)
	}
}

// Noise is the hardest picture there is to make small. It must still come in
// under the size allowed, by giving up quality.
func TestTheHardestPictureStillFitsTheSizeAllowed(t *testing.T) {
	cover := asPNG(t, noise(240, 240))

	var at80 bytes.Buffer
	if err := jpeg.Encode(&at80, Scale(noise(240, 240), Side), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if at80.Len() <= MaxBytes {
		t.Fatalf("noise is only %d bytes at the first quality, so no quality was given up", at80.Len())
	}

	small, err := Make(cover)
	if err != nil {
		t.Fatalf("noise has no small form: %v", err)
	}
	decoded(t, small)
	t.Logf("noise: %d bytes at the first quality, %d as published", at80.Len(), len(small))
}

func TestWhatIsNotAPictureIsRefused(t *testing.T) {
	for name, data := range map[string][]byte{
		"nothing":          nil,
		"text":             []byte("online"),
		"a truncated jpeg": asJPEG(t, quadrants(64, 64))[:200],
		"a truncated png":  asPNG(t, quadrants(64, 64))[:60],
	} {
		t.Run(name, func(t *testing.T) {
			if small, err := Make(data); err == nil {
				t.Errorf("made %d bytes of it", len(small))
			}
		})
	}
}

// The header of a picture can claim any size at all, in a few bytes.
func TestAPictureClaimingToBeEnormousIsRefusedBeforeItIsDecoded(t *testing.T) {
	huge := asPNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	// A PNG's width and height are the eight bytes after "IHDR".
	at := bytes.Index(huge, []byte("IHDR")) + 4
	copy(huge[at:], []byte{0, 0, 0x40, 0, 0, 0, 0x40, 0}) // 16384 x 16384
	if _, err := Make(huge); err == nil {
		t.Error("accepted")
	}
}

// The same picture must make the same bytes: it is retained, and a
// subscriber that tells covers apart by what it was sent must not be told
// one cover is two.
func TestTheSamePictureMakesTheSameBytes(t *testing.T) {
	cover := asJPEG(t, quadrants(600, 600))
	a, err := Make(cover)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Make(cover)
	if !bytes.Equal(a, b) {
		t.Error("two different small covers from one cover")
	}
}
