// Package sim is a simulated Trust Issues device. It speaks the serial protocol
// like the firmware does (same validation, same replies), but the button is a
// callback, so trustctl can be developed and tested without hardware.
package sim

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Outcome is what the person at the device does about a request.
type Outcome int

const (
	Accept  Outcome = iota // hold the button for the right time
	Deny                   // release too early or too late
	Timeout                // do nothing until the device gives up
)

// Decider is called when a command needs the button. what is the command
// word ("TOKEN", "ADD", ...) and detail names the key or value concerned.
type Decider func(what, detail string) Outcome

// AcceptAll presses the button every time.
func AcceptAll(string, string) Outcome { return Accept }

type key struct {
	name   string
	secret []byte // zero padded to 20 bytes
	digits int
	period int
}

// Device is the simulated device.
type Device struct {
	decide Decider

	mu      sync.Mutex
	keys    [20]*key
	tz      int
	config  uint8
	pending bool
	out     io.Writer
	counts  map[string]int
}

// New returns the host side of a connection to a fresh simulated device.
func New(decide Decider) (io.ReadWriteCloser, *Device) {
	host, dev := net.Pipe()
	d := &Device{decide: decide, out: dev, counts: map[string]int{}}
	go d.serve(dev)
	return host, d
}

// Keys returns the names of the stored keys by slot (empty string: free).
func (d *Device) Keys() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	names := make([]string, len(d.keys))
	for i, k := range d.keys {
		if k != nil {
			names[i] = k.name
		}
	}
	return names
}

// Count returns how many commands with this first word the device has handled.
func (d *Device) Count(word string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[word]
}

func (d *Device) say(format string, args ...any) {
	fmt.Fprintf(d.out, format+"\n", args...)
}

func (d *Device) serve(conn net.Conn) {
	d.say("TrustIssue 0.1.0 proto 1")
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		d.mu.Lock()
		busy := d.pending
		d.mu.Unlock()
		if busy {
			d.say("ERR BUSY")
			continue
		}
		d.handle(line)
	}
}

func validName(name string) bool {
	if name == "" || len(name) > 27 || name[0] == ' ' || name[len(name)-1] == ' ' || strings.Contains(name, "  ") {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == ':' || c == '-' || c == ' ') {
			return false
		}
	}
	return true
}

func number(s string, max int64) (int64, bool) {
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n <= max
}

func (d *Device) used() int {
	n := 0
	for _, k := range d.keys {
		if k != nil {
			n++
		}
	}
	return n
}

func (d *Device) handle(line string) {
	word, rest, _ := strings.Cut(line, " ")
	d.mu.Lock()
	defer d.mu.Unlock()
	d.counts[word]++
	switch word {
	case "HELLO":
		d.say("TrustIssue 0.1.0 proto 1")
	case "LIST":
		for i, k := range d.keys {
			if k != nil {
				d.say("KEY %d %s", i, k.name)
			}
		}
		d.say("END")
	case "GETTZ":
		d.say("OK %d", d.tz)
	case "GETCFG":
		d.say("OK %02x", d.config)
	case "SETTZ":
		q, err := strconv.Atoi(rest)
		if err != nil || q < -48 || q > 56 {
			d.say("ERR BADARG")
			return
		}
		d.unlockedConfirm("SETTZ", rest, func() string { d.tz = q; return "OK" })
	case "SETCFG":
		v, err := strconv.ParseUint(rest, 16, 8)
		if err != nil || len(rest) != 2 || v&^0x0F != 0 {
			d.say("ERR BADARG")
			return
		}
		d.unlockedConfirm("SETCFG", rest, func() string { d.config = uint8(v); return "OK" })
	case "DEL", "TOKEN":
		idText, timeText, _ := strings.Cut(rest, " ")
		id64, ok := number(idText, 19)
		id := int(id64)
		if !ok || (word == "DEL") != (timeText == "") {
			d.say("ERR BADARG")
			return
		}
		if d.keys[id] == nil {
			d.say("ERR NOTFOUND")
			return
		}
		if word == "DEL" {
			d.unlockedConfirm("DEL", d.keys[id].name, func() string { d.keys[id] = nil; return "OK" })
			return
		}
		unix, ok := number(timeText, 0xFFFFFFFF)
		if !ok {
			d.say("ERR BADARG")
			return
		}
		k := d.keys[id]
		d.unlockedConfirm("TOKEN", k.name, func() string { return "OK " + totp(k, uint32(unix)) })
	case "ADD":
		d.handleAdd(rest)
	case "WIPE":
		if d.used() == 0 {
			d.say("OK")
			return
		}
		d.unlockedConfirm("WIPE", "", func() string { d.keys = [20]*key{}; return "OK" })
	default:
		d.say("ERR UNKNOWN")
	}
}

// unlockedConfirm is confirm for callers that hold d.mu; the action runs with it held.
func (d *Device) unlockedConfirm(what, detail string, action func() string) {
	d.pending = true
	d.say("PENDING")
	go func() {
		outcome := d.decide(what, detail)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.pending = false
		switch outcome {
		case Accept:
			d.say("%s", action())
		case Deny:
			d.say("ERR DENIED")
		default:
			d.say("ERR TIMEOUT")
		}
	}()
}

func (d *Device) handleAdd(rest string) {
	f := strings.SplitN(rest, " ", 5)
	if len(f) != 5 || f[1] != "SHA1" {
		d.say("ERR BADARG")
		return
	}
	digits64, ok1 := number(f[2], 8)
	period64, ok2 := number(f[3], 60)
	digits, period := int(digits64), int(period64)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(strings.ToUpper(f[0]), "="))
	if !ok1 || !ok2 || digits < 6 || (period != 30 && period != 60) || err != nil || len(secret) > 20 || len(secret) == 0 || !validName(f[4]) {
		d.say("ERR BADARG")
		return
	}
	free := -1
	for i, k := range d.keys {
		if k == nil {
			if free < 0 {
				free = i
			}
		} else if strings.EqualFold(k.name, f[4]) {
			d.say("ERR DUPLICATE")
			return
		}
	}
	if free < 0 {
		d.say("ERR FULL")
		return
	}
	padded := make([]byte, 20)
	copy(padded, secret)
	nk := &key{name: f[4], secret: padded, digits: digits, period: period}
	d.unlockedConfirm("ADD", nk.name, func() string {
		slot := -1
		for i, k := range d.keys {
			if k == nil {
				slot = i
				break
			}
		}
		d.keys[slot] = nk
		return fmt.Sprintf("OK %d", slot)
	})
}

func totp(k *key, unix uint32) string {
	counter := make([]byte, 8)
	binary.BigEndian.PutUint64(counter, uint64(unix/uint32(k.period)))
	mac := hmac.New(sha1.New, k.secret)
	mac.Write(counter)
	sum := mac.Sum(nil)
	offset := sum[19] & 0x0F
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF
	mod := uint32(1)
	for i := 0; i < k.digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", k.digits, code%mod)
}
