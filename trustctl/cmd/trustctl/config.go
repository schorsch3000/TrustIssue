package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// config is the optional ~/.config/trustctl/config file: lines "key = value",
// "#" starts a comment. Keys: device (serial port path), autoprobe (true/false).
type config struct {
	device    string
	autoprobe bool
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
	var c config
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
		case "device":
			c.device = value
		case "autoprobe":
			c.autoprobe = value == "true" || value == "yes" || value == "1"
		}
	}
	return c
}
