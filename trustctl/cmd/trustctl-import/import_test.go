package main

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"trustissues/trustctl/internal/protocol"
)

// ---- a protobuf encoder for the tests

func varint(n uint64) []byte {
	var out []byte
	for {
		b := byte(n & 0x7F)
		n >>= 7
		if n != 0 {
			out = append(out, b|0x80)
		} else {
			return append(out, b)
		}
	}
}

func fieldNum(number, value uint64) []byte { return append(varint(number<<3), varint(value)...) }

func fieldBytes(number uint64, value []byte) []byte {
	return append(append(varint(number<<3|2), varint(uint64(len(value)))...), value...)
}

type acct struct {
	secret       []byte
	name, issuer string
	algorithm    uint64
	digits       uint64
	kind         uint64
}

func encode(a acct) []byte {
	if a.algorithm == 0 {
		a.algorithm = 1
	}
	if a.digits == 0 {
		a.digits = 1
	}
	if a.kind == 0 {
		a.kind = 2
	}
	msg := fieldBytes(1, a.secret)
	msg = append(msg, fieldBytes(2, []byte(a.name))...)
	msg = append(msg, fieldBytes(3, []byte(a.issuer))...)
	msg = append(msg, fieldNum(4, a.algorithm)...)
	msg = append(msg, fieldNum(5, a.digits)...)
	msg = append(msg, fieldNum(6, a.kind)...)
	return fieldBytes(1, msg)
}

func export(size, index int, id uint64, accounts ...acct) string {
	var payload []byte
	for _, a := range accounts {
		payload = append(payload, encode(a)...)
	}
	payload = append(payload, fieldNum(2, 1)...)
	payload = append(payload, fieldNum(3, uint64(size))...)
	payload = append(payload, fieldNum(4, uint64(index))...)
	payload = append(payload, fieldNum(5, id)...)
	return migrationPrefix + "offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(payload))
}

var secret = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

func TestParseMigration(t *testing.T) {
	b, err := parseMigration(export(1, 0, 7,
		acct{secret: secret, name: "alice@example.com", issuer: "GitHub"},
		acct{secret: secret, name: "GitHub:bob", issuer: "GitHub", digits: 2},
		acct{secret: secret, name: "hotp", issuer: "X", kind: 1},
		acct{secret: secret, name: "sha256", issuer: "X", algorithm: 2},
		acct{secret: bytes.Repeat([]byte{'A'}, 21), name: "long", issuer: "X"},
		acct{secret: secret, name: "Müller", issuer: "Café"},
	))
	if err != nil {
		t.Fatal(err)
	}
	a := b.accounts
	if len(a) != 6 || b.id != 7 || b.size != 1 {
		t.Fatalf("got %d accounts, id %d, size %d", len(a), b.id, b.size)
	}
	if !bytes.Equal(a[0].secret, secret) || a[0].algorithm != "SHA1" || a[0].digits != 6 || a[0].kind != "TOTP" {
		t.Errorf("first account: %+v", a[0])
	}
	if a[1].digits != 8 {
		t.Errorf("digits enum 2 should be 8, got %d", a[1].digits)
	}
	for i, want := range []string{"", "", "HOTP", "SHA256", "21 bytes"} {
		got := a[i].unsupported()
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("account %d unsupported() = %q, want it to mention %q", i, got, want)
		}
	}
	for i, want := range []string{"GitHub:alice@example.com", "GitHub:bob", "", "", "", "Caf-:M-ller"} {
		if want != "" && a[i].defaultName() != want {
			t.Errorf("account %d name %q, want %q", i, a[i].defaultName(), want)
		}
	}
}

