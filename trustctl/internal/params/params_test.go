package params_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"trustissues/trustctl/internal/hexfile"
	"trustissues/trustctl/internal/params"
)

func imageWithBlock() *hexfile.Image {
	img := &hexfile.Image{}
	for i := range img.Data {
		img.Data[i] = 0xFF
	}
	block := bytes.Repeat([]byte{0xFF}, params.BlockSize)
	copy(block, []byte{'T', 'I', 'P', 'B', 1, 0x01, 0x20, 0x00})
	img.Write(params.BlockAddress, block)
	return img
}

func goodFont() []byte {
	font := make([]byte, params.FontSize)
	copy(font, []byte{32, 127, 1, 1})
	return font
}

func TestApply(t *testing.T) {
	img := imageWithBlock()
	if !params.Present(img) {
		t.Fatal("the block was not found")
	}
	full, dim := uint8(0x30), uint8(0x01)
	logo := bytes.Repeat([]byte{0x55}, params.LogoSize)
	font := goodFont()
	font[10] = 0x77
	if err := params.Apply(img, params.Patch{ContrastFull: &full, ContrastDim: &dim, Font: font, Logo: logo}); err != nil {
		t.Fatal(err)
	}
	b := img.Bytes(params.BlockAddress, params.BlockSize)
	if b[6] != 0x30 || b[7] != 0x01 || b[5] != 0x03 || b[0x10+10] != 0x77 || !bytes.Equal(b[0x314:], logo) {
		t.Fatalf("block after patch: % x", b[:24])
	}
	if string(b[:4]) != "TIPB" {
		t.Error("the magic was damaged")
	}
}

func TestApplyKeepsWhatIsNotPatched(t *testing.T) {
	img := imageWithBlock()
	full := uint8(9)
	if err := params.Apply(img, params.Patch{ContrastFull: &full}); err != nil {
		t.Fatal(err)
	}
	b := img.Bytes(params.BlockAddress, params.BlockSize)
	if b[6] != 9 || b[7] != 0 || b[5] != 0x01 {
		t.Fatalf("% x", b[:8])
	}
}

func TestApplyErrors(t *testing.T) {
	empty := &hexfile.Image{}
	full := uint8(1)
	if err := params.Apply(empty, params.Patch{ContrastFull: &full}); err == nil || !strings.Contains(err.Error(), "no device parameter block") {
		t.Errorf("an image without a block: %v", err)
	}
	if err := params.Apply(empty, params.Patch{}); err != nil {
		t.Errorf("nothing to patch must not fail: %v", err)
	}
	img := imageWithBlock()
	if err := params.Apply(img, params.Patch{Font: make([]byte, 100)}); err == nil {
		t.Error("a short font was accepted")
	}
	bad := goodFont()
	bad[1] = 100
	if err := params.Apply(img, params.Patch{Font: bad}); err == nil {
		t.Error("a font with a wrong header was accepted")
	}
	if err := params.Apply(img, params.Patch{Logo: make([]byte, 10)}); err == nil {
		t.Error("a short logo was accepted")
	}
}

func TestDefaultFontIsTheFirmwareFont(t *testing.T) {
	data, err := os.ReadFile("../../../firmware/fonts/victoriamedium8.bin")
	if err != nil {
		t.Skip(err)
	}
	if !bytes.Equal(data, params.DefaultFont) {
		t.Error("the embedded font differs from firmware/fonts/victoriamedium8.bin; copy it over")
	}
	if err := params.ValidateFont(params.DefaultFont); err != nil {
		t.Error(err)
	}
}
