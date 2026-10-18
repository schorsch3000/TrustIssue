package main

import (
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"trustissues/trustctl/internal/protocol"
)

const migrationPrefix = "otpauth-migration://"

// account is one entry of a Google Authenticator export (its OtpParameters message).
type account struct {
	secret    []byte
	name      string // the label as Authenticator has it: "Issuer:account" or "account"
	issuer    string
	algorithm string // "" when the export names one this code does not know
	digits    int    // 0 when unknown
	kind      string // "TOTP", "HOTP" or ""
}

// batch is what one QR code of an export carries.
type batch struct {
	accounts []account
	size     int // how many QR codes the export has (0 or 1: just this one)
	index    int
	id       int64
}

// unsupported says why the device cannot take the account, or "" when it can.
func (a account) unsupported() string {
	switch {
	case a.kind != "TOTP":
		if a.kind == "" {
			return "unknown type, only TOTP is supported"
		}
		return a.kind + ", only TOTP is supported"
	case a.algorithm != "SHA1":
		if a.algorithm == "" {
			return "unknown algorithm, the device supports SHA1 only"
		}
		return a.algorithm + ", the device supports SHA1 only"
	case a.digits < 6 || a.digits > 8:
		return "unsupported number of digits"
	case len(a.secret) == 0:
		return "no secret"
	case len(a.secret) > protocol.MaxSecretBytes:
		return fmt.Sprintf("the secret has %d bytes, the device stores at most %d", len(a.secret), protocol.MaxSecretBytes)
	}
	return ""
}

