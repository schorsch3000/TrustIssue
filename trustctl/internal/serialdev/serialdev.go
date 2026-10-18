// Package serialdev opens the Trust Issue device on a serial port.
package serialdev

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"go.bug.st/serial"

	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/sim"
)

// Baud is the speed of the device's serial protocol.
const Baud = 9600

// Device is an open connection to the hardware.
type Device struct {
	Path   string
	Banner string
	Client *protocol.Client
	port   serial.Port
	closer io.Closer // the simulated device has no port
}

// DummyPath is the device name of the simulated device that lives in memory: "--device DUMMY".
const DummyPath = "DUMMY"

// IsDummy says whether a device name asks for the simulated device (any case).
func IsDummy(path string) bool { return strings.EqualFold(path, DummyPath) }

// OpenDummy returns a fresh simulated device that accepts every confirmation. Behind the daemon it keeps its
// keys from one command to the next, until the daemon stops.
func OpenDummy() (*Device, error) {
	conn, _ := sim.New(sim.AcceptAll)
	client := protocol.NewClient(conn)
	banner, err := client.WaitBanner(time.Second)
	if err != nil {
		conn.Close()
		return nil, err
	}
	client.ReplyTimeout = 3 * time.Second
	return &Device{Path: DummyPath, Banner: banner, Client: client, closer: conn}, nil
}

// Open connects to the device at path. The first open after plugging in resets
// the device (DTR); later ones normally do not (see disableHangup). So this asks
// HELLO instead of just waiting for the start-up banner, and answers to either.
func Open(path string, timeout time.Duration) (*Device, error) {
	disableHangup(path)
	port, err := serial.Open(path, &serial.Mode{BaudRate: Baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	client := protocol.NewClient(port)
	client.ReplyTimeout = time.Second
	var banner string
	start := time.Now()
	deadline := start.Add(timeout)
	attempts := 0
	for {
		attempts++
		// a reset in progress swallows the first HELLOs; the banner it ends with also counts
		banner, err = client.Hello()
		if err == nil || time.Now().After(deadline) {
			break
		}
	}
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("%s does not answer like a Trust Issue device: %w", path, err)
	}
	client.ReplyTimeout = 3 * time.Second
	if attempts == 1 && time.Since(start) < 120*time.Millisecond {
		client.Settle(50 * time.Millisecond) // answered at once: the device was running, no reset
	} else {
		client.Settle(800 * time.Millisecond) // a reset happened; a second one (RTS/DTR toggling) may still follow
	}
	if !strings.Contains(banner, " proto 1") {
		port.Close()
		return nil, fmt.Errorf("%s speaks an unsupported protocol version: %q", path, banner)
	}
	return &Device{Path: path, Banner: banner, Client: client, port: port}, nil
}

// Close releases the port.
func (d *Device) Close() error {
	if d.closer != nil {
		return d.closer.Close()
	}
	return d.port.Close()
}

// Candidates lists the serial ports that could be the device.
func Candidates() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range ports {
		switch runtime.GOOS {
		case "linux":
			if !strings.Contains(p, "ttyUSB") && !strings.Contains(p, "ttyACM") {
				continue
			}
		case "darwin":
			if !strings.Contains(p, "cu.usb") {
				continue
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// Probe opens every candidate port until one answers as a Trust Issue device.
// Opening a port resets whatever board is attached, so this is opt-in.
func Probe() (*Device, error) {
	candidates, err := Candidates()
	if err != nil {
		return nil, err
	}
	for _, path := range candidates {
		if d, err := Open(path, 3*time.Second); err == nil {
			return d, nil
		}
	}
	return nil, fmt.Errorf("no Trust Issue device found among %d serial ports", len(candidates))
}

// OpenCalibration connects to the calibration firmware (it is not the app and
// does not speak the key protocol, only the display commands).
func OpenCalibration(path string) (*Device, error) {
	disableHangup(path)
	port, err := serial.Open(path, &serial.Mode{BaudRate: Baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	client := protocol.NewClient(port)
	client.ReplyTimeout = time.Second
	var banner string
	deadline := time.Now().Add(4 * time.Second)
	for {
		banner, err = client.Hello()
		if err == nil || time.Now().After(deadline) {
			break
		}
	}
	if err != nil || !strings.HasPrefix(banner, "TrustIssue-Calibration") {
		port.Close()
		if err == nil {
			err = fmt.Errorf("it says %q", banner)
		}
		return nil, fmt.Errorf("%s is not running the calibration firmware (%w). Flash it with: trustctl --device %s flash calibration; flash the core firmware again afterwards (trustctl flash core)", path, err, path)
	}
	client.ReplyTimeout = 3 * time.Second
	client.Settle(100 * time.Millisecond)
	return &Device{Path: path, Banner: banner, Client: client, port: port}, nil
}

// OpenBootloaderPort opens the port at the bootloader's speed and resets the
// device with a DTR (and RTS) pulse. Whether the bootloader opens depends on the button:
// it must be held while the reset happens.
func OpenBootloaderPort(path string) (serial.Port, error) {
	disableHangup(path)
	port, err := serial.Open(path, &serial.Mode{BaudRate: 115200, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	return port, nil
}

// Reset pulses DTR, which resets an Arduino, and RTS with it: the reset capacitor of some CH340 boards hangs on RTS
// and a DTR pulse alone does nothing there (found on a Nano clone of the test rig). Nothing on the usual boards
// listens to RTS, so pulsing both costs nothing. A line the driver does not support is not an error.
func Reset(port serial.Port) error {
	rtsErr := port.SetRTS(false)
	if err := port.SetDTR(false); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if rtsErr == nil {
		_ = port.SetRTS(true)
	}
	return port.SetDTR(true)
}
