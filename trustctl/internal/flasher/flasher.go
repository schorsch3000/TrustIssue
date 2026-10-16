// Package flasher speaks the bootloader's STK500v1 subset (flash only).
package flasher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"trustissues/trustctl/internal/hexfile"
)

const (
	// PageSize is the ATmega328P's flash page.
	PageSize = 128
	// BootStart is where the bootloader's own section begins; nothing may be written from here.
	BootStart = 0x7800

	// PinPage is the page of the optional update PIN, the last one before the bootloader. The
	// bootloader compares what the host sends with it and never lets anything read it back.
	PinPage = 0x7780
	// MinPIN and MaxPIN bound the PIN's length in characters.
	MinPIN = 4
	MaxPIN = 15

	insync = 0x14
	ok     = 0x10
	failed = 0x11
	eop    = 0x20

	pinPrompt = '?'  // the bootloader sends it every 100 ms while it waits for the PIN
	pinStart  = 0x02 // the PIN line is this byte, the characters and a line feed
	pageTries = 3    // a page that failed its checksum or its verify is sent again
)

// ErrWrongPIN is returned when the device took the PIN and started the application instead.
var ErrWrongPIN = errors.New("the device did not accept the PIN (a wrong PIN starts the application after a wait of about 7 s; the gate has to be done again)")

// ValidatePIN checks the characters a PIN may have: printable ASCII without spaces, MinPIN to MaxPIN long.
func ValidatePIN(pin string) error {
	if len(pin) < MinPIN || len(pin) > MaxPIN {
		return fmt.Errorf("the PIN needs %d to %d characters", MinPIN, MaxPIN)
	}
	for _, c := range []byte(pin) {
		if c <= ' ' || c >= 0x7F {
			return errors.New("the PIN may only use printable ASCII characters without spaces")
		}
	}
	return nil
}

// PinPageData is the page that sets the PIN; an empty PIN gives the blank page that removes it.
func PinPageData(pin string) []byte {
	page := bytes.Repeat([]byte{0xFF}, PageSize)
	copy(page, pin)
	return page
}

// errRefused marks a reply of INSYNC then FAILED: the device understood but did not do it.
var errRefused = errors.New("refused by the bootloader")

// Port is a serial port whose Read returns (0, nil) when the read timeout passes.
type Port interface {
	io.ReadWriter
	SetReadTimeout(t time.Duration) error
}

// Programmer talks STK500v1 over a port.
type Programmer struct {
	port Port
	// PinPause is how long polling stops after the PIN was sent: the device needs about 7 s to
	// decide and the polls would only pile up in its receive buffer.
	PinPause time.Duration
	// PinDecision is how long after sending the PIN the bootloader is given to open. A wrong PIN
	// starts the application, which at this baud rate cannot be told from silence, so silence for
	// this long is taken as a wrong PIN. The bootloader decides after about 7 s.
	PinDecision time.Duration
}

// New returns a programmer on an open port.
func New(port Port) *Programmer {
	return &Programmer{port: port, PinPause: 5 * time.Second, PinDecision: 12 * time.Second}
}

// readFull reads n bytes within the timeout.
func (p *Programmer) readFull(n int, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 0, n)
	for len(buf) < n {
		left := time.Until(deadline)
		if left <= 0 {
			return buf, errors.New("no answer from the bootloader")
		}
		if err := p.port.SetReadTimeout(left); err != nil {
			return buf, err
		}
		chunk := make([]byte, n-len(buf))
		got, err := p.port.Read(chunk)
		if err != nil {
			return buf, err
		}
		buf = append(buf, chunk[:got]...)
	}
	return buf, nil
}

