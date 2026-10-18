package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trustissue/trustctl/internal/fonts"
	"trustissue/trustctl/internal/params"
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
	if !bytes.Equal(got, defaultFont()) || params.ValidateFont(got) != nil {
		t.Error("the built-in font was not written intact")
	}
	if err := extractCommand("logo", []string{"extract", filepath.Join(dir, "l.png"), "--profile", profilePath}, &out, &errOut); err == nil {
		t.Error("a logo was extracted from an empty profile")
	}

	// a profile font wins
	custom := append([]byte{}, defaultFont()...)
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

	// a built-in font by name, and the list
	if err := extractCommand("font", []string{"extract", fontFile, "--name", "5x8"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(fontFile)
	if want, _ := fonts.Get("5x8"); !bytes.Equal(got, want) {
		t.Error("--name 5x8 did not write that font")
	}
	if err := extractCommand("font", []string{"extract", fontFile, "--name", "nope"}, &out, &errOut); err == nil {
		t.Error("an unknown built-in name was accepted")
	}
	out.Reset()
	if err := extractCommand("font", []string{"list"}, &out, &errOut); err != nil || !bytes.Contains(out.Bytes(), []byte("victoriabold8")) {
		t.Errorf("font list: %v %q", err, out.String())
	}
}

// A device that says it is unlocked or has no PIN is called out by hello and add.
func TestUnlockedWarning(t *testing.T) {
	var out bytes.Buffer
	warnAboutDevice("TrustIssue 0.1.0 proto 1 caps=autowipe", &out)
	if out.Len() != 0 {
		t.Errorf("a locked device was warned about: %q", out.String())
	}
	warnAboutDevice("TrustIssue 0.1.0 proto 1 caps=autowipe unlocked", &out)
	if !strings.Contains(out.String(), "not locked") || strings.Contains(out.String(), "no update PIN") {
		t.Errorf("no warning, or a wrong one: %q", out.String())
	}
	out.Reset()
	warnAboutDevice("TrustIssue 0.1.0 proto 1 nopin", &out)
	if !strings.Contains(out.String(), "no update PIN") || strings.Contains(out.String(), "not locked") {
		t.Errorf("no PIN warning, or a wrong one: %q", out.String())
	}
	out.Reset()
	warnAboutDevice("TrustIssue 0.1.0 proto 1 nopin unlocked", &out)
	if !strings.Contains(out.String(), "no update PIN") || !strings.Contains(out.String(), "not locked") {
		t.Errorf("both warnings expected: %q", out.String())
	}
}

// A font given to flash replaces the profile's, which may name a file that is gone.
func TestFlashFontOverridesMissingProfileFont(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "device.json")
	os.WriteFile(profilePath, []byte(`{"font": "`+filepath.Join(dir, "gone.bin")+`"}`), 0o644)

	_, _, err := flashPatch(profilePath, "")
	if err == nil || !strings.Contains(err.Error(), "forget font") {
		t.Errorf("a missing profile font should say how to get out: %v", err)
	}
	patch, p, err := flashPatch(profilePath, "victoriabold8")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := fonts.Get("victoriabold8")
	if !bytes.Equal(patch.Font, want) || p.Font != "victoriabold8" {
		t.Error("the --font font was not used")
	}
	if _, _, err := flashPatch(profilePath, "nonsense"); err == nil {
		t.Error("an unknown font name was accepted")
	}
}
