package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"trustissues/trustctl/internal/serialdev"
)

func TestDevicePathNormalizesDummy(t *testing.T) {
	for _, in := range []string{"DUMMY", "dummy", "Dummy"} {
		got, err := devicePath(options{device: in}, config{})
		if err != nil || got != "DUMMY" {
			t.Errorf("devicePath(%q) = %q, %v; want DUMMY", in, got, err)
		}
	}
}

func TestDummyDeviceKeepsItsKeysWhileOpen(t *testing.T) {
	dev, err := openDevice("dummy", false)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	if dev.Path != serialdev.DummyPath || !strings.HasPrefix(dev.Banner, "TrustIssue") {
		t.Fatalf("path %q banner %q", dev.Path, dev.Banner)
	}
	dev.Client.ReplyTimeout = 5 * time.Second
	var final string
	if err := dev.Client.Raw("ADD JBSWY3DPEHPK3PXP SHA1 6 30 GitHub", func(l string) { final = l }); err != nil || !strings.HasPrefix(final, "OK") {
		t.Fatalf("ADD: %q %v", final, err)
	}
	var lines []string
	if err := dev.Client.Raw("LIST", func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "GitHub") {
		t.Errorf("the key added to the dummy device is not listed: %q", lines)
	}
}

func TestFlashAndCalibrateRefuseTheDummyDevice(t *testing.T) {
	var out, errOut bytes.Buffer
	err := flashCommand(options{device: "DUMMY"}, nil, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "real device") {
		t.Errorf("flash on DUMMY: %v", err)
	}
	profile := t.TempDir() + "/device.json"
	err = calibrateCommand(options{device: "dummy"}, []string{"contrast", "full", "10", "--profile", profile}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "real device") {
		t.Errorf("calibrate on DUMMY: %v", err)
	}
}
