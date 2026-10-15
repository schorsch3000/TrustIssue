// Package logo converts PNG images to the bitmap of the device's idle logo.
package logo

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // register the PNG decoder
	"io"
)

const (
	Width  = 128
	Height = 32
	// Bytes is the size of the bitmap: 8 vertical pixels per byte, LSB at the
	// top, 128 bytes per page of 8 rows, like the SSD1306's display memory.
	Bytes = Width * Height / 8
)

// Convert reads a PNG and returns the 512 byte bitmap. The image must not be
// larger than the display; a smaller one is centered (an odd margin goes to
// the top and left). There is no scaling or dithering:
//
//   - truecolor, gray and 1-bit images: exactly black (every channel 0, after
//     compositing over black) is off, anything else is on, however dark;
//   - palette images: the darkest palette color is off (ties: the first one),
//     and so is every entry with exactly that color; all others are on.
//
// invert swaps on and off, including the margin around a smaller image.
func Convert(r io.Reader, invert bool) ([]byte, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read the image: %w", err)
	}
	return FromImage(img, invert)
}

type rgb struct{ r, g, b uint32 }

func channels(c color.Color) rgb {
	r, g, b, _ := c.RGBA() // premultiplied: a transparent pixel is black
	return rgb{r, g, b}
}

func luminance(c rgb) uint64 { return 299*uint64(c.r) + 587*uint64(c.g) + 114*uint64(c.b) }

// FromImage is Convert for an already decoded image.
func FromImage(img image.Image, invert bool) ([]byte, error) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w > Width || h > Height {
		return nil, fmt.Errorf("the image is %dx%d, the display only %dx%d", w, h, Width, Height)
	}
	var isOff func(x, y int) bool
	if p, ok := img.(*image.Paletted); ok {
		var darkest rgb
		best := ^uint64(0)
		for _, entry := range p.Palette { // the first of equally dark colors wins
			if c := channels(entry); luminance(c) < best {
				best, darkest = luminance(c), c
			}
		}
		isOff = func(x, y int) bool { return channels(p.At(x, y)) == darkest }
	} else {
		isOff = func(x, y int) bool { return channels(img.At(x, y)) == rgb{} }
	}
	left, top := (Width-w)/2, (Height-h)/2
	out := make([]byte, Bytes)
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			inside := x >= left && x < left+w && y >= top && y < top+h
			on := invert // outside the image: off, or on when inverted
			if inside {
				on = !isOff(bounds.Min.X+x-left, bounds.Min.Y+y-top) != invert
			}
			if on {
				out[(y/8)*Width+x] |= 1 << (y % 8)
			}
		}
	}
	return out, nil
}
