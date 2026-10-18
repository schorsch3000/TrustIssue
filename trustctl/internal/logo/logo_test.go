package logo_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"trustissue/trustctl/internal/logo"
)

func pixel(bitmap []byte, x, y int) bool { return bitmap[(y/8)*logo.Width+x]>>(y%8)&1 == 1 }

func convert(t *testing.T, img image.Image, invert bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := logo.Convert(&buf, invert)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != logo.Bytes {
		t.Fatalf("%d bytes", len(out))
	}
	return out
}

func TestTruecolorBlackIsOffAnythingElseIsOn(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	img.Set(0, 0, color.NRGBA{0, 0, 1, 255}) // almost black: on
	img.Set(1, 0, color.NRGBA{1, 0, 0, 255}) // on
	img.Set(2, 0, color.NRGBA{0, 1, 0, 255}) // on
	img.Set(3, 0, color.NRGBA{255, 255, 255, 255})
	img.Set(4, 0, color.NRGBA{255, 255, 255, 0}) // transparent: composited over black, off
	img.Set(5, 0, color.NRGBA{0, 0, 0, 255})     // black: off
	b := convert(t, img, false)
	for x, want := range []bool{true, true, true, true, false, false} {
		if pixel(b, x, 0) != want {
			t.Errorf("pixel %d,0: got %v want %v", x, !want, want)
		}
	}
}

func TestGray16UsesTheRawValue(t *testing.T) {
	img := image.NewGray16(image.Rect(0, 0, 128, 32))
	img.SetGray16(0, 0, color.Gray16{Y: 1}) // 8 bit conversion would round it to black
	img.SetGray16(1, 0, color.Gray16{Y: 0})
	b := convert(t, img, false)
	if !pixel(b, 0, 0) || pixel(b, 1, 0) {
		t.Error("a 16 bit gray of 1 must be on, 0 must be off")
	}
}

func TestPaletteDarkestIsOffAndEqualColorsToo(t *testing.T) {
	pal := color.Palette{
		color.RGBA{200, 0, 0, 255},  // 0: red, on
		color.RGBA{10, 10, 10, 255}, // 1: darkest
		color.RGBA{10, 10, 10, 255}, // 2: the same color again: off too
		color.RGBA{0, 0, 100, 255},  // 3: dark too, but a little brighter than the darkest: on
	}
	img := image.NewPaletted(image.Rect(0, 0, 128, 32), pal)
	for i := 0; i < 4; i++ {
		img.SetColorIndex(i, 0, uint8(i))
	}
	b := convert(t, img, false)
	for x, want := range []bool{true, false, false, true} {
		if pixel(b, x, 0) != want {
			t.Errorf("palette index %d: on = %v, want %v", x, !want, want)
		}
	}
}

func TestPaletteTiesGoToTheFirstColor(t *testing.T) {
	// two different colors of the same luminance: only the first is "the dark one"
	pal := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{0, 0, 0, 255}}
	img := image.NewPaletted(image.Rect(0, 0, 128, 32), pal)
	img.SetColorIndex(0, 0, 1)
	b := convert(t, img, false)
	if pixel(b, 0, 0) {
		t.Error("index 1 has the same color as the dark index 0 and must be off")
	}
}

func TestOneBitPaletteWhiteFirst(t *testing.T) {
	pal := color.Palette{color.White, color.Black} // index 0 is white
	img := image.NewPaletted(image.Rect(0, 0, 128, 32), pal)
	img.SetColorIndex(0, 0, 0)
	img.SetColorIndex(1, 0, 1)
	b := convert(t, img, false)
	if !pixel(b, 0, 0) || pixel(b, 1, 0) {
		t.Error("the darkest palette entry (black, index 1) must be off")
	}
}

func TestSmallImageIsCentered(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			img.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	b := convert(t, img, false)
	// margins: (128-3)/2 = 62 left (odd margin: the extra column goes right), (32-3)/2 = 14 top
	for y := 0; y < 32; y++ {
		for x := 0; x < 128; x++ {
			inside := x >= 62 && x < 65 && y >= 14 && y < 17
			if pixel(b, x, y) != inside {
				t.Fatalf("pixel %d,%d: %v", x, y, pixel(b, x, y))
			}
		}
	}
}

func TestInvertIncludesTheMargin(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 2, 2))
	img.SetGray(0, 0, color.Gray{Y: 255})
	b := convert(t, img, true)
	if pixel(b, 63, 15) {
		t.Error("the lit pixel must be dark when inverted")
	}
	if !pixel(b, 0, 0) || !pixel(b, 64, 16) {
		t.Error("the margin and the dark pixels must be lit when inverted")
	}
}

func TestTooLarge(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 129, 10)))
	if _, err := logo.Convert(&buf, false); err == nil {
		t.Error("an image wider than the display was accepted")
	}
	buf.Reset()
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 10, 33)))
	if _, err := logo.Convert(&buf, false); err == nil {
		t.Error("an image taller than the display was accepted")
	}
}

func TestBitLayout(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 128, 32))
	img.SetGray(5, 9, color.Gray{Y: 255}) // page 1, bit 1 of byte 128+5
	b := convert(t, img, false)
	if b[128+5] != 0x02 {
		t.Errorf("byte %#x, want 0x02", b[128+5])
	}
}

func TestToImageRoundTrip(t *testing.T) {
	bitmap := make([]byte, logo.Bytes)
	for i := range bitmap {
		bitmap[i] = byte(i*37 + i/128)
	}
	img, err := logo.ToImage(bitmap)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	back, err := logo.Convert(&buf, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, bitmap) {
		t.Error("bitmap changed on the way through a PNG")
	}
	if _, err := logo.ToImage(bitmap[:10]); err == nil {
		t.Error("a short bitmap was accepted")
	}
}
