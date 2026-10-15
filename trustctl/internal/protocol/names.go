package protocol

import (
	"encoding/base32"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	// MaxNameLength is the longest key name the device stores.
	MaxNameLength = 27
	// MaxSecretBytes is the longest TOTP secret (decoded) the device stores.
	MaxSecretBytes = 20
	// MaxKeys is the number of key slots of the device.
	MaxKeys = 20
)

// ValidateName checks the key name rules of the device: 1 to 27 characters from
// A-Z a-z 0-9 . : - and space, no leading or trailing space, no double spaces.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("the name is empty")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("the name %q has %d characters, the device stores at most %d", name, len(name), MaxNameLength)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '.' || c == ':' || c == '-' || c == ' '
		if !ok {
			return fmt.Errorf("the name %q contains %q; allowed are letters A-Z a-z, digits, . : - and space (no accents, no emoji)", name, string(rune(c)))
		}
	}
	if name[0] == ' ' || name[len(name)-1] == ' ' {
		return fmt.Errorf("the name %q starts or ends with a space", name)
	}
	if strings.Contains(name, "  ") {
		return fmt.Errorf("the name %q contains two spaces in a row", name)
	}
	return nil
}

// NormalizeSecret cleans a Base32 TOTP secret as services show it (groups of
// letters with spaces or dashes, any case, optional padding) and checks that
// it decodes to at most MaxSecretBytes bytes. It returns the canonical form.
func NormalizeSecret(secret string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(secret) {
		switch {
		case r == ' ' || r == '-' || r == '\t':
		case r == '=':
		case r >= 'A' && r <= 'Z' || r >= '2' && r <= '7':
			b.WriteRune(r)
		default:
			return "", fmt.Errorf("the secret contains %q, which is not Base32 (letters A-Z and digits 2-7)", string(r))
		}
	}
	clean := b.String()
	if clean == "" {
		return "", fmt.Errorf("the secret is empty")
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
	if err != nil {
		return "", fmt.Errorf("the secret is not valid Base32: %v", err)
	}
	if len(decoded) > MaxSecretBytes {
		return "", fmt.Errorf("the secret is %d bytes, the device stores at most %d", len(decoded), MaxSecretBytes)
	}
	return clean, nil
}

// OTPAuth holds what an otpauth://totp/... URI (the content of a QR code) says.
type OTPAuth struct {
	Secret    string
	Issuer    string
	Account   string
	Algorithm string
	Digits    int
	Period    int
}

// ParseOTPAuth parses an otpauth URI. The device supports TOTP with SHA1,
// 6 to 8 digits and a period of 30 or 60 seconds; anything else is an error.
func ParseOTPAuth(raw string) (*OTPAuth, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "otpauth" {
		return nil, fmt.Errorf("not an otpauth:// URI")
	}
	if u.Host != "totp" {
		return nil, fmt.Errorf("only totp is supported, the URI says %q", u.Host)
	}
	q := u.Query()
	secret, err := NormalizeSecret(q.Get("secret"))
	if err != nil {
		return nil, err
	}
	a := &OTPAuth{Secret: secret, Issuer: q.Get("issuer"), Algorithm: "SHA1", Digits: 6, Period: 30}
	label := strings.TrimPrefix(u.Path, "/")
	if i := strings.Index(label, ":"); i >= 0 {
		if a.Issuer == "" {
			a.Issuer = strings.TrimSpace(label[:i])
		}
		label = label[i+1:]
	}
	a.Account = strings.TrimSpace(label)
	if v := q.Get("algorithm"); v != "" && !strings.EqualFold(v, "SHA1") {
		return nil, fmt.Errorf("the device supports SHA1 only, the URI says %s", v)
	}
	if v := q.Get("digits"); v != "" {
		if a.Digits, err = strconv.Atoi(v); err != nil || a.Digits < 6 || a.Digits > 8 {
			return nil, fmt.Errorf("the device supports 6 to 8 digits, the URI says %q", v)
		}
	}
	if v := q.Get("period"); v != "" {
		if a.Period, err = strconv.Atoi(v); err != nil || (a.Period != 30 && a.Period != 60) {
			return nil, fmt.Errorf("the device supports a period of 30 or 60 s, the URI says %q", v)
		}
	}
	return a, nil
}
