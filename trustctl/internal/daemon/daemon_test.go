package daemon_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"trustissues/trustctl/internal/daemon"
	"trustissues/trustctl/internal/protocol"
	"trustissues/trustctl/internal/sim"
)

const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

type rig struct {
	dev     *sim.Device
	result  chan string
	socket  string
	release chan struct{}
}

// start runs a daemon in front of a simulated device. Requests that need the
// button wait for rig.release when block is set.
func start(t *testing.T, idle time.Duration, block bool) *rig {
	t.Helper()
	dir, err := os.MkdirTemp("", "tc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	r := &rig{result: make(chan string, 1), socket: filepath.Join(dir, "s"), release: make(chan struct{})}
	conn, dev := sim.New(func(string, string) sim.Outcome {
		if block {
			<-r.release
		}
		return sim.Accept
	})
	r.dev = dev
	device := protocol.NewClient(conn)
	if _, err := device.WaitBanner(time.Second); err != nil {
		t.Fatal(err)
	}
	go func() {
		reason, err := daemon.Run(device, daemon.Options{SocketPath: r.socket, Idle: idle, Banner: "TrustIssue 0.1.0 proto 1", DevicePath: "sim"})
		if err != nil {
			reason = "error: " + err.Error()
		}
		r.result <- reason
	}()
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("unix", r.socket); err == nil {
			c.Close()
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the daemon did not listen")
	return nil
}

func (r *rig) client(t *testing.T) *protocol.Client {
	t.Helper()
	conn, err := net.Dial("unix", r.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := protocol.NewClient(conn)
	if _, err := c.WaitBanner(time.Second); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCacheAndInvalidation(t *testing.T) {
	r := start(t, time.Minute, false)
	c := r.client(t)
	for i := 0; i < 3; i++ {
		if keys, err := c.List(); err != nil || len(keys) != 0 {
			t.Fatalf("List: %v %v", keys, err)
		}
	}
	if n := r.dev.Count("LIST"); n != 1 {
		t.Fatalf("the device was asked LIST %d times, want 1 (cached)", n)
	}
	c.Timezone()
	c.Timezone()
	if n := r.dev.Count("GETTZ"); n != 1 {
		t.Fatalf("GETTZ reached the device %d times, want 1", n)
	}
	if _, err := c.Add("GitHub", secret, 6, 30); err != nil {
		t.Fatal(err)
	}
	keys, err := c.List()
	if err != nil || len(keys) != 1 || keys[0].Name != "GitHub" {
		t.Fatalf("List after ADD must not be stale: %v %v", keys, err)
	}
	if err := c.SetTimezone(8); err != nil {
		t.Fatal(err)
	}
	if q, _ := c.Timezone(); q != 8 {
		t.Fatalf("GETTZ after SETTZ must not be stale: %d", q)
	}
	if code, err := c.Token(0, 59); err != nil || code != "287082" {
		t.Fatalf("Token: %q %v", code, err)
	}
	if err := c.Delete(0); err != nil {
		t.Fatal(err)
	}
	if keys, _ := c.List(); len(keys) != 0 {
		t.Fatalf("List after DEL must not be stale: %v", keys)
	}
	// a second client sees the same state
	if q, _ := r.client(t).Timezone(); q != 8 {
		t.Fatalf("second client: %d", q)
	}
}

func TestPendingIsStreamed(t *testing.T) {
	r := start(t, time.Minute, false)
	c := r.client(t)
	pending := false
	c.OnPending = func() { pending = true }
	if _, err := c.Add("GitHub", secret, 6, 30); err != nil || !pending {
		t.Fatalf("Add: %v, pending seen: %v", err, pending)
	}
}

func TestBusyWhileAnotherRequestWaitsForTheButton(t *testing.T) {
	r := start(t, time.Minute, true)
	first := r.client(t)
	second := r.client(t)
	done := make(chan error, 1)
	go func() { _, err := first.Add("GitHub", secret, 6, 30); done <- err }()
	time.Sleep(300 * time.Millisecond) // the device is now waiting for the button
	if _, err := second.List(); !errors.Is(err, protocol.ErrBusy) {
		t.Fatalf("a second client during a pending request: %v", err)
	}
	close(r.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if keys, err := second.List(); err != nil || len(keys) != 1 {
		t.Fatalf("after the confirmation: %v %v", keys, err)
	}
}

func TestStopAndSocketCleanup(t *testing.T) {
	r := start(t, time.Minute, false)
	c := r.client(t)
	if err := c.Raw("!STATUS", func(l string) {
		if !strings.HasPrefix(l, "OK device=sim") {
			t.Errorf("status: %q", l)
		}
	}); err != nil {
		t.Fatal(err)
	}
	c.Raw("!STOP", func(string) {})
	select {
	case reason := <-r.result:
		if !strings.Contains(reason, "stopped") {
			t.Fatalf("reason: %q", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the daemon did not stop")
	}
	if _, err := os.Stat(r.socket); err == nil {
		t.Fatal("the socket file was left behind")
	}
}

func TestIdleTimeout(t *testing.T) {
	r := start(t, 1200*time.Millisecond, false)
	select {
	case reason := <-r.result:
		if !strings.Contains(reason, "idle") {
			t.Fatalf("reason: %q", reason)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("the daemon did not time out")
	}
}

func TestDeviceDisconnectEndsTheDaemon(t *testing.T) {
	dir, _ := os.MkdirTemp("", "tc")
	defer os.RemoveAll(dir)
	conn, _ := sim.New(sim.AcceptAll)
	device := protocol.NewClient(conn)
	device.WaitBanner(time.Second)
	result := make(chan string, 1)
	go func() {
		reason, _ := daemon.Run(device, daemon.Options{SocketPath: filepath.Join(dir, "s"), Idle: time.Minute, Banner: "x proto 1"})
		result <- reason
	}()
	time.Sleep(200 * time.Millisecond)
	conn.Close()
	select {
	case reason := <-result:
		if !strings.Contains(reason, "disconnected") {
			t.Fatalf("reason: %q", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the daemon did not notice the disconnect")
	}
}

func TestSecondDaemonRefused(t *testing.T) {
	r := start(t, time.Minute, false)
	conn, _ := sim.New(sim.AcceptAll)
	device := protocol.NewClient(conn)
	device.WaitBanner(time.Second)
	if _, err := daemon.Run(device, daemon.Options{SocketPath: r.socket, Banner: "x proto 1"}); err == nil {
		t.Fatal("a second daemon on the same socket was accepted")
	}
}