// defaultName is the name `trustctl add` would derive from the label (protocol.DeriveName), except
// that a name that is too long is returned as it is: the user shortens it in the window.
func (a account) defaultName() string {
	label := a.name
	switch {
	case strings.Contains(label, ":"):
		issuer, rest, _ := strings.Cut(label, ":")
		label = strings.TrimSpace(issuer) + ":" + strings.TrimSpace(rest)
	case a.issuer != "" && label != "" && !strings.HasPrefix(strings.ToLower(label), strings.ToLower(a.issuer)):
		label = a.issuer + ":" + label
	}
	if strings.Trim(label, ": ") == "" {
		label = a.issuer
	}
	var b strings.Builder
	for _, r := range label {
		if r >= 0x20 && r <= 0x7E {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := strings.TrimSpace(b.String())
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	return name
}

// uri is the otpauth:// URI that `trustctl add` reads from stdin. The name is passed to it separately.
func (a account) uri() string {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(a.secret)
	label := a.defaultName()
	if label == "" {
		label = "key"
	}
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("digits", fmt.Sprint(a.digits))
	q.Set("algorithm", a.algorithm)
	q.Set("period", "30") // the export has no period; Authenticator uses 30
	if a.issuer != "" {
		q.Set("issuer", a.issuer)
	}
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}

// parseMigration decodes an otpauth-migration:// URI.
func parseMigration(text string) (batch, error) {
	var b batch
	text = strings.TrimSpace(text)
	if len(text) < len(migrationPrefix) || !strings.EqualFold(text[:len(migrationPrefix)], migrationPrefix) {
		return b, errors.New("this is not a Google Authenticator export code")
	}
	u, err := url.Parse(text)
	if err != nil || u.Query().Get("data") == "" {
		return b, errors.New("the export code has no data")
	}
	raw := strings.ReplaceAll(u.Query().Get("data"), " ", "+") // a "+" that came through as a space
	raw = strings.TrimRight(raw, "=")
	payload, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		if payload, err = base64.RawURLEncoding.DecodeString(raw); err != nil {
			return b, errors.New("the export code is damaged (bad base64)")
		}
	}
	err = eachField(payload, func(number uint64, wire int, num uint64, data []byte) error {
		switch {
		case number == 1 && wire == 2:
			a, err := parseAccount(data)
			if err != nil {
				return err
			}
			b.accounts = append(b.accounts, a)
		case number == 3 && wire == 0:
			b.size = int(num)
		case number == 4 && wire == 0:
			b.index = int(num)
		case number == 5 && wire == 0:
			b.id = int64(num)
		}
		return nil
	})
	if err != nil {
		return b, fmt.Errorf("the export code is damaged (%v)", err)
	}
	if len(b.accounts) == 0 {
		return b, errors.New("the export code holds no accounts")
	}
	return b, nil
}

var (
	algorithms = map[uint64]string{0: "SHA1", 1: "SHA1", 2: "SHA256", 3: "SHA512", 4: "MD5"} // 0: unspecified
	otpKinds   = map[uint64]string{0: "TOTP", 1: "HOTP", 2: "TOTP"}                          // 0: unspecified
	digitCount = map[uint64]int{0: 6, 1: 6, 2: 8}
)

func parseAccount(data []byte) (account, error) {
	var a account
	err := eachField(data, func(number uint64, wire int, num uint64, bytes []byte) error {
		switch {
		case number == 1 && wire == 2:
			a.secret = append([]byte(nil), bytes...)
		case number == 2 && wire == 2:
			a.name = strings.ToValidUTF8(string(bytes), "?")
		case number == 3 && wire == 2:
			a.issuer = strings.ToValidUTF8(string(bytes), "?")
		case number == 4 && wire == 0:
			a.algorithm = algorithms[num]
		case number == 5 && wire == 0:
			a.digits = digitCount[num]
		case number == 6 && wire == 0:
			a.kind = otpKinds[num]
		}
		return nil
	})
	return a, err
}

// eachField walks the fields of a protobuf message. For varints num is the value, for
// length-delimited fields data is the content; fixed-size fields are skipped.
func eachField(msg []byte, fn func(number uint64, wire int, num uint64, data []byte) error) error {
	for len(msg) > 0 {
		key, n := uvarint(msg)
		if n <= 0 {
			return errors.New("bad number")
		}
		msg = msg[n:]
		number, wire := key>>3, int(key&7)
		switch wire {
		case 0:
			v, n := uvarint(msg)
			if n <= 0 {
				return errors.New("bad number")
			}
			msg = msg[n:]
			if err := fn(number, wire, v, nil); err != nil {
				return err
			}
		case 2:
			l, n := uvarint(msg)
			if n <= 0 || l > uint64(len(msg)-n) {
				return errors.New("a field runs past the end")
			}
			data := msg[n : n+int(l)]
			msg = msg[n+int(l):]
			if err := fn(number, wire, 0, data); err != nil {
				return err
			}
		case 1:
			if len(msg) < 8 {
				return errors.New("a field runs past the end")
			}
			msg = msg[8:]
		case 5:
			if len(msg) < 4 {
				return errors.New("a field runs past the end")
			}
			msg = msg[4:]
		default:
			return fmt.Errorf("unknown field type %d", wire)
		}
	}
	return nil
}

func uvarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7F) << (7 * uint(i))
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, -1
}

// collector gathers the QR codes of one export (Google splits big exports over several).
type collector struct {
	id    int64
	size  int
	parts map[int][]account
}

func newCollector() *collector { return &collector{parts: map[int][]account{}} }

// add takes a batch and reports whether the export is complete.
func (c *collector) add(b batch) bool {
	if len(c.parts) > 0 && b.id != c.id { // another export: start over
		c.parts = map[int][]account{}
	}
	c.id = b.id
	c.size = max(b.size, 1)
	c.parts[b.index] = b.accounts
	return c.complete()
}

func (c *collector) complete() bool { return len(c.parts) > 0 && len(c.parts) >= c.size }

func (c *collector) progress() (have, size int) { return len(c.parts), max(c.size, 1) }

func (c *collector) accounts() []account {
	var order []int
	for i := range c.parts {
		order = append(order, i)
	}
	sort.Ints(order)
	var all []account
	for _, i := range order {
		all = append(all, c.parts[i]...)
	}
	return all
}
