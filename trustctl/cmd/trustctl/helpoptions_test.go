package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func helpOptionsText(t *testing.T) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := run([]string{"help-options"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	return out.String()
}

func TestHelpOptionsMentionsEverything(t *testing.T) {
	text := helpOptionsText(t)
	for _, want := range []string{
		"--device PATH", "--autoprobe", "--simulate", "--no-daemon",
		envDevice, envSimulate, envNoDaemon,
		configKeyDevice + " = PATH", configKeyAutoprobe + " = true", configKeyDaemon + " = false", configKeyIdleTimeout + " = 5m",
		"--digits N", "--period S", "--no-patch", "--pin", "--set-pin", "--clear-pin", "--profile PATH", "--wait DURATION", "--countdown N", "--invert",
		`"contrast_full"`, `"contrast_dim"`, `"logo"`, `"logo_invert"`, `"font"`,
		"rotate=on|off", "invert=on|off", "logo=on|off", "autowipe=on|off",
		configPath(), "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("help-options does not mention %q", want)
		}
	}
}

// The documented flags, variables and keys really exist.
func TestDocumentedGlobalOptionsAreAccepted(t *testing.T) {
	for _, args := range [][]string{
		{"--simulate", "hello"},
		{"--simulate", "--autoprobe", "hello"},
		{"--simulate", "--no-daemon", "hello"},
		{"--device", "/dev/null", "--simulate", "hello"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != exitOK {
			t.Errorf("%v: exit %d: %s", args, code, errOut.String())
		}
	}
	t.Setenv(envSimulate, "1")
	var out, errOut bytes.Buffer
	if code := run([]string{"hello"}, &out, &errOut); code != exitOK || !strings.Contains(out.String(), "TrustIssue") {
		t.Errorf("%s=1: exit %d", envSimulate, code)
	}
}

func TestDocumentedConfigKeysAreRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "trustctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := "# a comment\n" + configKeyDevice + " = /dev/ttyACM9\n" + configKeyAutoprobe + " = true  # inline\n" +
		configKeyDaemon + " = false\n" + configKeyIdleTimeout + " = 90s\n"
	if err := os.WriteFile(filepath.Join(dir, "trustctl", "config"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig()
	if cfg.device != "/dev/ttyACM9" || !cfg.autoprobe || !cfg.noDaemon || cfg.idleTimeout != 90*time.Second {
		t.Errorf("config read as %+v", cfg)
	}
	if !strings.Contains(helpOptionsText(t), filepath.Join(dir, "trustctl", "config")) {
		t.Error("help-options must show where the config file is")
	}
}

func TestEnvironmentBeatsConfigAndFlagBeatsEnvironment(t *testing.T) {
	cfg := config{device: "/from/config"}
	t.Setenv(envDevice, "/from/env")
	if got, _ := devicePath(options{}, cfg); got != "/from/env" {
		t.Errorf("environment: %q", got)
	}
	if got, _ := devicePath(options{device: "/from/flag"}, cfg); got != "/from/flag" {
		t.Errorf("flag: %q", got)
	}
	t.Setenv(envDevice, "")
	if got, _ := devicePath(options{}, cfg); got != "/from/config" {
		t.Errorf("config: %q", got)
	}
}

func TestHelpShowsTheUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"help"}, &out, &errOut); code != exitOK || !strings.Contains(out.String(), "usage: trustctl") {
		t.Errorf("exit %d", code)
	}
}

func TestHelpOptionsLinesFitATerminal(t *testing.T) {
	for _, line := range strings.Split(helpOptionsText(t), "\n") {
		if len(line) > 100 && !strings.Contains(line, "/") { // long paths in the text are the user's, not ours
			t.Errorf("a line of %d characters: %q", len(line), line)
		}
	}
	if got := wrap("one two three four", "  ", 12); got != "  one two\n  three four\n" {
		t.Errorf("wrap: %q", got)
	}
}

func TestGlobalOptionsMayFollowTheCommand(t *testing.T) {
	cases := []struct {
		args []string
		want options
		rest []string
	}{
		{[]string{"--device", "/dev/x", "logo", "a.png"}, options{device: "/dev/x"}, []string{"logo", "a.png"}},
		{[]string{"logo", "--device=/dev/y", "a.png"}, options{device: "/dev/y"}, []string{"logo", "a.png"}},
		{[]string{"-device", "/dev/z", "--autoprobe", "--simulate", "--no-daemon", "x"}, options{device: "/dev/z", autoprobe: true, simulate: true, noDaemon: true}, []string{"x"}},
		{[]string{"NAME", "--digits", "8"}, options{}, []string{"NAME", "--digits", "8"}},
		{[]string{"x", "--device"}, options{}, []string{"x", "--device"}}, // no value: left alone
		{[]string{"-x", "---device", "y"}, options{}, []string{"-x", "---device", "y"}},
	}
	for _, c := range cases {
		var opt options
		rest := takeGlobalOptions(c.args, &opt)
		if opt != c.want || strings.Join(rest, "|") != strings.Join(c.rest, "|") {
			t.Errorf("%q: got %+v %q, want %+v %q", c.args, opt, rest, c.want, c.rest)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"hello", "--simulate"}, &out, &errOut); code != exitOK || !strings.Contains(out.String(), "TrustIssue") {
		t.Errorf("--simulate after the command: exit %d %s", code, errOut.String())
	}
	if code := run([]string{"calibrate", "--device", "/dev/null", "show"}, &out, &errOut); code != exitOK {
		t.Errorf("calibrate --device ... show: exit %d %s", code, errOut.String())
	}
}