// The names must be what trustctl itself derives, whenever it can derive one.
func TestDefaultNameMatchesTrustctl(t *testing.T) {
	for _, c := range []struct{ name, issuer string }{
		{"alice@example.com", "GitHub"}, {"GitHub:bob", "GitHub"}, {"GitHub-schorsch", "GitHub"},
		{"Müller", "Café"}, {"a  b", ""}, {"  spaced : out ", "Iss"}, {"", "OnlyIssuer"}, {"x", ""},
	} {
		want, _, err := (&protocol.OTPAuth{Issuer: c.issuer, Label: c.name}).DeriveName()
		if err != nil {
			continue
		}
		if got := (account{name: c.name, issuer: c.issuer}).defaultName(); got != want {
			t.Errorf("%+v: got %q, trustctl derives %q", c, got, want)
		}
	}
	long := account{name: strings.Repeat("x", 40)}.defaultName()
	if len(long) != 40 {
		t.Errorf("a long name must stay long for the user to shorten, got %d characters", len(long))
	}
}

func TestURIIsAcceptedByTrustctlsParser(t *testing.T) {
	a := account{secret: secret, name: "GitHub:bob", issuer: "GitHub", algorithm: "SHA1", digits: 8, kind: "TOTP"}
	parsed, err := protocol.ParseOTPAuth(a.uri())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Secret != strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "=") ||
		parsed.Digits != 8 || parsed.Period != 30 || parsed.Algorithm != "SHA1" {
		t.Errorf("parsed: %+v", parsed)
	}
}

func TestCollector(t *testing.T) {
	first, _ := parseMigration(export(2, 0, 5, acct{secret: secret, name: "one", issuer: "A"}))
	second, _ := parseMigration(export(2, 1, 5, acct{secret: secret, name: "two", issuer: "B"}))
	other, _ := parseMigration(export(2, 0, 6, acct{secret: secret, name: "x", issuer: "C"}))
	c := newCollector()
	if c.add(first) {
		t.Error("one of two codes is not complete")
	}
	if c.add(first) {
		t.Error("the same code again changes nothing")
	}
	if have, size := c.progress(); have != 1 || size != 2 {
		t.Errorf("progress %d of %d", have, size)
	}
	if !c.add(second) || len(c.accounts()) != 2 || c.accounts()[0].name != "one" || c.accounts()[1].name != "two" {
		t.Error("the second code should complete it, in order")
	}
	c = newCollector()
	c.add(first)
	if c.add(other) {
		t.Error("a code of another export must start over")
	}
	single, _ := parseMigration(export(0, 0, 0, acct{secret: secret, name: "s", issuer: "S"}))
	if !newCollector().add(single) {
		t.Error("a code without batch fields is complete at once")
	}
}

