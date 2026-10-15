package main

import (
	"fmt"
	"io"
	"strings"

	"trustissues/trustctl/internal/profile"
)

// option describes one setting in every place where it can be given.
type option struct {
	arg    string // on the command line, e.g. "--device PATH"
	env    string // environment variable, e.g. "TRUSTCTL_DEVICE"
	config string // line of the config file, e.g. "device = PATH"
	def    string // what applies if it is not given
	text   string // what it does
}

var globalOptions = []option{
	{"--device PATH", envDevice, configKeyDevice + " = PATH",
		"none: trustctl needs a device, or --autoprobe",
		"The serial port of the device, for example /dev/ttyUSB0 or COM3. With a running daemon: the daemon holds one device, a different one needs \"trustctl daemon stop\" first."},
	{"--autoprobe", "", configKeyAutoprobe + " = true",
		"off",
		"Open every serial port that could be the device until one answers. Opening a port resets whatever Arduino is on it, so this is opt-in."},
	{"--simulate", envSimulate + "=1 (any value)", "",
		"off",
		"Use a simulated device in memory that confirms everything; nothing is stored. For trying trustctl out."},
	{"--no-daemon", envNoDaemon + "=1 (any value)", configKeyDaemon + " = false",
		"the daemon is used and started when needed",
		"Talk to the device directly instead of through the background daemon (which keeps the port open and caches the key list)."},
	{"", "", configKeyIdleTimeout + " = 5m",
		"5m",
		"How long the daemon waits without a command before it exits (a Go duration: 90s, 10m, 1h). Only the config file sets it; trustctl passes it to the daemon it starts as the internal option --idle."},
}

var commandOptions = []struct {
	command string
	options []option
}{
	{"add [NAME]", []option{
		{"--digits N", "", "", "6", "Digits of the code, 6 to 8. Only for a plain Base32 secret; an otpauth:// URI brings its own."},
		{"--period S", "", "", "30", "Seconds a code is valid, 30 or 60. Only for a plain Base32 secret; an otpauth:// URI brings its own."},
	}},
	{"flash FILE.hex", []option{
		{"--no-patch", "", "", "off", "Flash the hex file as it is, without the font, logo and contrast of the profile."},
		{"--profile PATH", "", "", "the device profile (see below)", "The profile file to patch into the firmware."},
		{"--wait DURATION", "", "", "90s", "How long to wait for the bootloader to open (the button gesture)."},
		{"--countdown N", "", "", "4", "Seconds between \"hold the button\" and the reset of the device."},
	}},
	{"calibrate logo FILE.png", []option{
		{"--invert", "", "", "off", "Swap lit and dark pixels, including the margin around a smaller image."},
	}},
	{"font extract / logo extract", []option{
		{"--from FILE.hex", "", "", "the profile, else the built-in font", "Take the font or logo from the parameter block of a firmware file."},
		{"--profile PATH", "", "", "the device profile (see below)", "The profile file the current font or logo is read from."},
	}},
	{"calibrate ...", []option{
		{"--profile PATH", "", "", "the device profile (see below)", "The profile file that the settings are remembered in."},
	}},
}

var profileKeys = []option{
	{"calibrate contrast full VALUE", "", `"contrast_full": 32`, "0x20", "Contrast of the display at full brightness, 0 to 255."},
	{"calibrate contrast dim VALUE", "", `"contrast_dim": 0`, "0x00", "Contrast of the dimmed logo, 0 to 255."},
	{"calibrate logo FILE.png", "", `"logo": "/path/logo.png"`, "no logo", "The idle logo, at most 128x32 pixels."},
	{"calibrate logo FILE.png --invert", "", `"logo_invert": true`, "false", "Whether the logo is inverted."},
	{"calibrate font FILE.bin", "", `"font": "/path/font.bin"`, "the built-in Victoria font", "A 772 byte 8x8 font in U8x8 format."},
}

var deviceSettings = []option{
	{"config set rotate=on|off", "", "", "off", "Turn the display by 180 degrees."},
	{"config set invert=on|off", "", "", "off", "Swap lit and dark pixels of the display."},
	{"config set logo=on|off", "", "", "off", "Show the idle logo, and for 5 s after power-up."},
	{"config set autowipe=on|off", "", "", "off", "Allow wiping all keys with the button alone (a long hold at idle, then a gesture)."},
}

// wrap breaks text into lines of at most width characters, each starting with indent.
func wrap(text, indent string, width int) string {
	var b strings.Builder
	line := indent
	for _, word := range strings.Fields(text) {
		if len(line) > len(indent) && len(line)+1+len(word) > width {
			b.WriteString(line + "\n")
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	return b.String() + line + "\n"
}

func printOption(w io.Writer, o option, indent, configLabel string) {
	arg := o.arg
	if arg == "" {
		arg = "(no command line option)"
	}
	fmt.Fprintf(w, "%s%s\n", indent, arg)
	fmt.Fprint(w, wrap(o.text, indent+"    ", 78))
	if o.env != "" {
		fmt.Fprintf(w, "%s    environment: %s\n", indent, o.env)
	}
	if o.config != "" {
		fmt.Fprintf(w, "%s    %s %s\n", indent, configLabel, o.config)
	}
	if o.def != "" {
		fmt.Fprintf(w, "%s    default:     %s\n", indent, o.def)
	}
}

// printOptions is the text of "trustctl help-options".
func printOptions(w io.Writer) {
	fmt.Fprintln(w, "Options of trustctl")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global options, before the command (trustctl --device /dev/ttyUSB0 list).")
	fmt.Fprintln(w, "A value comes from the first of these that is set: the command line, the")
	fmt.Fprintln(w, "environment, the config file, the default.")
	fmt.Fprintln(w)
	for _, o := range globalOptions {
		printOption(w, o, "  ", "config file:")
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "The config file is %s\n", configPath())
	fmt.Fprintln(w, "(or $XDG_CONFIG_HOME/trustctl/config). One \"key = value\" per line, \"#\" starts a comment:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    # my device")
	fmt.Fprintf(w, "    %s = /dev/ttyUSB0\n", configKeyDevice)
	fmt.Fprintf(w, "    %s = 10m\n", configKeyIdleTimeout)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options of single commands:")
	for _, c := range commandOptions {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  trustctl %s\n", c.command)
		for _, o := range c.options {
			printOption(w, o, "    ", "config file:")
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Look and feel of the display, kept by \"trustctl calibrate\" and patched into the firmware by")
	fmt.Fprintf(w, "\"trustctl flash\". The profile is %s\n", profile.DefaultPath())
	fmt.Fprintln(w, "(JSON; you can edit it by hand):")
	for _, o := range profileKeys {
		fmt.Fprintln(w)
		printOption(w, o, "  ", "profile key:")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Settings that live on the device itself (they survive a firmware update; each change is")
	fmt.Fprintln(w, "confirmed with the button; \"trustctl config\" shows them):")
	for _, o := range deviceSettings {
		fmt.Fprintln(w)
		printOption(w, o, "  ", "config file:")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Other environment variables that are read: XDG_CONFIG_HOME (where the config file and the")
	fmt.Fprintln(w, "profile live), XDG_RUNTIME_DIR (the daemon's socket and log, else a private directory under")
	fmt.Fprintln(w, "the temporary directory), HOME.")
}
