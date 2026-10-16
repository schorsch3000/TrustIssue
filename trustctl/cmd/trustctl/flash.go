package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
	"trustissues/trustctl/internal/daemon"
	"trustissues/trustctl/internal/firmware"
	"trustissues/trustctl/internal/flasher"
	"trustissues/trustctl/internal/fonts"
	"trustissues/trustctl/internal/hexfile"
	"trustissues/trustctl/internal/params"
	"trustissues/trustctl/internal/profile"
	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/serialdev"
)

// stopDaemon asks a running daemon to let go of the serial port.
func stopDaemon() {
	conn, err := daemon.Dial()
	if err != nil {
		return
	}
	client := protocol.NewClient(conn)
	if _, err := client.WaitBanner(2 * time.Second); err == nil {
		client.Raw("!STOP", func(string) {})
	}
	conn.Close()
	for i := 0; i < 30; i++ { // wait until the socket is gone
		if c, err := daemon.Dial(); err != nil {
			return
		} else {
			c.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func flashCommand(opt options, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("flash", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	profilePath := set.String("profile", profile.DefaultPath(), "")
	noPatch := set.Bool("no-patch", false, "")
	wait := set.Duration("wait", 90*time.Second, "")
	countdown := set.Int("countdown", 4, "")
	fontSpec := set.String("font", "", "")
	setPin := set.Bool("set-pin", false, "")
	clearPin := set.Bool("clear-pin", false, "")
	askPin := set.Bool("pin", false, "")
	// what to flash comes first: trustctl flash [core|calibration|pin|FILE.hex] --no-patch;
	// without it the built-in core firmware is flashed
	what := firmware.Core
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		what, args = args[0], args[1:]
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 {
		return usageError("flash: bad options")
	}
	path, err := devicePath(opt, loadConfig())
	if err != nil || path == "" {
		return fmt.Errorf("flash needs the serial port: use --device PATH (probing would reset other boards)")
	}

	if *setPin && *clearPin {
		return usageError("flash: --set-pin and --clear-pin exclude each other")
	}
	if what == "pin" && !*setPin && !*clearPin {
		return usageError("flash pin: needs --set-pin or --clear-pin")
	}

	// the built-in names win, unless the argument looks like a path
	var img *hexfile.Image
	var builtin bool
	if what == "pin" { // only the PIN page, the firmware stays as it is
		img, *noPatch = newImage(), true
		fmt.Fprintln(stderr, "Changing only the update PIN, the firmware stays as it is.")
	} else {
		img, builtin, err = firmware.Image(what)
	}
	switch {
	case what == "pin":
	case err != nil:
		return fmt.Errorf("built-in %s firmware: %w", what, err)
	case builtin:
		fmt.Fprintf(stderr, "Flashing the built-in %s firmware %s.\n", what, firmware.Version())
		if what == firmware.Calibration {
			*noPatch = true // it has no parameter block
		}
	default:
		f, err := os.Open(what)
		if err != nil {
			return fmt.Errorf("%w\n(\"flash\" takes core, calibration or the path of a .hex file)", err)
		}
		img, err = hexfile.Parse(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	if img.Highest() >= flasher.PinPage {
		return fmt.Errorf("%s writes into the end of the flash (from %#x): the bootloader's section and the update PIN's page are not available to firmware", what, flasher.PinPage)
	}
	if *setPin {
		pin, err := askNewPIN(stderr)
		if err != nil {
			return err
		}
		img.Write(flasher.PinPage, flasher.PinPageData(pin))
	} else if *clearPin {
		img.Write(flasher.PinPage, flasher.PinPageData(""))
		fmt.Fprintln(stderr, "The update PIN will be removed: the gesture alone opens the bootloader again.")
	}
	if *noPatch && *fontSpec != "" {
		return usageError("flash: --font and --no-patch exclude each other")
	}
	if !*noPatch {
		patch, p, err := flashPatch(*profilePath, *fontSpec)
		if err != nil {
			return err
		}
		if !patch.Empty() {
			if err := params.Apply(img, patch); err != nil {
				return fmt.Errorf("%w (use --no-patch to flash it unchanged)", err)
			}
			fmt.Fprintf(stderr, "Patched from %s: %s\n", *profilePath, describe(p))
		}
	}
	pages := img.Pages(flasher.PageSize)

	stopDaemon()
	port, err := serialdev.OpenBootloaderPort(path)
	if err != nil {
		return err
	}
	defer port.Close()

	havePin := ""
	if *askPin { // the bootloader waits only 8 s for the PIN, so ask before the gesture
		if havePin, err = readHidden("Current update PIN: ", stderr); err != nil {
			return err
		}
		if err := flasher.ValidatePIN(havePin); err != nil {
			return err
		}
	}
	fmt.Fprintf(stderr, "Flashing %d pages (%d bytes) through the bootloader of %s.\n", len(pages), len(pages)*flasher.PageSize, path)
	fmt.Fprintln(stderr, "HOLD THE BUTTON on the device and keep holding it.")
	for n := *countdown; n > 0; n-- {
		fmt.Fprintf(stderr, "Resetting the device in %d...\n", n)
		time.Sleep(time.Second)
	}
	fmt.Fprintln(stderr, "Resetting the device now.")
	if err := serialdev.Reset(port); err != nil {
		return err
	}
	fmt.Fprintln(stderr, "Follow the display: release the button when it says RELEASE BUTTON, wait for the bar to run out,")
	fmt.Fprintln(stderr, "then tap, tap, and press a third time and hold until the bar is full.")

	programmer := flasher.New(port)
	asked := false
	err = programmer.WaitForBootloader(*wait, nil, func() (string, error) {
		asked = true
		if havePin != "" {
			return havePin, nil
		}
		fmt.Fprintln(stderr, "The device asks for its update PIN. It waits only 8 s for it: type quickly,")
		fmt.Fprintln(stderr, "or use --pin to be asked before the gesture.")
		return readHidden("Update PIN: ", stderr)
	})
	if err != nil {
		return err
	}
	if asked {
		fmt.Fprintln(stderr, "The PIN was accepted.")
	}
	fmt.Fprintln(stderr, "The bootloader is open.")
	last := ""
	err = programmer.Write(pages, func(phase string, done, total int) {
		if phase != last || done == total {
			fmt.Fprintf(stderr, "%s %d/%d pages\n", phase, done, total)
			last = phase
		}
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Done: the device restarts into the new firmware.")
	if *fontSpec != "" { // the font on the device is now this one, so the profile says so too
		stored, err := profile.Load(*profilePath)
		if err == nil {
			if stored.Font, err = fonts.Canonical(*fontSpec, filepath.Abs); err == nil {
				err = stored.Save(*profilePath)
			}
		}
		if err != nil {
			return fmt.Errorf("the device is flashed, but the profile %s could not remember the font: %w", *profilePath, err)
		}
		fmt.Fprintf(stderr, "The profile %s remembers the font %s.\n", *profilePath, stored.Font)
	}
	return nil
}

// newImage is an empty flash image (every byte 0xFF and undefined).
func newImage() *hexfile.Image {
	img := &hexfile.Image{}
	for i := range img.Data {
		img.Data[i] = 0xFF
	}
	return img
}

// askNewPIN asks twice for the PIN that will be written to the device.
func askNewPIN(stderr io.Writer) (string, error) {
	fmt.Fprintf(stderr, "The update PIN guards the bootloader after the button gesture: %d to %d characters,\nasked for only when flashing. Without it the device cannot be reflashed except over ISP,\nwhich erases the keys.\n", flasher.MinPIN, flasher.MaxPIN)
	pin, err := readHidden("New update PIN: ", stderr)
	if err != nil {
		return "", err
	}
	if err := flasher.ValidatePIN(pin); err != nil {
		return "", err
	}
	again, err := readHidden("Again: ", stderr)
	if err != nil {
		return "", err
	}
	if again != pin {
		return "", errors.New("the two entries differ, nothing was changed")
	}
	return pin, nil
}

// readHidden reads one line without showing it: stars on a terminal, a plain line from a pipe.
func readHidden(prompt string, stderr io.Writer) (string, error) {
	in := int(os.Stdin.Fd())
	var line string
	switch {
	case term.IsTerminal(in) && term.IsTerminal(int(os.Stderr.Fd())):
		var err error
		if line, err = readMasked(in, prompt); err != nil {
			return "", err
		}
	case term.IsTerminal(in):
		fmt.Fprint(stderr, prompt)
		b, err := term.ReadPassword(in)
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		line = string(b)
	default:
		l, err := stdinReader().ReadString('\n')
		if err != nil && l == "" {
			return "", errors.New("no PIN on stdin")
		}
		line = l
	}
	return strings.TrimRight(line, "\r\n"), nil
}

var sharedStdin *bufio.Reader

// stdinReader is one reader for all prompts, so a pipe with two lines (PIN and repeat) works.
func stdinReader() *bufio.Reader {
	if sharedStdin == nil {
		sharedStdin = bufio.NewReader(os.Stdin)
	}
	return sharedStdin
}

func describe(p profile.Profile) string {
	var parts []string
	if p.ContrastFull != nil {
		parts = append(parts, fmt.Sprintf("contrast full %d", *p.ContrastFull))
	}
	if p.ContrastDim != nil {
		parts = append(parts, fmt.Sprintf("dim %d", *p.ContrastDim))
	}
	if p.Logo != "" {
		parts = append(parts, "logo "+p.Logo)
	}
	if p.Font != "" {
		parts = append(parts, "font "+p.Font)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	text := parts[0]
	for _, part := range parts[1:] {
		text += ", " + part
	}
	return text
}

// flashPatch reads the profile and builds what "flash" patches into the firmware. A font given
// with --font replaces the profile's, which then need not even exist. The returned profile only
// serves for the message that says what was patched.
func flashPatch(profilePath, fontSpec string) (params.Patch, profile.Profile, error) {
	p, err := profile.Load(profilePath)
	if err != nil {
		return params.Patch{}, p, err
	}
	if fontSpec != "" {
		p.Font = ""
	}
	patch, err := p.Patch()
	if err != nil {
		if p.Font != "" && errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%w\n(the profile %s names this font; use --font to flash another one, or \"trustctl calibrate forget font\")", err, profilePath)
		}
		return patch, p, err
	}
	if fontSpec != "" {
		if patch.Font, err = fonts.Load(fontSpec); err != nil {
			return patch, p, err
		}
		if err := params.ValidateFont(patch.Font); err != nil {
			return patch, p, fmt.Errorf("font %s: %w", fontSpec, err)
		}
		p.Font = fontSpec
	}
	return patch, p, nil
}
