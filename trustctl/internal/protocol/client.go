package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Key is one entry of the device's key list.
type Key struct {
	ID   int
	Name string
}

// Client talks the line protocol over any byte stream (a serial port, the
// simulator, a socket to the daemon). It is safe for concurrent use; requests
// are strictly one at a time, as the device wants.
type Client struct {
	w     io.Writer
	lines chan lineOrError
	done  chan struct{}
	mu    sync.Mutex

	// OnPending is called when the device waits for the button.
	OnPending func()
	// ReplyTimeout is how long an immediate reply may take.
	ReplyTimeout time.Duration
	// ConfirmTimeout is how long a confirmation may take (the device itself gives up after 30 s).
	ConfirmTimeout time.Duration
}

type lineOrError struct {
	text string
	err  error
}

// NewClient starts reading lines from rw.
func NewClient(rw io.ReadWriter) *Client {
	c := &Client{
		w:              rw,
		lines:          make(chan lineOrError, 256),
		done:           make(chan struct{}),
		ReplyTimeout:   3 * time.Second,
		ConfirmTimeout: 40 * time.Second,
	}
	go func() {
		r := bufio.NewReader(rw)
		for {
			text, err := r.ReadString('\n')
			if len(text) > 0 && err == nil {
				c.lines <- lineOrError{text: strings.TrimRight(text, "\r\n")}
			}
			if err != nil {
				c.lines <- lineOrError{err: err}
				close(c.lines)
				close(c.done)
				return
			}
		}
	}()
	return c
}

var errClosed = errors.New("the connection to the device was closed")

// Done is closed when the connection to the device has ended.
func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) readLine(timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case l, ok := <-c.lines:
		if !ok {
			return "", errClosed
		}
		if l.err != nil {
			return "", fmt.Errorf("%w: %v", errClosed, l.err)
		}
		return l.text, nil
	case <-timer.C:
		return "", fmt.Errorf("no reply from the device within %v", timeout)
	}
}

// drain throws away unsolicited lines (such as the boot banner) before a request.
func (c *Client) drain() {
	for {
		select {
		case _, ok := <-c.lines:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// WaitBanner waits for the "TrustIssue <version> proto <n>" line that the device
// prints at start-up (opening the serial port resets it).
func (c *Client) WaitBanner(timeout time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return "", fmt.Errorf("no start-up banner within %v", timeout)
		}
		line, err := c.readLine(left)
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(line, "TrustIssue ") {
			return line, nil
		}
	}
}

// Settle discards whatever the device prints until it has been quiet for the
// given time. Opening a serial port can reset the device more than once in a
// row (DTR and RTS toggling), so one banner is not yet a stable connection.
func (c *Client) Settle(quiet time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if _, err := c.readLine(quiet); err != nil {
			return
		}
	}
}

func (c *Client) send(command string) error {
	_, err := io.WriteString(c.w, command+"\n")
	return err
}

func parseError(line string) error {
	if code, ok := strings.CutPrefix(line, "ERR "); ok {
		return &Error{Code: strings.TrimSpace(code)}
	}
	return fmt.Errorf("unexpected reply %q", line)
}

// request sends a command and returns the text after "OK ". For commands that
// need the button the device answers PENDING first.
func (c *Client) request(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drain()
	if err := c.send(command); err != nil {
		return "", err
	}
	line, err := c.readLine(c.ReplyTimeout)
	if err != nil {
		return "", err
	}
	if line == "PENDING" {
		if c.OnPending != nil {
			c.OnPending()
		}
		if line, err = c.readLine(c.ConfirmTimeout); err != nil {
			return "", err
		}
	}
	if line == "OK" {
		return "", nil
	}
	if rest, ok := strings.CutPrefix(line, "OK "); ok {
		return rest, nil
	}
	if strings.HasPrefix(line, "TrustIssue") { // "TrustIssue 0.1.0 ..." or "TrustIssue-Calibration ..."
		if command == "HELLO" {
			return line, nil
		}
		return "", fmt.Errorf("the device restarted while handling %q", command)
	}
	return "", parseError(line)
}

