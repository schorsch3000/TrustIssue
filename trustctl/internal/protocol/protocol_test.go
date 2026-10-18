package protocol_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"trustissue/trustctl/internal/protocol"
	"trustissue/trustctl/internal/sim"
)

// RFC 6238 secret "12345678901234567890"
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func newClient(t *testing.T, decide sim.Decider) (*protocol.Client, *sim.Device) {
	t.Helper()
	conn, dev := sim.New(decide)
	t.Cleanup(func() { conn.Close() })
	c := protocol.NewClient(conn)
	if _, err := c.WaitBanner(time.Second); err != nil {
		t.Fatal(err)
	}
	return c, dev
}

func TestAddListTokenDelete(t *testing.T) {
	var pending []string
	c, _ := newClient(t, sim.AcceptAll)
	c.OnPending = func() { pending = append(pending, "pending") }

	id, err := c.Add("GitHub", rfcSecret, 6, 30)
	if err != nil || id != 0 {
		t.Fatalf("Add: %d %v", id, err)
	}
	if _, err := c.Add("AWS root", "gezd gnbv-gy3t qojq gezd gnbv gy3t qojq", 8, 30); err != nil {
		t.Fatalf("Add with a spaced lower case secret: %v", err)
	}
	keys, err := c.List()
	if err != nil || len(keys) != 2 || keys[1].Name != "AWS root" || keys[1].ID != 1 {
		t.Fatalf("List: %v %v", keys, err)
	}
	code, err := c.Token(0, 59)
	if err != nil || code != "287082" {
		t.Fatalf("Token: %q %v", code, err)
	}
	code, err = c.Token(1, 1111111109)
	if err != nil || code != "07081804" {
		t.Fatalf("Token with 8 digits: %q %v", code, err)
	}
	if err := c.Delete(0); err != nil {
		t.Fatal(err)
	}
	if keys, _ = c.List(); len(keys) != 1 || keys[0].ID != 1 {
		t.Fatalf("List after delete: %v", keys)
	}
	if len(pending) != 5 { // 2 adds, 2 tokens, 1 delete
		t.Fatalf("OnPending was called %d times, want 5", len(pending))
	}
}

func TestDeviceErrors(t *testing.T) {
	outcome := sim.Accept
	c, _ := newClient(t, func(string, string) sim.Outcome { return outcome })
	if _, err := c.Add("GitHub", rfcSecret, 6, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Add("github", rfcSecret, 6, 30); !errors.Is(err, protocol.ErrDuplicate) {
		t.Errorf("duplicate name: %v", err)
	}
	if _, err := c.Token(7, 59); !errors.Is(err, protocol.ErrNotFound) {
		t.Errorf("unknown key: %v", err)
	}
	outcome = sim.Deny
	if _, err := c.Token(0, 59); !errors.Is(err, protocol.ErrDenied) {
		t.Errorf("denied: %v", err)
	}
	outcome = sim.Timeout
	if _, err := c.Token(0, 59); !errors.Is(err, protocol.ErrTimeout) {
		t.Errorf("timeout: %v", err)
	}
	if err := c.Delete(0); !errors.Is(err, protocol.ErrTimeout) {
		t.Errorf("delete timeout: %v", err)
	}
	if keys, _ := c.List(); len(keys) != 1 {
		t.Errorf("a denied or timed out delete must keep the key: %v", keys)
	}
}

func TestFullAndWipe(t *testing.T) {
	c, dev := newClient(t, sim.AcceptAll)
	for i := 0; i < protocol.MaxKeys; i++ {
		if _, err := c.Add("Key "+string(rune('A'+i)), rfcSecret, 6, 30); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if _, err := c.Add("One too many", rfcSecret, 6, 30); !errors.Is(err, protocol.ErrFull) {
		t.Errorf("full: %v", err)
	}
	if err := c.Wipe(); err != nil {
		t.Fatal(err)
	}
	for _, name := range dev.Keys() {
		if name != "" {
			t.Fatalf("wipe left %q", name)
		}
	}
	if err := c.Wipe(); err != nil { // nothing to wipe: no confirmation needed
		t.Fatal(err)
	}
}

func TestTimezoneAndConfig(t *testing.T) {
	c, _ := newClient(t, sim.AcceptAll)
	if q, err := c.Timezone(); err != nil || q != 0 {
		t.Fatalf("Timezone: %d %v", q, err)
	}
	if err := c.SetTimezone(8); err != nil {
		t.Fatal(err)
	}
	if q, _ := c.Timezone(); q != 8 {
		t.Fatalf("Timezone after set: %d", q)
	}
	if err := c.SetTimezone(100); !errors.Is(err, protocol.ErrBadArg) {
		t.Errorf("out of range: %v", err)
	}
	if err := c.SetConfig(0x05); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Config(); v != 0x05 {
		t.Fatalf("Config: %02x", v)
	}
	if err := c.SetConfig(0x20); !errors.Is(err, protocol.ErrBadArg) {
		t.Errorf("reserved bit: %v", err)
	}
}

func TestClientValidatesBeforeSending(t *testing.T) {
	c, dev := newClient(t, func(string, string) sim.Outcome { t.Error("the device must not be asked"); return sim.Deny })
	for _, name := range []string{"", "Bad\x01Name", "Bad\x7fName", " lead", "trail ", "two  spaces", "Müller", strings.Repeat("x", 28)} {
		if _, err := c.Add(name, rfcSecret, 6, 30); err == nil {
			t.Errorf("name %q was accepted", name)
		}
	}
	if _, err := c.Add("ok", "not base32!", 6, 30); err == nil {
		t.Error("a bad secret was accepted")
	}
	if _, err := c.Add("ok", strings.Repeat("A", 40), 6, 30); err == nil {
		t.Error("a 25 byte secret was accepted")
	}
	if _, err := c.Token(0, -1); err == nil {
		t.Error("a negative time was accepted")
	}
	for _, name := range dev.Keys() {
		if name != "" {
			t.Fatal("something was stored")
		}
	}
}

func TestParseOTPAuth(t *testing.T) {
	a, err := protocol.ParseOTPAuth("otpauth://totp/GitHub:alice%40example.com?secret=gezd%20gnbv&issuer=GitHub&digits=8&period=60")
	if err != nil {
		t.Fatal(err)
	}
	if a.Secret != "GEZDGNBV" || a.Issuer != "GitHub" || a.Account != "alice@example.com" || a.Digits != 8 || a.Period != 60 {
		t.Errorf("parsed %+v", a)
	}
	a, err = protocol.ParseOTPAuth("otpauth://totp/user?secret=MFRGGZDF")
	if err != nil || a.Digits != 6 || a.Period != 30 || a.Account != "user" {
		t.Errorf("defaults: %+v %v", a, err)
	}
	for _, bad := range []string{
		"https://example.com", "otpauth://hotp/x?secret=MFRGGZDF", "otpauth://totp/x",
		"otpauth://totp/x?secret=MFRGGZDF&algorithm=SHA256", "otpauth://totp/x?secret=MFRGGZDF&digits=5",
		"otpauth://totp/x?secret=MFRGGZDF&period=45",
	} {
		if _, err := protocol.ParseOTPAuth(bad); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
}
