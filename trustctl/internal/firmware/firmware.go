// Package firmware carries the firmware images that belong to this build of
// trustctl. trustctl and the firmware are released together, so a user flashes
// a device with nothing but the trustctl binary.
//
// The files are produced by tools/build/embed_firmware.sh from firmware/core and
// firmware/calibration; a test checks that the version matches the source.
package firmware

import (
	"bytes"
	_ "embed"
	"strings"

	"trustissues/trustctl/internal/hexfile"
)

//go:embed core.hex
var coreHex []byte

//go:embed calibration.hex
var calibrationHex []byte

//go:embed VERSION
var version string

// Version is the firmware version baked into this build.
func Version() string { return strings.TrimSpace(version) }

// Names of the built-in images.
const (
	Core        = "core"
	Calibration = "calibration"
)

// Image parses a built-in firmware ("core" or "calibration"); ok is false for
// any other name.
func Image(name string) (img *hexfile.Image, ok bool, err error) {
	var data []byte
	switch name {
	case Core:
		data = coreHex
	case Calibration:
		data = calibrationHex
	default:
		return nil, false, nil
	}
	img, err = hexfile.Parse(bytes.NewReader(data))
	return img, true, err
}
