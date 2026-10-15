package flasher_test

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"trustissues/trustctl/internal/flasher"
	"trustissues/trustctl/internal/hexfile"
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
	flash   [32768]byte
	opened  time.Time // answers only from this moment on, like the gate
	address int
	left    bool
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
		reply := byte(0x10)
		var data []byte
		switch cmd[0] {
		case 0x42:
			read(21)
		case 0x45:
			read(6)
		case 0x55:
			a := read(3)
			f.address = (int(a[1])<<8 | int(a[0])) << 1
		case 0x64:
			h := read(3)
			length := int(h[0])<<8 | int(h[1])
			body := read(length + 1)
			if h[2] != 'F' || f.address+length > 0x7800 {
				reply = 0x11
			} else {
				copy(f.flash[f.address:], body[:length])
			}
		case 0x74:
			h := read(4)
			length := int(h[0])<<8 | int(h[1])
			data = f.flash[f.address : f.address+length]
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

func TestSyncWriteVerify(t *testing.T) {
	host, dev := net.Pipe()
	defer host.Close()
	fb := &fakeBootloader{opened: time.Now().Add(700 * time.Millisecond)}
	go fb.serve(dev)
	p := flasher.New(pipePort{host})

	tries := 0
	if err := p.WaitForBootloader(5*time.Second, func() { tries++ }); err != nil {
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
	if len(phases) != 2*len(pages) {
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
	err := flasher.New(pipePort{host}).WaitForBootloader(1*time.Second, nil)
	if err == nil || !strings.Contains(err.Error(), "gesture") {
		t.Fatalf("expected a hint about the gesture: %v", err)
	}
}
