//go:build !linux && !darwin && !freebsd && !openbsd

package serialdev

// disableHangup does nothing here: the device is reset on every open. The
// trustctl daemon, which keeps the port open, avoids that.
func disableHangup(path string) {}
