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

	insync = 0x14
	ok     = 0x10
	eop    = 0x20
)

// Port is a serial port whose Read returns (0, nil) when the read timeout passes.
type Port interface {
	io.ReadWriter
	SetReadTimeout(t time.Duration) error
}

// Programmer talks STK500v1 over a port.
type Programmer struct{ port Port }

// New returns a programmer on an open port.
func New(port Port) *Programmer { return &Programmer{port: port} }

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
		return nil, fmt.Errorf("command %#02x was refused (reply %x)", request[0], reply)
	}
	return reply[1 : len(reply)-1], nil
}

// WaitForBootloader sends GET_SYNC until the bootloader answers. The gate (hold
// the button, release, wait for the bar, tap, tap, hold) opens it, which takes
// the person at the device a while; onTry is called once per attempt.
func (p *Programmer) WaitForBootloader(wait time.Duration, onTry func()) error {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if onTry != nil {
			onTry()
		}
		// anything that arrived meanwhile is leftover of an earlier attempt
		p.port.SetReadTimeout(time.Millisecond)
		p.port.Read(make([]byte, 64))
		p.port.Write([]byte{0x30, eop})
		reply, _ := p.readFull(2, 200*time.Millisecond)
		if bytes.Equal(reply, []byte{insync, ok}) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the bootloader did not open: was the gesture completed? (hold the button at reset, release, wait for the bar, then tap, tap, press and hold until the bar is full)")
}

// Write programs the pages, verifies them by reading them back, and leaves
// programming mode (the device then restarts into the new firmware).
func (p *Programmer) Write(pages []hexfile.Page, progress func(phase string, done, total int)) error {
	for _, page := range pages {
		if page.Address+len(page.Data) > BootStart {
			return fmt.Errorf("page %#x lies in the bootloader's section", page.Address)
		}
		if len(page.Data) != PageSize || page.Address%PageSize != 0 {
			return fmt.Errorf("page %#x has a bad size or alignment", page.Address)
		}
	}
	if _, err := p.exchange(append(append([]byte{0x42}, make([]byte, 20)...), eop), 0); err != nil { // set device
		return err
	}
	if _, err := p.exchange(append(append([]byte{0x45}, make([]byte, 5)...), eop), 0); err != nil { // set device ext
		return err
	}
	if _, err := p.exchange([]byte{0x50, eop}, 0); err != nil { // enter programming mode
		return err
	}
	loadAddress := func(address int) error {
		word := address >> 1
		_, err := p.exchange([]byte{0x55, byte(word), byte(word >> 8), eop}, 0)
		return err
	}
	for i, page := range pages {
		if err := loadAddress(page.Address); err != nil {
			return err
		}
		request := append([]byte{0x64, byte(len(page.Data) >> 8), byte(len(page.Data)), 'F'}, page.Data...)
		if _, err := p.exchange(append(request, eop), 0); err != nil {
			return fmt.Errorf("writing page %#x: %w", page.Address, err)
		}
		if progress != nil {
			progress("writing", i+1, len(pages))
		}
	}
	for i, page := range pages {
		if err := loadAddress(page.Address); err != nil {
			return err
		}
		got, err := p.exchange([]byte{0x74, byte(PageSize >> 8), byte(PageSize), 'F', eop}, PageSize)
		if err != nil {
			return fmt.Errorf("reading page %#x: %w", page.Address, err)
		}
		if !bytes.Equal(got, page.Data) {
			return fmt.Errorf("verification failed at page %#x", page.Address)
		}
		if progress != nil {
			progress("verifying", i+1, len(pages))
		}
	}
	_, err := p.exchange([]byte{0x51, eop}, 0) // leave programming mode; the device restarts
	return err
}
