package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"trustissues/trustctl/internal/daemon"
	"trustissues/trustctl/internal/firmware"
	"trustissues/trustctl/internal/flasher"
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
	// what to flash comes first: trustctl flash [core|calibration|FILE.hex] --no-patch;
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

	// the built-in names win, unless the argument looks like a path
	img, builtin, err := firmware.Image(what)
	switch {
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
	if img.Highest() >= flasher.BootStart {
		return fmt.Errorf("%s writes into the bootloader's section (from %#x)", what, flasher.BootStart)
	}
	if !*noPatch {
		p, err := profile.Load(*profilePath)
		if err != nil {
			return err
		}
		patch, err := p.Patch()
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
	if err := programmer.WaitForBootloader(*wait, nil); err != nil {
		return err
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
	return nil
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
