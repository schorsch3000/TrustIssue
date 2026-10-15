package firmware

import (
	"os"
	"regexp"
	"testing"

	"trustissues/trustctl/internal/flasher"
	"trustissues/trustctl/internal/params"
)

func TestBuiltInImages(t *testing.T) {
	core, ok, err := Image(Core)
	if !ok || err != nil {
		t.Fatalf("core: ok=%v err=%v", ok, err)
	}
	if !params.Present(core) {
		t.Error("the built-in core firmware has no parameter block to patch")
	}
	calibration, ok, err := Image(Calibration)
	if !ok || err != nil {
		t.Fatalf("calibration: ok=%v err=%v", ok, err)
	}
	for name, img := range map[string]interface{ Highest() int }{"core": core, "calibration": calibration} {
		if img.Highest() >= flasher.BootStart {
			t.Errorf("%s reaches into the bootloader's section", name)
		}
	}
	if _, ok, _ := Image("firmware.hex"); ok {
		t.Error("an unknown name was taken for a built-in image")
	}
}

// The embedded images must come from the current source: run tools/build/embed_firmware.sh.
func TestVersionMatchesSource(t *testing.T) {
	source, err := os.ReadFile("../../../firmware/core/core.ino")
	if err != nil {
		t.Skip(err)
	}
	m := regexp.MustCompile(`VERSION\[\] PROGMEM = "([^"]+)"`).FindSubmatch(source)
	if m == nil {
		t.Fatal("no VERSION in core.ino")
	}
	if string(m[1]) != Version() {
		t.Errorf("core.ino says %s, the embedded firmware is %s: run tools/build/embed_firmware.sh", m[1], Version())
	}
}
