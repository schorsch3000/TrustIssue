// Package params describes the device parameter block that sits at a fixed
// address in the firmware's flash and holds what is specific to one device
// and one taste: contrast values, the font and the idle logo. trustctl patches
// it into the firmware's hex file before flashing, so no recompile is needed.
package params

import (
	"bytes"
	"fmt"

	"trustissues/trustctl/internal/hexfile"
)

const (
	BlockAddress = 0x7000
	BlockSize    = 0x514

	offVersion      = 4
	offFlags        = 5
	offContrastFull = 6
	offContrastDim  = 7
	offDisplay      = 8 // 0xA8 <mux> 0xDA <COM pins>, sent to the display as they are
	offFont         = 0x10
	offLogo         = 0x314

	// FontSize is a 96 glyph 8x8 U8x8 font: 4 header bytes and 8 bytes per glyph.
	FontSize = 772
	// LogoSize is the 128x32 bitmap.
	LogoSize = 512

	flagFont = 0x01
	flagLogo = 0x02
	flagTall = 0x04 // a 128x64 display: only the top half is used
)

// Patch says what to change; nil leaves the firmware's default.
type Patch struct {
	ContrastFull *uint8
	ContrastDim  *uint8
	Font         []byte
	Logo         []byte
	Rows         int // 32 or 64, the height of the display; 0 leaves the firmware's default (32)
}

// Empty reports whether nothing would change.
func (p Patch) Empty() bool {
	return p.ContrastFull == nil && p.ContrastDim == nil && p.Font == nil && p.Logo == nil && p.Rows == 0
}

// Present reports whether the image carries a parameter block.
func Present(img *hexfile.Image) bool {
	return bytes.Equal(img.Bytes(BlockAddress, 4), []byte("TIPB")) && img.Bytes(BlockAddress+offVersion, 1)[0] == 1
}

// ValidateFont checks that data is a 96 glyph 8x8 font in U8x8 format
// (header: first character 32, last 127, 1x1 tiles).
func ValidateFont(data []byte) error {
	if len(data) != FontSize {
		return fmt.Errorf("a font file must be %d bytes (96 glyphs of 8x8 pixels in U8x8 format), this one has %d", FontSize, len(data))
	}
	if !bytes.Equal(data[:4], []byte{32, 127, 1, 1}) {
		return fmt.Errorf("the font header is %v, expected [32 127 1 1] (characters 32 to 127, 1x1 tiles)", data[:4])
	}
	return nil
}

// Apply patches the image.
func Apply(img *hexfile.Image, p Patch) error {
	if p.Empty() {
		return nil
	}
	if !Present(img) {
		return fmt.Errorf("this firmware has no device parameter block at %#x (not a build of firmware/core?), nothing can be patched into it", BlockAddress)
	}
	flags := img.Bytes(BlockAddress+offFlags, 1)[0]
	if p.ContrastFull != nil {
		img.Write(BlockAddress+offContrastFull, []byte{*p.ContrastFull})
	}
	if p.ContrastDim != nil {
		img.Write(BlockAddress+offContrastDim, []byte{*p.ContrastDim})
	}
	switch p.Rows {
	case 0:
	case 32:
		img.Write(BlockAddress+offDisplay, []byte{0xA8, 0x1F, 0xDA, 0x02})
		flags &^= flagTall
	case 64:
		img.Write(BlockAddress+offDisplay, []byte{0xA8, 0x3F, 0xDA, 0x12})
		flags |= flagTall
	default:
		return fmt.Errorf("a display has 32 or 64 rows, not %d", p.Rows)
	}
	if p.Font != nil {
		if err := ValidateFont(p.Font); err != nil {
			return err
		}
		img.Write(BlockAddress+offFont, p.Font)
		flags |= flagFont
	}
	if p.Logo != nil {
		if len(p.Logo) != LogoSize {
			return fmt.Errorf("the logo must be %d bytes, not %d", LogoSize, len(p.Logo))
		}
		img.Write(BlockAddress+offLogo, p.Logo)
		flags |= flagLogo
	}
	img.Write(BlockAddress+offFlags, []byte{flags})
	return nil
}

// Extract returns the font and the logo stored in an image's parameter block.
// A part that the block does not mark as valid is nil.
func Extract(img *hexfile.Image) (font, logo []byte, err error) {
	if !Present(img) {
		return nil, nil, fmt.Errorf("this firmware has no device parameter block at %#x (not a build of firmware/core?)", BlockAddress)
	}
	flags := img.Bytes(BlockAddress+offFlags, 1)[0]
	if flags&flagFont != 0 {
		font = img.Bytes(BlockAddress+offFont, FontSize)
	}
	if flags&flagLogo != 0 {
		logo = img.Bytes(BlockAddress+offLogo, LogoSize)
	}
	return font, logo, nil
}
