//go:build darwin || freebsd || openbsd

package serialdev

import "golang.org/x/sys/unix"

// disableHangup clears HUPCL so that DTR stays up between runs (see hangup_linux.go).
func disableHangup(path string) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer unix.Close(fd)
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	termios.Cflag &^= unix.HUPCL
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, termios)
}
