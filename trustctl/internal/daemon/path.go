// Package daemon is the optional background process that keeps the serial port
// open (so the device is not reset on every command, which also matters on
// systems where opening a port always resets it) and caches what does not change.
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the directory for the socket and the log, created with mode 0700.
func Dir() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" || !filepath.IsAbs(dir) {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("trustctl-%d", os.Getuid()))
	} else {
		dir = filepath.Join(dir, "trustctl")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if info, err := os.Stat(dir); err == nil && info.Mode().Perm()&0o077 != 0 && os.Getuid() >= 0 {
		// somebody else could reach the socket: tighten it
		if err := os.Chmod(dir, 0o700); err != nil {
			return "", fmt.Errorf("%s is accessible to others and cannot be fixed: %w", dir, err)
		}
	}
	return dir, nil
}

// SocketPath is where the daemon listens.
func SocketPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trustctl.sock"), nil
}

// LogPath is where a daemon started by trustctl writes its messages.
func LogPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.log"), nil
}
