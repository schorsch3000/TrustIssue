// Package protocol implements the host side of the Trust Issues serial protocol.
package protocol

import "fmt"

// Error is an "ERR <code>" reply from the device.
type Error struct{ Code string }

func (e *Error) Error() string { return "device: " + describe(e.Code) }

// Is makes errors.Is(err, protocol.ErrDenied) work by comparing the codes.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

var (
	ErrDenied    = &Error{"DENIED"}
	ErrTimeout   = &Error{"TIMEOUT"}
	ErrBusy      = &Error{"BUSY"}
	ErrFull      = &Error{"FULL"}
	ErrDuplicate = &Error{"DUPLICATE"}
	ErrBadArg    = &Error{"BADARG"}
	ErrNotFound  = &Error{"NOTFOUND"}
	ErrUnknown   = &Error{"UNKNOWN"}
)

func describe(code string) string {
	switch code {
	case "DENIED":
		return "refused on the device (the button was not held for the right time)"
	case "TIMEOUT":
		return "nobody confirmed on the device in time"
	case "BUSY":
		return "the device is waiting for a confirmation of another request"
	case "FULL":
		return "no free key slot (the device stores 20 keys)"
	case "DUPLICATE":
		return "a key with this name already exists (names are compared ignoring case)"
	case "BADARG":
		return "the device rejected the arguments"
	case "NOTFOUND":
		return "no such key"
	case "UNKNOWN":
		return "the device does not know this command"
	}
	return fmt.Sprintf("error %s", code)
}