// Hello returns the device's identification line.
func (c *Client) Hello() (string, error) { return c.request("HELLO") }

// List returns the keys on the device (names are not secret).
func (c *Client) List() ([]Key, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drain()
	if err := c.send("LIST"); err != nil {
		return nil, err
	}
	var keys []Key
	for {
		line, err := c.readLine(c.ReplyTimeout)
		if err != nil {
			return nil, err
		}
		if line == "END" {
			return keys, nil
		}
		rest, ok := strings.CutPrefix(line, "KEY ")
		if !ok {
			return nil, parseError(line)
		}
		idText, name, _ := strings.Cut(rest, " ")
		id, err := strconv.Atoi(idText)
		if err != nil {
			return nil, fmt.Errorf("bad key line %q", line)
		}
		keys = append(keys, Key{ID: id, Name: name})
	}
}

// Timezone returns the device's offset from UTC in 15 minute units.
func (c *Client) Timezone() (int, error) {
	reply, err := c.request("GETTZ")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(reply)
}

// SetTimezone sets the offset (needs the button on the device).
func (c *Client) SetTimezone(quarterHours int) error {
	_, err := c.request(fmt.Sprintf("SETTZ %d", quarterHours))
	return err
}

// Config returns the device's configuration flags.
func (c *Client) Config() (uint8, error) {
	reply, err := c.request("GETCFG")
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(reply, 16, 8)
	return uint8(v), err
}

// SetConfig sets the configuration flags (needs the button on the device).
func (c *Client) SetConfig(flags uint8) error {
	_, err := c.request(fmt.Sprintf("SETCFG %02x", flags))
	return err
}

// Add stores a key (needs the button on the device) and returns its slot.
// The secret must be Base32 as returned by NormalizeSecret.
func (c *Client) Add(name, secret string, digits, period int) (int, error) {
	if err := ValidateName(name); err != nil {
		return 0, err
	}
	secret, err := NormalizeSecret(secret)
	if err != nil {
		return 0, err
	}
	reply, err := c.request(fmt.Sprintf("ADD %s SHA1 %d %d %s", secret, digits, period, name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(reply)
}

// Delete removes a key (needs the button on the device).
func (c *Client) Delete(id int) error {
	_, err := c.request(fmt.Sprintf("DEL %d", id))
	return err
}

// Token asks for the current code of a key (needs the button on the device).
func (c *Client) Token(id int, unixTime int64) (string, error) {
	if unixTime < 0 || unixTime > 0xFFFFFFFF {
		return "", fmt.Errorf("the time %d does not fit the device's 32 bit clock", unixTime)
	}
	return c.request(fmt.Sprintf("TOKEN %d %d", id, unixTime))
}

// Wipe erases all keys (needs the long hold on the device).
func (c *Client) Wipe() error {
	_, err := c.request("WIPE")
	return err
}

// Raw sends one protocol line and passes every reply line to onLine as it
// arrives (PENDING, then the final line; or the KEY lines up to END for LIST).
// Device errors are just reply lines here; the returned error means that the
// transport failed or the device did not answer.
func (c *Client) Raw(command string, onLine func(string)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drain()
	if err := c.send(command); err != nil {
		return err
	}
	line, err := c.readLine(c.ReplyTimeout)
	if err != nil {
		return err
	}
	if command == "LIST" {
		for line != "END" {
			onLine(line)
			if !strings.HasPrefix(line, "KEY ") {
				return nil // an error line instead of a list
			}
			if line, err = c.readLine(c.ReplyTimeout); err != nil {
				return err
			}
		}
		onLine(line)
		return nil
	}
	if line == "PENDING" {
		onLine(line)
		if line, err = c.readLine(c.ConfirmTimeout); err != nil {
			return err
		}
	}
	onLine(line)
	return nil
}
