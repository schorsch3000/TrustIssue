package daemon

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"trustissues/trustctl/internal/protocol"
)

// Options configure Run.
type Options struct {
	SocketPath string
	Idle       time.Duration // exit after this long without a command
	Banner     string        // what the device said about itself
	DevicePath string
	Logf       func(format string, args ...any)
}

type server struct {
	dev  *protocol.Client
	opt  Options
	stop chan string
	once sync.Once

	busy sync.Mutex // one request on the device at a time

	mu      sync.Mutex
	last    time.Time
	clients int
	list    []string // cached LIST reply lines, nil if unknown
	tz      string   // cached GETTZ reply, "" if unknown
	cfg     string   // cached GETCFG reply
}

func (s *server) shutdown(reason string) {
	s.once.Do(func() { s.stop <- reason })
}

func (s *server) touch() {
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
}

func (s *server) invalidate() {
	s.mu.Lock()
	s.list, s.tz, s.cfg = nil, "", ""
	s.mu.Unlock()
}

// Run serves the device on a Unix socket until it is told to stop, has been
// idle for too long, the device goes away, or the machine was suspended
// (clean state matters more than a quick restart). It returns the reason.
func Run(dev *protocol.Client, opt Options) (string, error) {
	logf := opt.Logf
	if logf == nil {
		logf = func(string, ...any) {}
		opt.Logf = logf
	}
	listener, err := listen(opt.SocketPath)
	if err != nil {
		return "", err
	}
	defer os.Remove(opt.SocketPath)
	defer listener.Close()

	s := &server{dev: dev, opt: opt, stop: make(chan string, 1), last: time.Now()}
	go s.accept(listener)
	go func() {
		<-dev.Done()
		s.shutdown("the device was disconnected")
	}()

	// idle timeout and suspend detection
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous := time.Now().Round(0)
	for {
		select {
		case reason := <-s.stop:
			logf("stopping: %s", reason)
			return reason, nil
		case now := <-ticker.C:
			wall := now.Round(0) // without the monotonic clock, which stands still while suspended
			if wall.Sub(previous) > 5*time.Second {
				s.shutdown("the machine was suspended")
			}
			previous = wall
			s.mu.Lock()
			quiet := time.Since(s.last)
			active := s.clients > 0
			s.mu.Unlock()
			if opt.Idle > 0 && !active && quiet > opt.Idle {
				s.shutdown(fmt.Sprintf("idle for %v", opt.Idle))
			}
		}
	}
}

func listen(path string) (net.Listener, error) {
	if conn, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
		conn.Close()
		return nil, fmt.Errorf("another trustctl daemon is already listening on %s", path)
	}
	os.Remove(path) // a stale socket of a dead daemon
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return listener, nil
}

func (s *server) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

func (s *server) serve(conn net.Conn) {
	defer conn.Close()
	s.mu.Lock()
	s.clients++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.clients--
		s.mu.Unlock()
		s.touch()
	}()
	fmt.Fprintf(conn, "%s\n", s.opt.Banner)
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		s.touch()
		s.handle(strings.TrimRight(line, "\r\n"), conn)
	}
}

func (s *server) handle(line string, out io.Writer) {
	word, _, _ := strings.Cut(line, " ")
	say := func(text string) { fmt.Fprintln(out, text) }
	switch word {
	case "HELLO":
		say(s.opt.Banner)
		return
	case "!STATUS":
		s.mu.Lock()
		say(fmt.Sprintf("OK device=%s clients=%d idle=%v", s.opt.DevicePath, s.clients, s.opt.Idle))
		s.mu.Unlock()
		return
	case "!STOP":
		say("OK")
		s.shutdown("stopped by trustctl daemon stop")
		return
	}
	if !s.busy.TryLock() {
		say("ERR BUSY")
		return
	}
	defer s.busy.Unlock()

	// cached read-only answers
	s.mu.Lock()
	switch {
	case word == "LIST" && s.list != nil:
		cached := s.list
		s.mu.Unlock()
		for _, l := range cached {
			say(l)
		}
		return
	case word == "GETTZ" && s.tz != "":
		reply := s.tz
		s.mu.Unlock()
		say(reply)
		return
	case word == "GETCFG" && s.cfg != "":
		reply := s.cfg
		s.mu.Unlock()
		say(reply)
		return
	}
	s.mu.Unlock()

	changes := word == "ADD" || word == "DEL" || word == "WIPE" || word == "SETTZ" || word == "SETCFG"
	if changes {
		s.invalidate() // before and after: whatever the outcome, the device may have changed
		defer s.invalidate()
	}
	var reply []string
	err := s.dev.Raw(line, func(l string) {
		reply = append(reply, l)
		say(l) // stream: PENDING must reach the user before the final line
	})
	if err != nil {
		s.opt.Logf("device error for %q: %v", word, err)
		say("ERR NODEVICE")
		return
	}
	s.mu.Lock()
	switch {
	case word == "LIST" && len(reply) > 0 && reply[len(reply)-1] == "END":
		s.list = reply
	case word == "GETTZ" && len(reply) == 1 && strings.HasPrefix(reply[0], "OK "):
		s.tz = reply[0]
	case word == "GETCFG" && len(reply) == 1 && strings.HasPrefix(reply[0], "OK "):
		s.cfg = reply[0]
	}
	s.mu.Unlock()
}
