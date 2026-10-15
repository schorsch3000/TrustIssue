package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"trustissues/trustctl/internal/params"
)

func TestExtractFontAndLogo(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "device.json")
	var out, errOut bytes.Buffer

	// no profile: the built-in font, no logo
	fontFile := filepath.Join(dir, "font.bin")
	if err := extractCommand("font", []string{"extract", fontFile, "--profile", profilePath}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(fontFile)
	if !bytes.Equal(got, params.DefaultFont) || params.ValidateFont(got) != nil {
		t.Error("the built-in font was not written intact")
	}
	if err := extractCommand("logo", []string{"extract", filepath.Join(dir, "l.png"), "--profile", profilePath}, &out, &errOut); err == nil {
		t.Error("a logo was extracted from an empty profile")
	}

	// a profile font wins
	custom := append([]byte{}, params.DefaultFont...)
	custom[10] ^= 0xFF
	customFile := filepath.Join(dir, "custom.bin")
	os.WriteFile(customFile, custom, 0o644)
	os.WriteFile(profilePath, []byte(`{"font": "`+customFile+`"}`), 0o644)
	if err := extractCommand("font", []string{"extract", fontFile, "--profile", profilePath}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(fontFile)
	if !bytes.Equal(got, custom) {
		t.Error("the profile's font was not written")
	}

	if err := extractCommand("font", []string{"extract"}, &out, &errOut); err == nil {
		t.Error("missing file name accepted")
	}
}