// exchange sends a command and expects INSYNC, replyLength bytes and OK.
func (p *Programmer) exchange(request []byte, replyLength int) ([]byte, error) {
	if _, err := p.port.Write(request); err != nil {
		return nil, err
	}
	reply, err := p.readFull(replyLength+2, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("command %#02x: %w (got %x)", request[0], err, reply)
	}
	if reply[0] != insync || reply[len(reply)-1] != ok {
		if reply[0] == insync && reply[len(reply)-1] == failed {
			return nil, fmt.Errorf("command %#02x: %w", request[0], errRefused)
		}
		return nil, fmt.Errorf("command %#02x was refused (reply %x)", request[0], reply)
	}
	return reply[1 : len(reply)-1], nil
}

// WaitForBootloader sends GET_SYNC until the bootloader answers. The gate (hold
// the button, release, wait for the bar, tap, tap, hold) opens it, which takes
// the person at the device a while; onTry is called once per attempt. If the
// device has an update PIN it asks for it (a stream of '?'); pin is then called
// once and its answer sent, after which the device takes about 7 s to decide.
func (p *Programmer) WaitForBootloader(wait time.Duration, onTry func(), pin func() (string, error)) error {
	deadline := time.Now().Add(wait)
	var answered time.Time
	for time.Now().Before(deadline) {
		if onTry != nil {
			onTry()
		}
		// anything that arrived meanwhile is leftover of an earlier attempt
		p.port.SetReadTimeout(time.Millisecond)
		p.port.Read(make([]byte, 64))
		if !answered.IsZero() && time.Since(answered) > p.PinDecision {
			return ErrWrongPIN
		}
		p.port.Write([]byte{0x30, eop})
		reply, _ := p.readFull(2, 200*time.Millisecond)
		switch {
		case bytes.Equal(reply, []byte{insync, ok}):
			return nil
		case bytes.IndexByte(reply, pinPrompt) >= 0 && answered.IsZero():
			if pin == nil {
				return errors.New("the device asks for its update PIN, but none can be entered here")
			}
			text, err := pin()
			if err != nil {
				return err
			}
			if err := ValidatePIN(text); err != nil {
				return err
			}
			// The bootloader only listens between its prompts (the UART holds just 3 bytes): the start
			// byte ends the prompting, the rest follows once it has had time to finish its wait.
			p.port.Write([]byte{pinStart})
			time.Sleep(250 * time.Millisecond)
			p.port.Write(append([]byte(text), '\n'))
			answered = time.Now()
			time.Sleep(p.PinPause)
			continue
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the bootloader did not open: was the gesture completed? (hold the button at reset, release, wait for the bar, then tap, tap, press and hold until the bar is full)")
}

// Write programs the pages and leaves programming mode (the device then
// restarts into the new firmware). Every page carries a checksum byte, and the
// bootloader checks the page it wrote against what it received; flash cannot be
// read back, so there is nothing to verify here. A page that fails is sent again.
func (p *Programmer) Write(pages []hexfile.Page, progress func(phase string, done, total int)) error {
	for _, page := range pages {
		if page.Address+len(page.Data) > BootStart {
			return fmt.Errorf("page %#x lies in the bootloader's section", page.Address)
		}
		if len(page.Data) != PageSize || page.Address%PageSize != 0 {
			return fmt.Errorf("page %#x has a bad size or alignment", page.Address)
		}
	}
	if _, err := p.exchange([]byte{0x50, eop}, 0); err != nil { // enter programming mode
		return err
	}
	for i, page := range pages {
		word := page.Address >> 1
		var sum byte
		for _, b := range page.Data {
			sum += b
		}
		request := append([]byte{0x64, byte(len(page.Data) >> 8), byte(len(page.Data)), 'F'}, page.Data...)
		request = append(request, sum, eop)
		var err error
		for try := 0; try < pageTries; try++ {
			if _, err = p.exchange([]byte{0x55, byte(word), byte(word >> 8), eop}, 0); err != nil {
				return err
			}
			if _, err = p.exchange(request, 0); !errors.Is(err, errRefused) {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("writing page %#x: %w (the bootloader checks every page after writing it)", page.Address, err)
		}
		if progress != nil {
			progress("writing", i+1, len(pages))
		}
	}
	_, err := p.exchange([]byte{0x51, eop}, 0) // leave programming mode; the device restarts
	return err
}