func TestParseMigrationErrors(t *testing.T) {
	for name, text := range map[string]string{
		"another URL":        "https://example.com",
		"no data":            migrationPrefix + "offline",
		"bad base64":         migrationPrefix + "offline?data=%25%25%25",
		"truncated protobuf": migrationPrefix + "offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte{0x0a, 0xff})),
		"no accounts":        migrationPrefix + "offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(fieldNum(2, 1))),
		"empty":              "",
	} {
		if _, err := parseMigration(text); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// ---- the server

const testHost = "127.0.0.1:4711"

func request(t *testing.T, s *server, method, path string, body any, mod func(*http.Request)) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Host = testHost
	r.Header.Set("X-Token", s.token)
	if mod != nil {
		mod(r)
	}
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	res := w.Result()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	json.Unmarshal(raw, &out)
	return res, out
}

// fakeTrustctl writes a script that records what it was given and acts on the name.
func fakeTrustctl(t *testing.T) (command []string, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"cat >> " + log + "\n" +
		"case \"$*\" in\n" +
		"  *denied*) echo 'trustctl: denied on the device' >&2; exit 2 ;;\n" +
		"  *dup*) echo 'trustctl: the device has that name already' >&2; exit 1 ;;\n" +
		"  *nodev*) echo 'trustctl: no device' >&2; exit 69 ;;\n" +
		"esac\n"
	path := filepath.Join(dir, "trustctl")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return []string{path, "--simulate"}, log
}

func scanned(t *testing.T, s *server, text string) map[string]any {
	t.Helper()
	res, out := request(t, s, "POST", "/api/scan", map[string]string{"text": text}, nil)
	if res.StatusCode != 200 {
		t.Fatalf("scan: %d %v", res.StatusCode, out)
	}
	return out
}

func TestServerGuards(t *testing.T) {
	s := newServer("tok", testHost, []string{"unused"})
	for name, c := range map[string]struct {
		path string
		mod  func(*http.Request)
		want int
	}{
		"state with the token":       {"/api/state", nil, 200},
		"state without a token":      {"/api/state", func(r *http.Request) { r.Header.Del("X-Token") }, 403},
		"state with a wrong token":   {"/api/state", func(r *http.Request) { r.Header.Set("X-Token", "nope") }, 403},
		"a foreign Host (rebinding)": {"/api/state", func(r *http.Request) { r.Host = "evil.example:4711" }, 403},
		"a foreign Origin":           {"/api/state", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, 403},
		"our own Origin":             {"/api/state", func(r *http.Request) { r.Header.Set("Origin", "http://"+testHost) }, 200},
		"the page with its token":    {"/?t=tok", nil, 200},
		"the page without a token":   {"/", nil, 403},
		"the page, foreign Host":     {"/?t=tok", func(r *http.Request) { r.Host = "evil.example" }, 403},
		"the scripts need no token":  {"/app.js", func(r *http.Request) { r.Header.Del("X-Token") }, 200},
	} {
		if res, _ := request(t, s, "GET", c.path, nil, c.mod); res.StatusCode != c.want {
			t.Errorf("%s: %d, want %d", name, res.StatusCode, c.want)
		}
	}
	res, _ := request(t, s, "GET", "/jsQR.js", nil, nil)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
		t.Errorf("jsQR.js: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res, _ := request(t, s, "POST", "/api/scan", map[string]string{"text": "x"}, func(r *http.Request) { r.Header.Del("X-Token") }); res.StatusCode != 403 {
		t.Errorf("scan without a token: %d", res.StatusCode)
	}
}

func TestScanImportFlow(t *testing.T) {
	command, log := fakeTrustctl(t)
	s := newServer("tok", testHost, command)

	if _, out := request(t, s, "POST", "/api/scan", map[string]string{"text": "https://example.com"}, nil); out["error"] == nil {
		t.Error("a foreign QR code should be an error")
	}
	first := scanned(t, s, export(2, 0, 9, acct{secret: secret, name: "alice@example.com", issuer: "GitHub"}))
	if first["phase"] != "scan" || first["have"].(float64) != 1 || first["size"].(float64) != 2 {
		t.Fatalf("after one of two codes: %v", first)
	}
	done := scanned(t, s, export(2, 1, 9,
		acct{secret: secret, name: "bob", issuer: "AWS"},
		acct{secret: secret, name: "legacy", issuer: "X", kind: 1},
		acct{secret: secret, name: "dup", issuer: ""},
	))
	if done["phase"] != "review" {
		t.Fatalf("after both codes: %v", done)
	}
	raw, _ := json.Marshal(done)
	for _, leak := range []string{"AQIDBAUGBwgJCg", base32.StdEncoding.EncodeToString(secret)[:10], "secret", "otpauth"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the page's state contains %q: %s", leak, raw)
		}
	}
	list := done["accounts"].([]any)
	if len(list) != 4 || list[0].(map[string]any)["name"] != "GitHub:alice@example.com" ||
		list[2].(map[string]any)["supported"] != false {
		t.Fatalf("accounts: %v", list)
	}

	// bad names, a duplicate, an unsupported account: refused, nothing run
	res, out := request(t, s, "POST", "/api/import", map[string]any{"items": []map[string]any{
		{"id": 0, "name": "same"}, {"id": 1, "name": "SAME"}, {"id": 2, "name": "x"}, {"id": 3, "name": strings.Repeat("y", 28)}}}, nil)
	problems, _ := out["problems"].(map[string]any)
	if res.StatusCode != 400 || problems["1"] == nil || problems["2"] == nil || problems["3"] == nil || problems["0"] != nil {
		t.Errorf("bad import: %d %v", res.StatusCode, out)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("trustctl ran although the request was refused")
	}

	// a good import: the first stored, the third reports an error from trustctl, then it goes on
	res, out = request(t, s, "POST", "/api/import", map[string]any{"items": []map[string]any{
		{"id": 0, "name": "GitHub:alice"}, {"id": 3, "name": "dup"}}}, nil)
	if res.StatusCode != 202 {
		t.Fatalf("import: %d %v", res.StatusCode, out)
	}
	state := waitIdle(t, s)
	rows := state["rows"].(map[string]any)
	if rows["0"].(map[string]any)["state"] != "ok" || rows["3"].(map[string]any)["state"] != "error" ||
		!strings.Contains(rows["3"].(map[string]any)["text"].(string), "already") {
		t.Errorf("rows: %v", rows)
	}
	logged, _ := os.ReadFile(log)
	if !strings.Contains(string(logged), "--simulate add GitHub:alice\n") ||
		!strings.Contains(string(logged), "otpauth://totp/GitHub:alice@example.com?") ||
		!strings.Contains(string(logged), "secret="+strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "=")) {
		t.Errorf("trustctl got: %q", logged)
	}

	// the stored one is not stored twice; the failed one can be retried under another name
	res, _ = request(t, s, "POST", "/api/import", map[string]any{"items": []map[string]any{
		{"id": 0, "name": "GitHub:alice"}, {"id": 3, "name": "other"}}}, nil)
	waitIdle(t, s)
	logged, _ = os.ReadFile(log)
	if res.StatusCode != 202 || strings.Count(string(logged), "add GitHub:alice") != 1 || !strings.Contains(string(logged), "add other") {
		t.Errorf("retry: %d %q", res.StatusCode, logged)
	}
	if st := s.view(); !st.Done[0] || !st.Done[3] {
		t.Errorf("done: %v", st.Done)
	}

	// scan again clears everything
	if res, _ := request(t, s, "POST", "/api/reset", map[string]any{}, nil); res.StatusCode != 200 {
		t.Errorf("reset: %d", res.StatusCode)
	}
	if st := s.view(); st.Phase != "scan" || len(st.Done) != 0 || len(st.Rows) != 0 {
		t.Errorf("after reset: %+v", st)
	}
}

