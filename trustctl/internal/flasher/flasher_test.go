package flasher_test

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"trustissue/trustctl/internal/flasher"
	"trustissue/trustctl/internal/hexfile"
)

// pipePort adapts a net.Pipe to the Port interface (Read returns 0, nil on timeout).
type pipePort struct{ net.Conn }

func (p pipePort) SetReadTimeout(t time.Duration) error {
	return p.Conn.SetReadDeadline(time.Now().Add(t))
}
func (p pipePort) Read(b []byte) (int, error) {
	n, err := p.Conn.Read(b)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return 0, nil
		}
	}
	return n, err
}

// fakeBootloader implements the bootloader's STK500 subset like firmware/bootloader does:
// flash only, nothing at or above 0x7800.
type fakeBootloader struct {
	flash    [32768]byte
	opened   time.Time // answers only from this moment on, like the gate
	address  int
	left     bool
	corrupt  int // the next pages that fail the checksum
	refused  int
	pin      string // the update PIN; empty: none
	unlocked bool   // the right PIN was received
	app      bool   // a wrong PIN: the application runs, the bootloader is gone
}

func (f *fakeBootloader) serve(c net.Conn) {
	read := func(n int) []byte {
		buf := make([]byte, n)
		for got := 0; got < n; {
			m, err := c.Read(buf[got:])
			if err != nil {
				return nil
			}
			got += m
		}
		return buf
	}
	for {
		cmd := read(1)
		if cmd == nil {
			return
		}
		if time.Now().Before(f.opened) { // the gate is still closed: the UART is off
			if cmd[0] == 0x30 {
				read(1)
			}
			continue
		}
		if f.app { // a wrong PIN: the application printed its banner and ignores everything
			continue
		}
		if f.pin != "" && !f.unlocked {
			if cmd[0] == 0x02 {
				var line []byte
				for {
					b := read(1)
					if b == nil || b[0] == '\n' {
						break
					}
					line = append(line, b[0])
				}
				if string(line) == f.pin {
					f.unlocked = true
				} else {
					f.app = true // the application: silent at this baud rate
				}
			} else {
				if cmd[0] == 0x30 {
					read(1)
				}
				c.Write([]byte("?"))
			}
			continue
		}
		reply := byte(0x10)
		var data []byte
		switch cmd[0] {
		case 0x55:
			a := read(3)
			f.address = (int(a[1])<<8 | int(a[0])) << 1
		case 0x64:
			h := read(3)
			length := int(h[0])<<8 | int(h[1])
			body := read(length + 2) // the page, the sum of the page, EOP
			var sum byte
			for _, b := range body[:length] {
				sum += b
			}
			switch {
			case h[2] != 'F' || f.address+length > 0x7800:
				reply = 0x11
			case sum != body[length] || f.corrupt > 0: // a line error: refused, nothing written
				f.corrupt--
				f.refused++
				reply = 0x11
			default:
				copy(f.flash[f.address:], body[:length])
			}
		case 0x51:
			read(1)
			f.left = true
		default:
			read(1)
		}
		c.Write([]byte{0x14})
		c.Write(data)
		c.Write([]byte{reply})
	}
}

func TestSyncWrite(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	fb := &fakeBootloader{opened: time.Now().Add(700 * time.Millisecond)}
	go fb.serve(dev)
	p := flasher.New(pipePort{host})

	tries := 0
	if err := p.WaitForBootloader(5*time.Second, func() { tries++ }, nil); err != nil {
		t.Fatal(err)
	}
	if tries < 2 {
		t.Errorf("the gate was closed for 0.7 s but only %d attempts were made", tries)
	}

	img := &hexfile.Image{}
	for i := range img.Data {
		img.Data[i] = 0xFF
	}
	img.Write(0, bytes.Repeat([]byte{0xAB}, 300))
	img.Write(0x7000, []byte("TIPB"))
	pages := img.Pages(flasher.PageSize)
	var phases []string
	if err := p.Write(pages, func(phase string, done, total int) { phases = append(phases, phase) }); err != nil {
		t.Fatal(err)
	}
	if !fb.left {
		t.Error("programming mode was not left")
	}
	if !bytes.Equal(fb.flash[0:300], bytes.Repeat([]byte{0xAB}, 300)) || fb.flash[300] != 0xFF {
		t.Error("the flash does not hold the image")
	}
	if string(fb.flash[0x7000:0x7004]) != "TIPB" {
		t.Error("the parameter block page was not written")
	}
	if len(phases) != len(pages) {
		t.Errorf("progress called %d times for %d pages", len(phases), len(pages))
	}
}

