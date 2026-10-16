package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"trustissues/trustctl/internal/fonts"
	"trustissues/trustctl/internal/logo"
	"trustissues/trustctl/internal/params"
	"trustissues/trustctl/internal/profile"
	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/serialdev"
)

const calibrateUsage = `calibrate commands (the device must run the calibration firmware, firmware/calibration):
  calibrate contrast full|dim VALUE   try a contrast (0-255, or 0x..) and remember it for "flash"
  calibrate logo FILE.png [--invert]  show the logo on the display and remember it for "flash"
  calibrate font NAME|FILE.bin        show a sample with a built-in font (see: font list) or a font file and remember it
  calibrate rotate on|off             try the display rotated by 180 degrees (not remembered: "config set rotate=on" on the core firmware)
  calibrate invert on|off             try swapped light and dark pixels (not remembered: "config set invert=on")
  calibrate show                      what "flash" will patch into the firmware
  calibrate forget logo|font|contrast drop a remembered setting
The settings are kept in ~/.config/trustctl/device.json (--profile PATH to use another file).
`

// calibration is a connection to the calibration firmware.
type calibration struct{ client *protocol.Client }

func (c calibration) send(command string) error {
	var reply string
	if err := c.client.Raw(command, func(l string) { reply = l }); err != nil {
		return err
	}
	if reply != "OK" {
		return fmt.Errorf("%s: %s", strings.Fields(command)[0], reply)
	}
	return nil
}

func (c calibration) bitmap(data []byte) error {
	if err := c.send("CLEAR"); err != nil {
		return err
	}
	for offset := 0; offset < len(data); offset += 16 {
		if err := c.send(fmt.Sprintf("LOGO %d %x", offset, data[offset:offset+16])); err != nil {
			return err
		}
	}
	return nil
}

func (c calibration) font(data []byte) error {
	padded := append(append([]byte(nil), data...), make([]byte, 784-len(data))...)
	for offset := 0; offset < len(padded); offset += 16 {
		if err := c.send(fmt.Sprintf("FONTDATA %d %x", offset, padded[offset:offset+16])); err != nil {
			return err
		}
	}
	if err := c.send("CLEAR"); err != nil {
		return err
	}
	for row, text := range []string{"Il1 O0oQ 5S 8B", "GitHub paypa1", "rn m vv cl .:-", "AaBbCcDdEeFfGgHh"} {
		if err := c.send(fmt.Sprintf("FONTTEXT %d %s", row, text)); err != nil {
			return err
		}
	}
	return nil
}

func calibrateCommand(opt options, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usageError("calibrate needs a command\n%s", calibrateUsage)
	}
	action, rest := args[0], args[1:]
	profilePath := profile.DefaultPath()
	// --profile PATH may come last
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--profile" && i+1 < len(rest) {
			profilePath = rest[i+1]
			rest = append(rest[:i:i], rest[i+2:]...)
			break
		}
	}
	p, err := profile.Load(profilePath)
	if err != nil {
		return err
	}

	switch action {
	case "show":
		fmt.Fprintf(stdout, "profile: %s\n%s\n", profilePath, describe(p))
		return nil
	case "forget":
		if len(rest) != 1 {
			return usageError("calibrate forget needs logo, font or contrast")
		}
		switch rest[0] {
		case "logo":
			p.Logo, p.LogoInvert = "", false
		case "font":
			p.Font = ""
		case "contrast":
			p.ContrastFull, p.ContrastDim = nil, nil
		default:
			return usageError("calibrate forget needs logo, font or contrast")
		}
		return p.Save(profilePath)
	}

	path, err := devicePath(opt, loadConfig())
	if err != nil || path == "" {
		return fmt.Errorf("calibrate needs the serial port: use --device PATH")
	}
	stopDaemon()
	dev, err := serialdev.OpenCalibration(path)
	if err != nil {
		return err
	}
	defer dev.Close()
	c := calibration{dev.Client}

	switch action {
	case "contrast":
		if len(rest) != 2 || (rest[0] != "full" && rest[0] != "dim") {
			return usageError("calibrate contrast full|dim VALUE")
		}
		v, err := strconv.ParseInt(rest[1], 0, 16)
		if err != nil || v < 0 || v > 255 {
			return fmt.Errorf("a contrast value is a number from 0 to 255 (or 0x00 to 0xFF), not %q", rest[1])
		}
		if err := c.send(fmt.Sprintf("CONTRAST %d", v)); err != nil {
			return err
		}
		n := int(v)
		if rest[0] == "full" {
			p.ContrastFull = &n
		} else {
			p.ContrastDim = &n
		}
		fmt.Fprintf(stderr, "Contrast %s is %d (0x%02X) now and is remembered for flashing.\n", rest[0], v, v)
	case "logo":
		if len(rest) < 1 {
			return usageError("calibrate logo FILE.png [--invert]")
		}
		invert := false
		for _, a := range rest[1:] {
			if a != "--invert" {
				return usageError("calibrate logo: unknown option %q", a)
			}
			invert = true
		}
		f, err := os.Open(rest[0])
		if err != nil {
			return err
		}
		bitmap, err := logo.Convert(f, invert)
		f.Close()
		if err != nil {
			return err
		}
		if err := c.bitmap(bitmap); err != nil {
			return err
		}
		if p.Logo, err = filepath.Abs(rest[0]); err != nil {
			return err
		}
		p.LogoInvert = invert
		fmt.Fprintln(stderr, "The logo is on the display and remembered for flashing.")
	case "font":
		if len(rest) != 1 {
			return usageError("calibrate font NAME|FILE.bin")
		}
		data, err := fonts.Load(rest[0])
		if err != nil {
			return err
		}
		if err := params.ValidateFont(data); err != nil {
			return err
		}
		if err := c.font(data); err != nil {
			return err
		}
		if p.Font, err = fonts.Canonical(rest[0], filepath.Abs); err != nil {
			return err
		}
		fmt.Fprintln(stderr, "The sample is on the display and the font is remembered for flashing.")
	case "rotate", "invert":
		if len(rest) != 1 || (rest[0] != "on" && rest[0] != "off") {
			return usageError("calibrate %s on|off", action)
		}
		value := 0
		if rest[0] == "on" {
			value = 1
		}
		command := "FLIP"
		if action == "invert" {
			command = "INVERT"
		}
		if err := c.send(fmt.Sprintf("%s %d", command, value)); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "The display is tried out with %s %s. This is not remembered: it is a setting of the device, change it with \"trustctl config set %s=%s\" on the core firmware.\n", action, rest[0], action, rest[0])
		return nil
	default:
		return usageError("unknown calibrate command %q\n%s", action, calibrateUsage)
	}
	return p.Save(profilePath)
}
