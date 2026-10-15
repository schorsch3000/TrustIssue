// trustctl talks to a Trust Issues device: a TOTP vault that only hands out
// codes while somebody holds its button.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/serialdev"
	"trustissues/trustctl/internal/sim"
)

const usage = `usage: trustctl [options] <command> [arguments]

commands:
  list                      show the keys on the device
  token NAME                print the current code (confirm on the device)
  add NAME                  store a key; the secret or an otpauth:// URI is read from stdin
        --digits N --period S   only if the secret is given as plain Base32 (default 6, 30)
  del NAME                  delete a key (confirm on the device)
  tz [get|sync|set +H[:MM]] show or change the device's timezone offset
  config [get|set FLAG=on|off ...]   flags: rotate invert logo autowipe
  wipe                      erase all keys (hold the button 5 to 6 seconds)
  hello                     show what the device says about itself

NAME is a key name (any case), its slot number, or a unique beginning of a name.

options:
  --device PATH             serial port of the device (or TRUSTCTL_DEVICE, or "device =" in the config file)
  --autoprobe               try all serial ports; this resets every board it opens
  --simulate                use a simulated device that confirms everything
`

// exit codes
const (
	exitOK       = 0
	exitFailure  = 1
	exitRefused  = 2 // denied or timed out on the device
	exitUsage    = 64
	exitNoDevice = 69
)

type options struct {
	device    string
	autoprobe bool
	simulate  bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("trustctl", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	var opt options
	global.StringVar(&opt.device, "device", "", "")
	global.BoolVar(&opt.autoprobe, "autoprobe", false, "")
	global.BoolVar(&opt.simulate, "simulate", false, "")
	if err := global.Parse(args); err != nil || global.NArg() == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	command, rest := global.Arg(0), global.Args()[1:]

	client, closeDevice, err := connect(opt, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "trustctl:", err)
		return exitNoDevice
	}
	defer closeDevice()

	hint := "Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels."
	client.OnPending = func() { fmt.Fprintln(stderr, hint) }

	err = dispatch(client, command, rest, stdout, stderr, func(h string) { hint = h })
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, errUsage):
		fmt.Fprintln(stderr, "trustctl:", strings.TrimPrefix(err.Error(), errUsage.Error()+": "))
		fmt.Fprint(stderr, usage)
		return exitUsage
	case errors.Is(err, protocol.ErrDenied), errors.Is(err, protocol.ErrTimeout):
		fmt.Fprintln(stderr, "trustctl:", err)
		return exitRefused
	}
	fmt.Fprintln(stderr, "trustctl:", err)
	return exitFailure
}

var errUsage = errors.New("usage")

func usageError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

func connect(opt options, stderr io.Writer) (*protocol.Client, func(), error) {
	if opt.simulate || os.Getenv("TRUSTCTL_SIMULATE") != "" {
		fmt.Fprintln(stderr, "(simulated device, every confirmation is accepted)")
		conn, _ := sim.New(sim.AcceptAll)
		client := protocol.NewClient(conn)
		if _, err := client.WaitBanner(time.Second); err != nil {
			return nil, nil, err
		}
		return client, func() { conn.Close() }, nil
	}
	cfg := loadConfig()
	path := firstNonEmpty(opt.device, os.Getenv("TRUSTCTL_DEVICE"), cfg.device)
	var dev *serialdev.Device
	var err error
	switch {
	case path != "":
		dev, err = serialdev.Open(path, 3*time.Second)
	case opt.autoprobe || cfg.autoprobe:
		dev, err = serialdev.Probe()
	default:
		return nil, nil, errors.New("no device given: use --device PATH, TRUSTCTL_DEVICE, \"device = PATH\" in " + configPath() + ", or --autoprobe")
	}
	if err != nil {
		return nil, nil, err
	}
	return dev.Client, func() { dev.Close() }, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func dispatch(c *protocol.Client, command string, args []string, stdout, stderr io.Writer, setHint func(string)) error {
	switch command {
	case "hello":
		line, err := c.Hello()
		if err == nil {
			fmt.Fprintln(stdout, line)
		}
		return err
	case "list":
		keys, err := c.List()
		if err != nil {
			return err
		}
		for _, k := range keys {
			fmt.Fprintf(stdout, "%2d  %s\n", k.ID, k.Name)
		}
		return nil
	case "token":
		if len(args) != 1 {
			return usageError("token needs exactly one NAME")
		}
		key, err := find(c, args[0])
		if err != nil {
			return err
		}
		if err := syncTimezone(c, stderr); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "Requesting the code for %q.\n", key.Name)
		code, err := c.Token(key.ID, time.Now().Unix())
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, code)
		return nil
	case "add":
		return add(c, args, stdout, stderr)
	case "del":
		if len(args) != 1 {
			return usageError("del needs exactly one NAME")
		}
		key, err := find(c, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "Deleting %q: check the name on the display.\n", key.Name)
		return c.Delete(key.ID)
	case "wipe":
		setHint("Confirm on the device: hold the button for 5 to 6 seconds, then let go. Any other time cancels.")
		fmt.Fprintln(stderr, "Erasing ALL keys on the device.")
		return c.Wipe()
	case "tz":
		return timezone(c, args, stdout, stderr)
	case "config":
		return configure(c, args, stdout)
	}
	return usageError("unknown command %q", command)
}

func find(c *protocol.Client, arg string) (protocol.Key, error) {
	keys, err := c.List()
	if err != nil {
		return protocol.Key{}, err
	}
	return resolve(keys, arg)
}

// ---------------------------------------------------------------- add