func TestRefusesTheBootloaderSection(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	go (&fakeBootloader{}).serve(dev)
	p := flasher.New(pipePort{host})
	err := p.Write([]hexfile.Page{{Address: 0x7800, Data: make([]byte, flasher.PageSize)}}, nil)
	if err == nil || !strings.Contains(err.Error(), "bootloader") {
		t.Fatalf("a page in the bootloader's section was accepted: %v", err)
	}
}

func TestGateNeverOpens(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	go (&fakeBootloader{opened: time.Now().Add(time.Hour)}).serve(dev)
	err := flasher.New(pipePort{host}).WaitForBootloader(1*time.Second, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "gesture") {
		t.Fatalf("expected a hint about the gesture: %v", err)
	}
}

func writeOnePage(t *testing.T, p *flasher.Programmer) {
	t.Helper()
	img := &hexfile.Image{}
	for i := range img.Data {
		img.Data[i] = 0xFF
	}
	img.Write(0, bytes.Repeat([]byte{0x5A}, 200))
	if err := p.Write(img.Pages(flasher.PageSize), nil); err != nil {
		t.Fatal(err)
	}
}

func TestPageIsSentAgainAfterALineError(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	fb := &fakeBootloader{corrupt: 2}
	go fb.serve(dev)
	p := flasher.New(pipePort{host})
	writeOnePage(t, p)
	if fb.refused != 2 || fb.flash[0] != 0x5A || fb.flash[199] != 0x5A {
		t.Errorf("refused %d times, flash %x", fb.refused, fb.flash[:4])
	}
}

func TestPageGivesUpAfterThreeTries(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	go (&fakeBootloader{corrupt: 99}).serve(dev)
	img := &hexfile.Image{}
	img.Write(0, []byte{1})
	err := flasher.New(pipePort{host}).Write(img.Pages(flasher.PageSize), nil)
	if err == nil || !strings.Contains(err.Error(), "page 0x0") {
		t.Fatalf("expected a failure naming the page: %v", err)
	}
}

func TestPinIsAskedForOnceAndSent(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	fb := &fakeBootloader{pin: "4711"}
	go fb.serve(dev)
	asked := 0
	p := flasher.New(pipePort{host})
	p.PinPause = 10 * time.Millisecond
	err := p.WaitForBootloader(5*time.Second, nil, func() (string, error) {
		asked++
		return "4711", nil
	})
	if err != nil || asked != 1 || !fb.unlocked {
		t.Fatalf("err %v, asked %d, unlocked %v", err, asked, fb.unlocked)
	}
}

func TestWrongPin(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	go (&fakeBootloader{pin: "4711"}).serve(dev)
	p := flasher.New(pipePort{host})
	p.PinPause, p.PinDecision = 10*time.Millisecond, 600*time.Millisecond
	err := p.WaitForBootloader(5*time.Second, nil, func() (string, error) { return "0815", nil })
	if !errors.Is(err, flasher.ErrWrongPIN) {
		t.Fatalf("expected ErrWrongPIN, got %v", err)
	}
}

func TestPinNeededButNoWayToAsk(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	go (&fakeBootloader{pin: "4711"}).serve(dev)
	err := flasher.New(pipePort{host}).WaitForBootloader(5*time.Second, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "PIN") {
		t.Fatalf("expected a hint about the PIN: %v", err)
	}
}

func TestValidatePINAndPage(t *testing.T) {
	for _, bad := range []string{"", "123", "1234567890123456", "12 34", "12\n34", "12ü34"} {
		if flasher.ValidatePIN(bad) == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	for _, good := range []string{"1234", "abcd!#%&", "123456789012345"} {
		if err := flasher.ValidatePIN(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
	page := flasher.PinPageData("4711")
	if len(page) != flasher.PageSize || string(page[:4]) != "4711" || page[4] != 0xFF || page[127] != 0xFF {
		t.Errorf("page %x", page[:8])
	}
	if !bytes.Equal(flasher.PinPageData(""), bytes.Repeat([]byte{0xFF}, flasher.PageSize)) {
		t.Error("an empty PIN must give the blank page")
	}
}
