package main

import (
	"bytes"
	"strings"
	"testing"
)

func edit(t *testing.T, input ...string) (text string, done, aborted bool, out string) {
	t.Helper()
	var buf bytes.Buffer
	m := &maskedLine{prompt: "> ", width: 40, out: &buf}
	for _, chunk := range input {
		done, aborted = m.handle([]byte(chunk))
		m.render()
		if done || aborted {
			break
		}
	}
	return m.text(), done, aborted, buf.String()
}

func TestMaskedTypingShowsStars(t *testing.T) {
	text, done, _, out := edit(t, "abc", "\r")
	if text != "abc" || !done {
		t.Fatalf("%q %v", text, done)
	}
	if strings.Contains(out, "abc") || strings.Contains(out, "a") && !strings.Contains(out, "\x1b") {
		t.Errorf("the typed text appeared on the screen: %q", out)
	}
	if !strings.Contains(out, "> ***") {
		t.Errorf("no stars drawn: %q", out)
	}
}

func TestMaskedPasteInOneChunk(t *testing.T) {
	text, done, _, _ := edit(t, "otpauth://totp/GitHub?secret=ABC\n")
	if !done || text != "otpauth://totp/GitHub?secret=ABC" {
		t.Fatalf("%q %v", text, done)
	}
}

func TestMaskedEditing(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  string
	}{
		{"backspace", []string{"abcd", "\x7f\x7f", "\r"}, "ab"},
		{"ctrl-h", []string{"abc", "\x08", "\r"}, "ab"},
		{"cursor left and insert", []string{"ac", "\x1b[D", "b", "\r"}, "abc"},
		{"home and insert", []string{"bc", "\x1b[H", "a", "\r"}, "abc"},
		{"end after home", []string{"ab", "\x1b[H", "\x1b[F", "c", "\r"}, "abc"},
		{"delete key", []string{"abc", "\x01", "\x1b[3~", "\r"}, "bc"},
		{"ctrl-a ctrl-e", []string{"bc", "\x01", "a", "\x05", "d", "\r"}, "abcd"},
		{"ctrl-u kills to the start", []string{"abc", "\x1b[D", "\x15", "\r"}, "c"},
		{"ctrl-k kills to the end", []string{"abc", "\x01", "\x1b[C", "\x0b", "\r"}, "a"},
		{"ctrl-w kills a word", []string{"abc def", "\x17", "\r"}, "abc "},
		{"cursor keys at the ends do nothing", []string{"a", "\x1b[C", "\x1b[C", "\x1b[D", "\x1b[D", "\x1b[D", "b", "\r"}, "ba"},
		{"unknown escape sequences are ignored", []string{"a", "\x1b[1;5C", "\x1b[A", "\x1bOP", "b", "\r"}, "ab"},
		{"tab is ignored", []string{"a\tb", "\r"}, "ab"},
		{"one star per character, not per byte", []string{"äb", "\x7f\x7f", "x", "\r"}, "x"},
		{"backspace on an empty line", []string{"\x7f", "a", "\r"}, "a"},
	}
	for _, c := range cases {
		text, done, aborted, _ := edit(t, c.input...)
		if text != c.want || !done || aborted {
			t.Errorf("%s: got %q (done %v, aborted %v), want %q", c.name, text, done, aborted, c.want)
		}
	}
}

func TestMaskedAbort(t *testing.T) {
	if _, _, aborted, _ := edit(t, "abc", "\x03"); !aborted {
		t.Error("Ctrl-C must abort")
	}
	if _, _, aborted, _ := edit(t, "\x04"); !aborted {
		t.Error("Ctrl-D on an empty line must abort")
	}
	if text, done, aborted, _ := edit(t, "ab", "\x01", "\x04", "\r"); aborted || !done || text != "b" {
		t.Errorf("Ctrl-D in a line deletes: %q", text)
	}
}

func TestMaskedLongInputScrollsWithinOneLine(t *testing.T) {
	var buf bytes.Buffer
	m := &maskedLine{prompt: "> ", width: 20, out: &buf}
	m.handle([]byte(strings.Repeat("x", 100)))
	buf.Reset()
	m.render()
	line := buf.String()
	if stars := strings.Count(line, "*"); stars > 20-2-1 || stars < 10 {
		t.Errorf("%d stars drawn into a 20 column terminal", stars)
	}
	if len(m.text()) != 100 {
		t.Errorf("the text must stay complete: %d", len(m.text()))
	}
	buf.Reset()
	m.handle([]byte("\x01")) // Home: the window moves back to the start
	m.render()
	if !strings.HasSuffix(buf.String(), "\x1b[17D") {
		t.Errorf("the cursor must go back to the start of the window: %q", buf.String())
	}
}