func add(c *protocol.Client, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("add", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	digits := set.Int("digits", 6, "")
	period := set.Int("period", 30, "")
	// the name comes first: trustctl add "Work VPN" --digits 8
	if len(args) == 0 {
		return usageError("add needs a NAME")
	}
	name := args[0]
	if err := set.Parse(args[1:]); err != nil || set.NArg() != 0 {
		return usageError("add: bad options")
	}
	if err := protocol.ValidateName(name); err != nil {
		return err
	}
	text, err := readSecret(stderr)
	if err != nil {
		return err
	}
	secret := text
	if strings.HasPrefix(strings.ToLower(text), "otpauth:") {
		uri, err := protocol.ParseOTPAuth(text)
		if err != nil {
			return err
		}
		secret, *digits, *period = uri.Secret, uri.Digits, uri.Period
	}
	if *digits < 6 || *digits > 8 {
		return fmt.Errorf("the device supports 6 to 8 digits, not %d", *digits)
	}
	if *period != 30 && *period != 60 {
		return fmt.Errorf("the device supports a period of 30 or 60 seconds, not %d", *period)
	}
	fmt.Fprintf(stderr, "Adding %q (%d digits, %d s): check the name on the display.\n", name, *digits, *period)
	id, err := c.Add(name, secret, *digits, *period)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "stored as slot %d\n", id)
	return nil
}

// readSecret reads the secret from stdin without echo on a terminal. It is
// never taken from the command line, where other users could see it.
func readSecret(stderr io.Writer) (string, error) {
	var line string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, "Secret (Base32) or otpauth:// URI, input is hidden: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		line = string(b)
	} else {
		l, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && l == "" {
			return "", fmt.Errorf("no secret on stdin")
		}
		line = l
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("the secret is empty")
	}
	return line, nil
}

// ---------------------------------------------------------------- timezone

func hostOffsetQuarters(t time.Time) (int, error) {
	_, seconds := t.Zone()
	if seconds%900 != 0 {
		return 0, fmt.Errorf("this host's UTC offset (%d s) is not a multiple of 15 minutes", seconds)
	}
	return seconds / 900, nil
}

func formatOffset(quarters int) string {
	sign := "+"
	if quarters < 0 {
		sign, quarters = "-", -quarters
	}
	if quarters%4 == 0 {
		return fmt.Sprintf("UTC%s%d", sign, quarters/4)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, quarters/4, quarters%4*15)
}

func parseOffset(s string) (int, error) {
	sign := 1
	switch {
	case strings.HasPrefix(s, "-"):
		sign, s = -1, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	hours, minutes, _ := strings.Cut(s, ":")
	h, err := strconv.Atoi(hours)
	m := 0
	if minutes != "" {
		m, err = strconv.Atoi(minutes)
	}
	if err != nil || h < 0 || m < 0 || m%15 != 0 || m >= 60 {
		return 0, fmt.Errorf("cannot read %q as an offset like +2, -9:30 or +5:45", s)
	}
	q := sign * (h*4 + m/15)
	if q < -48 || q > 56 {
		return 0, fmt.Errorf("the offset must be between UTC-12 and UTC+14")
	}
	return q, nil
}

// syncTimezone makes the device's offset match this host's, asking for a
// confirmation on the device if it has to change (daylight saving, travel).
func syncTimezone(c *protocol.Client, stderr io.Writer) error {
	host, err := hostOffsetQuarters(time.Now())
	if err != nil {
		return err
	}
	device, err := c.Timezone()
	if err != nil {
		return err
	}
	if device == host {
		return nil
	}
	fmt.Fprintf(stderr, "The device is set to %s but this host is at %s; setting the device's offset.\n", formatOffset(device), formatOffset(host))
	return c.SetTimezone(host)
}

func timezone(c *protocol.Client, args []string, stdout, stderr io.Writer) error {
	host, err := hostOffsetQuarters(time.Now())
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "get" {
		device, err := c.Timezone()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "device: %s\nhost:   %s\n", formatOffset(device), formatOffset(host))
		return nil
	}
	switch args[0] {
	case "sync":
		return syncTimezone(c, stderr)
	case "set":
		if len(args) != 2 {
			return usageError("tz set needs an offset like +2 or -9:30")
		}
		q, err := parseOffset(args[1])
		if err != nil {
			return err
		}
		return c.SetTimezone(q)
	}
	return usageError("unknown tz command %q", args[0])
}

// ---------------------------------------------------------------- config

var configFlags = []struct {
	name string
	bit  uint8
}{{"rotate", 0x01}, {"invert", 0x02}, {"logo", 0x04}, {"autowipe", 0x08}}

func configure(c *protocol.Client, args []string, stdout io.Writer) error {
	flags, err := c.Config()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "get" {
		for _, f := range configFlags {
			state := "off"
			if flags&f.bit != 0 {
				state = "on"
			}
			fmt.Fprintf(stdout, "%-9s %s\n", f.name, state)
		}
		return nil
	}
	if args[0] != "set" || len(args) < 2 {
		return usageError("config set needs FLAG=on|off, for example: config set rotate=on invert=off")
	}
	for _, change := range args[1:] {
		name, value, ok := strings.Cut(change, "=")
		bit := uint8(0)
		for _, f := range configFlags {
			if f.name == name {
				bit = f.bit
			}
		}
		if !ok || bit == 0 || (value != "on" && value != "off") {
			return usageError("cannot read %q, expected FLAG=on|off with FLAG one of rotate invert logo autowipe", change)
		}
		if value == "on" {
			flags |= bit
		} else {
			flags &^= bit
		}
	}
	return c.SetConfig(flags)
}
