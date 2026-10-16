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

	"trustissues/trustctl/internal/daemon"
	"trustissues/trustctl/internal/firmware"
	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/serialdev"
	"trustissues/trustctl/internal/sim"
)

// version is set at release time: -ldflags "-X main.version=..."
var version = "dev"

const usage = `usage: trustctl [options] <command> [arguments]

commands:
  list                      show the keys on the device
  token NAME                print the current code (confirm on the device)
  add [NAME]                store a key; the secret or an otpauth:// URI is read from stdin.
                            A URI supplies the name if none is given (issuer and account, e-mail
                            characters the device cannot show become "-")
        --digits N --period S   only if the secret is given as plain Base32 (default 6, 30)
  del NAME                  delete a key (confirm on the device)
  tz [get|sync|set +H[:MM]] show or change the device's timezone offset
  config [get|set FLAG=on|off ...]   flags: rotate invert logo autowipe
  wipe                      erase all keys (hold the button 5 to 6 seconds)
  hello                     show what the device says about itself
  daemon [status|stop]      the background process that keeps the port open (started by the first command)
  version                   the versions of trustctl and of the firmware built into it
  help-options              every option with its --argument, environment variable and config file syntax
  autocomplete              print the completion code for bash and sh: source <(trustctl autocomplete)
  autocomplete-zsh          the same for zsh
  autocomplete-fish         the same for fish: trustctl autocomplete-fish | source
  flash [core|calibration]  program the firmware that is built into trustctl through the bootloader
                            (needs the button gesture, and the update PIN if one is set); core is the default
  flash pin --set-pin       only set the update PIN (--clear-pin removes it); the firmware stays as it is
  flash FILE.hex            the same with a firmware file of your own (for development)
        --no-patch --profile PATH --font NAME|FILE.bin --pin --set-pin --clear-pin --wait 90s --countdown 4
  font list                 the fonts built into trustctl
  font extract FILE.bin     write the current font to a file to work on (--from FILE.hex, or --name NAME)
  logo extract FILE.png     write the current logo as a 128x32 PNG to work on
  calibrate ...             try out contrast, logo, font, rotation and inversion on the calibration firmware (see: trustctl calibrate)

NAME is a key name (any case), its slot number, or a unique beginning of a name.

options (before or after the command):
  --device PATH             serial port of the device (or TRUSTCTL_DEVICE, or "device =" in the config file)
  --autoprobe               try all serial ports; this resets every board it opens
  --simulate                use a simulated device that confirms everything
  --no-daemon               talk to the device directly (TRUSTCTL_NO_DAEMON=1, or "daemon = false" in the config file)
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
	noDaemon  bool
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
	global.BoolVar(&opt.noDaemon, "no-daemon", false, "")
	idle := global.Duration("idle", 5*time.Minute, "") // only for __daemon
	if err := global.Parse(args); err != nil || global.NArg() == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	command, rest := global.Arg(0), global.Args()[1:]
	rest = takeGlobalOptions(rest, &opt) // they may also stand after the command
	switch command {
	case "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "version", "--version":
		fmt.Fprintf(stdout, "trustctl %s, built-in firmware %s\n", version, firmware.Version())
		return exitOK
	case "help-options":
		printOptions(stdout)
		return exitOK
	case "autocomplete", "autocomplete-bash", "autocomplete-sh":
		fmt.Fprint(stdout, completionScript("bash"))
		return exitOK
	case "autocomplete-zsh":
		fmt.Fprint(stdout, completionScript("zsh"))
		return exitOK
	case "autocomplete-fish":
		fmt.Fprint(stdout, completionScript("fish"))
		return exitOK
	case "__keys": // for the completion scripts: the key names, one per line, never an error
		return keyNames(opt, stdout)
	case "__daemon":
		return runDaemon(opt, *idle, stderr)
	case "daemon":
		return daemonCommand(rest, stdout, stderr)
	case "font", "logo":
		return report(extractCommand(command, rest, stdout, stderr), stderr)
	case "flash", "calibrate":
		var err error
		if command == "flash" {
			err = flashCommand(opt, rest, stdout, stderr)
		} else {
			err = calibrateCommand(opt, rest, stdout, stderr)
		}
		return report(err, stderr)
	}

	client, closeDevice, err := connect(opt, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "trustctl:", err)
		return exitNoDevice
	}
	defer closeDevice()

	hint := "Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels."
	client.OnPending = func() { fmt.Fprintln(stderr, hint) }

	err = dispatch(client, command, rest, stdout, stderr, func(h string) { hint = h })
	return report(err, stderr)
}

// keyNames prints the names of the keys on the device; every problem is silent,
// because a completion must not print error messages into the command line.
func keyNames(opt options, stdout io.Writer) int {
	client, closeDevice, err := connect(opt, io.Discard)
	if err != nil {
		return exitOK
	}
	defer closeDevice()
	keys, err := client.List()
	if err != nil {
		return exitOK
	}
	for _, k := range keys {
		fmt.Fprintln(stdout, k.Name)
	}
	return exitOK
}

// report prints an error and returns the exit code for it.
func report(err error, stderr io.Writer) int {
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

// takeGlobalOptions moves the global options (--device PATH, --device=PATH, --autoprobe,
// --simulate, --no-daemon, with one or two dashes) out of the arguments that follow the
// command into opt, so that "trustctl calibrate --device /dev/ttyUSB0 logo x.png" works
// as well as "trustctl --device /dev/ttyUSB0 calibrate logo x.png".
func takeGlobalOptions(args []string, opt *options) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		name := strings.TrimLeft(args[i], "-")
		dashes := len(args[i]) - len(name)
		switch {
		case dashes < 1 || dashes > 2:
			rest = append(rest, args[i])
		case name == "autoprobe":
			opt.autoprobe = true
		case name == "simulate":
			opt.simulate = true
		case name == "no-daemon":
			opt.noDaemon = true
		case name == "device" && i+1 < len(args):
			opt.device = args[i+1]
			i++
		case strings.HasPrefix(name, "device="):
			opt.device = strings.TrimPrefix(name, "device=")
		default:
			rest = append(rest, args[i])
		}
	}
	return rest
}

// devicePath resolves which serial port to use ("" means: probe).
func devicePath(opt options, cfg config) (string, error) {
	path := firstNonEmpty(opt.device, os.Getenv(envDevice), cfg.device)
	if path == "" && !opt.autoprobe && !cfg.autoprobe {
		return "", errors.New("no device given: use --device PATH, TRUSTCTL_DEVICE, \"device = PATH\" in " + configPath() + ", or --autoprobe")
	}
	return path, nil
}

func openDevice(path string, probe bool) (*serialdev.Device, error) {
	if path != "" {
		return serialdev.Open(path, 3*time.Second)
	}
	if probe {
		return serialdev.Probe()
	}
	return nil, errors.New("no device")
}

func connect(opt options, stderr io.Writer) (*protocol.Client, func(), error) {
	if opt.simulate || os.Getenv(envSimulate) != "" {
		fmt.Fprintln(stderr, "(simulated device, every confirmation is accepted)")
		conn, _ := sim.New(sim.AcceptAll)
		client := protocol.NewClient(conn)
		if _, err := client.WaitBanner(time.Second); err != nil {
			return nil, nil, err
		}
		return client, func() { conn.Close() }, nil
	}
	cfg := loadConfig()
	path, err := devicePath(opt, cfg)
	if err != nil {
		return nil, nil, err
	}
	probe := opt.autoprobe || cfg.autoprobe
	if opt.noDaemon || cfg.noDaemon || os.Getenv(envNoDaemon) != "" {
		dev, err := openDevice(path, probe)
		if err != nil {
			return nil, nil, err
		}
		return dev.Client, func() { dev.Close() }, nil
	}
	return connectDaemon(path, probe, cfg)
}

// connectDaemon talks to the background daemon and starts it if it is not running.
func connectDaemon(path string, probe bool, cfg config) (*protocol.Client, func(), error) {
	conn, err := daemon.Dial()
	if err != nil {
		args := []string{"--idle", cfg.idleTimeout.String()}
		if path != "" {
			args = append(args, "--device", path)
		} else if probe {
			args = append(args, "--autoprobe")
		}
		if conn, err = daemon.Start(args); err != nil {
			return nil, nil, err
		}
	}
	client := protocol.NewClient(conn)
	if _, err := client.WaitBanner(3 * time.Second); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("the daemon does not answer: %w", err)
	}
	if path != "" { // a running daemon may hold a different device
		var status string
		client.Raw("!STATUS", func(l string) { status = l })
		if running := statusDevice(status); running != "" && running != path {
			conn.Close()
			return nil, nil, fmt.Errorf("the daemon already holds %s, not %s: stop it first with: trustctl daemon stop", running, path)
		}
	}
	return client, func() { conn.Close() }, nil
}

// statusDevice picks the device path out of a daemon status line.
func statusDevice(status string) string {
	for _, field := range strings.Fields(status) {
		if path, ok := strings.CutPrefix(field, "device="); ok {
			return path
		}
	}
	return ""
}

func runDaemon(opt options, idle time.Duration, stderr io.Writer) int {
	path, probe := opt.device, opt.autoprobe
	dev, err := openDevice(path, probe)
	if err != nil {
		fmt.Fprintln(stderr, "trustctl daemon:", err)
		return exitNoDevice
	}
	defer dev.Close()
	socket, err := daemon.SocketPath()
	if err != nil {
		fmt.Fprintln(stderr, "trustctl daemon:", err)
		return exitFailure
	}
	logf := func(format string, args ...any) {
		fmt.Fprintf(stderr, time.Now().Format("15:04:05 ")+format+"\n", args...)
	}
	logf("serving %s on %s", dev.Path, socket)
	reason, err := daemon.Run(dev.Client, daemon.Options{
		SocketPath: socket, Idle: idle, Banner: dev.Banner, DevicePath: dev.Path, Logf: logf,
	})
	if err != nil {
		fmt.Fprintln(stderr, "trustctl daemon:", err)
		return exitFailure
	}
	logf("ended: %s", reason)
	return exitOK
}

func daemonCommand(args []string, stdout, stderr io.Writer) int {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	conn, err := daemon.Dial()
	if err != nil {
		fmt.Fprintln(stdout, "no daemon is running")
		return exitOK
	}
	defer conn.Close()
	client := protocol.NewClient(conn)
	if _, err := client.WaitBanner(2 * time.Second); err != nil {
		fmt.Fprintln(stderr, "trustctl:", err)
		return exitFailure
	}
	command := map[string]string{"status": "!STATUS", "stop": "!STOP"}[action]
	if command == "" {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	client.Raw(command, func(l string) { fmt.Fprintln(stdout, l) })
	return exitOK
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
			warnIfUnlocked(line, stderr)
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

// addPlan is what `trustctl add` is going to send to the device.
type addPlan struct {
	name, secret    string
	digits, period  int
	derivedFromURI  bool // the name came from the otpauth label
	nameWasAdjusted bool // and some characters had to be replaced
}

// planAdd works out the key to add from the arguments (an optional NAME and
// --digits/--period) and the text read from stdin: a Base32 secret or an
// otpauth:// URI. Without a NAME, a URI supplies it.
func planAdd(args []string, input string) (addPlan, error) {
	var plan addPlan
	set := flag.NewFlagSet("add", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	digits := set.Int("digits", 6, "")
	period := set.Int("period", 30, "")
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		plan.name, args = args[0], args[1:]
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 {
		return plan, usageError("add: bad options")
	}
	plan.secret, plan.digits, plan.period = input, *digits, *period
	var uri *protocol.OTPAuth
	if strings.HasPrefix(strings.ToLower(input), "otpauth:") {
		var err error
		if uri, err = protocol.ParseOTPAuth(input); err != nil {
			return plan, err
		}
		plan.secret, plan.digits, plan.period = uri.Secret, uri.Digits, uri.Period
	}
	if plan.name == "" {
		if uri == nil {
			return plan, usageError("add needs a NAME, unless it is given an otpauth:// URI that carries one")
		}
		var err error
		if plan.name, plan.nameWasAdjusted, err = uri.DeriveName(); err != nil {
			return plan, fmt.Errorf("%w; give one: trustctl add NAME", err)
		}
		plan.derivedFromURI = true
	}
	if err := protocol.ValidateName(plan.name); err != nil {
		return plan, err
	}
	if plan.digits < 6 || plan.digits > 8 {
		return plan, fmt.Errorf("the device supports 6 to 8 digits, not %d", plan.digits)
	}
	if plan.period != 30 && plan.period != 60 {
		return plan, fmt.Errorf("the device supports a period of 30 or 60 seconds, not %d", plan.period)
	}
	return plan, nil
}

// unlockedWarning is printed when the device says it is a development unit.
const unlockedWarning = "WARNING: this device is not locked (its lock bits or fuses are not those of a finished device).\n" +
	"Anyone with physical access can read the keys out of it. Fine for a dev device, not for real keys.\n" +
	"(tools/user/final_lock.sh locks a finished device; this cannot be undone except by erasing it)"

func warnIfUnlocked(helloLine string, stderr io.Writer) {
	for _, word := range strings.Fields(helloLine) {
		if word == "unlocked" {
			fmt.Fprintln(stderr, unlockedWarning)
			return
		}
	}
}

func add(c *protocol.Client, args []string, stdout, stderr io.Writer) error {
	if line, err := c.Hello(); err == nil { // before real keys go in, say if the device is a dev unit
		warnIfUnlocked(line, stderr)
	}
	input, err := readSecret(stderr)
	if err != nil {
		return err
	}
	plan, err := planAdd(args, input)
	if err != nil {
		return err
	}
	if plan.derivedFromURI {
		note := ""
		if plan.nameWasAdjusted {
			note = " (characters the device does not accept were replaced by \"-\")"
		}
		fmt.Fprintf(stderr, "Using the name %q from the URI%s.\n", plan.name, note)
	}
	fmt.Fprintf(stderr, "Adding %q (%d digits, %d s): check the name on the display.\n", plan.name, plan.digits, plan.period)
	id, err := c.Add(plan.name, plan.secret, plan.digits, plan.period)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "stored as slot %d\n", id)
	return nil
}

// readSecret reads the secret from stdin. On a terminal it is a masked input
// field (a star for every character, with the usual line editing keys); the
// secret is never taken from the command line, where other users could see it.
func readSecret(stderr io.Writer) (string, error) {
	in := int(os.Stdin.Fd())
	var line string
	switch {
	case term.IsTerminal(in) && term.IsTerminal(int(os.Stderr.Fd())):
		var err error
		if line, err = readMasked(in, "Secret (Base32) or otpauth:// URI: "); err != nil {
			return "", err
		}
	case term.IsTerminal(in): // no terminal to draw the stars on: type blind
		fmt.Fprint(stderr, "Secret (Base32) or otpauth:// URI, input is hidden: ")
		b, err := term.ReadPassword(in)
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		line = string(b)
	default:
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

// readMasked runs the masked line editor on the terminal at fd (drawing on stderr).
func readMasked(fd int, prompt string) (string, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state)
	width, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || width < 20 {
		width = 80
	}
	m := &maskedLine{prompt: prompt, width: width, out: os.Stderr}
	m.render()
	buf := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			done, aborted := m.handle(buf[:n])
			m.render()
			if aborted {
				fmt.Fprint(os.Stderr, "\r\n")
				return "", fmt.Errorf("cancelled")
			}
			if done {
				fmt.Fprint(os.Stderr, "\r\n")
				return m.text(), nil
			}
		}
		if err != nil {
			fmt.Fprint(os.Stderr, "\r\n")
			return "", err
		}
	}
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
