package serialdev

import "golang.org/x/sys/unix"

// disableHangup clears HUPCL on the port. Without it the kernel drops DTR when
// the port is closed and raises it again on the next open; on an Arduino that
// edge resets the board (and wakes its display). With HUPCL cleared DTR stays
// up between runs, so only the very first open after plugging in resets it.
func disableHangup(path string) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer unix.Close(fd)
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	termios.Cflag &^= unix.HUPCL
	_ = unix.IoctlSetTermios(fd, unix.TCSETS, termios)
}
