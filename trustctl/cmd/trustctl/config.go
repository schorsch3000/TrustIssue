package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The keys of the config file and the environment variables, shared by the code
// and by the help-options text, so that they cannot drift apart.
const (
	configKeyDevice      = "device"
	configKeyAutoprobe   = "autoprobe"
	configKeyDaemon      = "daemon"
	configKeyIdleTimeout = "idle_timeout"

	envDevice   = "TRUSTCTL_DEVICE"
	envSimulate = "TRUSTCTL_SIMULATE"
	envNoDaemon = "TRUSTCTL_NO_DAEMON"
)

// config is the optional ~/.config/trustctl/config file: lines "key = value",
// "#" starts a comment. Keys: device (serial port path), autoprobe (true/false), daemon (false disables
// the background daemon), idle_timeout (daemon lifetime without a command, default 5m).
type config struct {
	device      string
	autoprobe   bool
	noDaemon    bool          // "daemon = false"
	idleTimeout time.Duration // "idle_timeout = 5m": the daemon exits after this long without a command
}

func configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "trustctl", "config")
}

func loadConfig() config {
	c := config{idleTimeout: 5 * time.Minute}
	f, err := os.Open(configPath())
	if err != nil {
		return c
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line, _, _ := strings.Cut(s.Text(), "#")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case configKeyDevice:
			c.device = value
		case configKeyAutoprobe:
			c.autoprobe = value == "true" || value == "yes" || value == "1"
		case configKeyDaemon:
			c.noDaemon = value == "false" || value == "no" || value == "0"
		case configKeyIdleTimeout:
			if d, err := time.ParseDuration(value); err == nil {
				c.idleTimeout = d
			}
		}
	}
	return c
}
