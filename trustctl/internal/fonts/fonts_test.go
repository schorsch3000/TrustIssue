package fonts

import (
	"bytes"
	"os"
	"testing"
)

func TestBuiltInFonts(t *testing.T) {
	names := Names()
	if len(names) < 7 {
		t.Fatalf("only %v", names)
	}
	for _, name := range names {
		data, ok := Get(name)
		if !ok || len(data) != 772 || !bytes.Equal(data[:4], []byte{32, 127, 1, 1}) {
			t.Errorf("%s is no 772 byte font", name)
		}
	}
	if _, ok := Get(Default); !ok {
		t.Error("the default font is missing")
	}
}

func TestDefaultIsTheFirmwareFont(t *testing.T) {
	want, err := os.ReadFile("../../../firmware/fonts/victoriamedium8.bin")
	if err != nil {
		t.Skip(err)
	}
	got, _ := Get(Default)
	if !bytes.Equal(got, want) {
		t.Error("differs from firmware/fonts/victoriamedium8.bin; the firmware's built-in default must be the same")
	}
}

func TestSpecs(t *testing.T) {
	for spec, want := range map[string]bool{
		"victoriabold8": true, "builtin:5x8": true, "./victoriabold8": false,
		"victoriabold8.bin": false, "nonsense": false, "dir/5x8": false,
	} {
		if Builtin(spec) != want {
			t.Errorf("Builtin(%q) = %v", spec, !want)
		}
	}
	if _, err := Load("builtin:nonsense"); err == nil {
		t.Error("unknown built-in name accepted")
	}
}
