package daemon

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

// Dial connects to a running daemon.
func Dial() (net.Conn, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	return net.DialTimeout("unix", path, time.Second)
}

// Start launches a daemon process (this same executable with the hidden
// "__daemon" command after the given global options) and waits until it listens.
func Start(args []string) (net.Conn, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath, err := LogPath()
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	cmd := exec.Command(exe, append(append([]string{}, args...), "__daemon")...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start the daemon: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.Now().Add(20 * time.Second) // opening the device can take a few seconds
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			text, _ := os.ReadFile(logPath)
			return nil, fmt.Errorf("the daemon stopped right away: %s", bytes.TrimSpace(text))
		case <-time.After(100 * time.Millisecond):
		}
		if conn, err := Dial(); err == nil {
			_ = cmd.Process.Release() // it lives on without us
			return conn, nil
		}
	}
	return nil, fmt.Errorf("the daemon did not start listening in time (see %s)", logPath)
}
