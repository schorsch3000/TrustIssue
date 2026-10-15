package main

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// maskedLine is a small line editor for a password-like field: every
// character shows as "*", but the usual readline keys work (cursor keys,
// Home/End, Backspace/Delete, Ctrl-A/E/K/U/W). It draws on out with plain
// ANSI sequences and is fed the raw bytes read from a terminal in raw mode.
type maskedLine struct {
	prompt string
	width  int // terminal width in columns
	out    io.Writer

	buf []rune
	cur int // cursor position in runes
}

// handle processes one chunk of input bytes. done is set by Enter, aborted by
// Ctrl-C or by Ctrl-D on an empty line.
func (m *maskedLine) handle(b []byte) (done, aborted bool) {
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == '\r' || c == '\n':
			return true, false
		case c == 0x03:
			return false, true
		case c == 0x04:
			if len(m.buf) == 0 {
				return false, true
			}
			m.deleteAt(m.cur)
		case c == 0x1b:
			i += m.escape(b[i:])
			continue
		case c == 0x7f || c == 0x08:
			if m.cur > 0 {
				m.cur--
				m.deleteAt(m.cur)
			}
		case c == 0x01:
			m.cur = 0
		case c == 0x05:
			m.cur = len(m.buf)
		case c == 0x02:
			m.left()
		case c == 0x06:
			m.right()
		case c == 0x0b: // kill to the end
			m.buf = m.buf[:m.cur]
		case c == 0x15: // kill to the start
			m.buf = append([]rune(nil), m.buf[m.cur:]...)
			m.cur = 0
		case c == 0x17: // kill the word before the cursor
			end := m.cur
			for m.cur > 0 && m.buf[m.cur-1] == ' ' {
				m.cur--
			}
			for m.cur > 0 && m.buf[m.cur-1] != ' ' {
				m.cur--
			}
			m.buf = append(m.buf[:m.cur], m.buf[end:]...)
		case c < 0x20: // other control characters, Tab included
		default:
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			m.buf = append(m.buf, 0)
			copy(m.buf[m.cur+1:], m.buf[m.cur:])
			m.buf[m.cur] = r
			m.cur++
			i += size
			continue
		}
		i++
	}
	return false, false
}

// escape handles an escape sequence at the start of b and returns its length.
func (m *maskedLine) escape(b []byte) int {
	if len(b) < 2 || (b[1] != '[' && b[1] != 'O') {
		return 1
	}
	end := 2
	for end < len(b) && (b[end] < 0x40 || b[end] > 0x7e) { // parameter bytes
		end++
	}
	if end >= len(b) {
		return len(b)
	}
	params := string(b[2:end])
	switch b[end] {
	case 'D':
		m.left()
	case 'C':
		m.right()
	case 'H':
		m.cur = 0
	case 'F':
		m.cur = len(m.buf)
	case '~':
		switch params {
		case "1", "7":
			m.cur = 0
		case "4", "8":
			m.cur = len(m.buf)
		case "3":
			m.deleteAt(m.cur)
		}
	}
	return end + 1
}

func (m *maskedLine) left() {
	if m.cur > 0 {
		m.cur--
	}
}

func (m *maskedLine) right() {
	if m.cur < len(m.buf) {
		m.cur++
	}
}

func (m *maskedLine) deleteAt(i int) {
	if i < len(m.buf) {
		m.buf = append(m.buf[:i], m.buf[i+1:]...)
	}
}

// render redraws the line. Long input scrolls inside one terminal line (a
// wrapped line could not be redrawn with a carriage return).
func (m *maskedLine) render() {
	visible := m.width - utf8.RuneCountInString(m.prompt) - 1
	if visible < 1 {
		visible = 1
	}
	start := 0
	if m.cur > visible {
		start = m.cur - visible
	}
	stop := start + visible
	if stop > len(m.buf) {
		stop = len(m.buf)
	}
	fmt.Fprintf(m.out, "\r\x1b[K%s%s", m.prompt, strings.Repeat("*", stop-start))
	if back := stop - m.cur; back > 0 {
		fmt.Fprintf(m.out, "\x1b[%dD", back)
	}
}

func (m *maskedLine) text() string { return string(m.buf) }