func TestDenialStopsTheRest(t *testing.T) {
	command, log := fakeTrustctl(t)
	s := newServer("tok", testHost, command)
	scanned(t, s, export(1, 0, 1,
		acct{secret: secret, name: "a", issuer: "A"}, acct{secret: secret, name: "denied", issuer: "B"}, acct{secret: secret, name: "c", issuer: "C"}))
	request(t, s, "POST", "/api/import", map[string]any{"items": []map[string]any{
		{"id": 0, "name": "one"}, {"id": 1, "name": "denied"}, {"id": 2, "name": "three"}}}, nil)
	rows := waitIdle(t, s)["rows"].(map[string]any)
	if rows["0"].(map[string]any)["state"] != "ok" || rows["1"].(map[string]any)["state"] != "denied" || rows["2"] != nil {
		t.Errorf("rows: %v", rows)
	}
	if logged, _ := os.ReadFile(log); strings.Contains(string(logged), "three") {
		t.Error("the key after a denial was sent")
	}
}

func TestNoDeviceStops(t *testing.T) {
	command, _ := fakeTrustctl(t)
	var states []string
	ok := storeKeys(command, []job{{id: 0, name: "nodev", uri: "u"}, {id: 1, name: "x", uri: "u"}},
		func(id int, state, text string) { states = append(states, state) })
	if ok || strings.Join(states, ",") != "working,error" {
		t.Errorf("ok %v, states %v", ok, states)
	}
	ok = storeKeys([]string{"/nonexistent/trustctl"}, []job{{id: 0, name: "x", uri: "u"}},
		func(id int, state, text string) { states = append(states, state+":"+text) })
	if ok || !strings.Contains(states[len(states)-1], "error:") {
		t.Errorf("a missing binary: %v", states)
	}
}

func waitIdle(t *testing.T, s *server) map[string]any {
	t.Helper()
	for i := 0; i < 400; i++ {
		_, out := request(t, s, "GET", "/api/state", nil, nil)
		if out["running"] == false {
			return out
		}
		sleep()
	}
	t.Fatal("the import did not finish")
	return nil
}
