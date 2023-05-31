// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

// Starlark quoted string utilities.

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// unesc maps single-letter chars following \ to their actual values.
var unesc = [256]byte{
	'a':  '\a',
	'b':  '\b',
	'f':  '\f',
	'n':  '\n',
	'r':  '\r',
	't':  '\t',
	'v':  '\v',
	'\\': '\\',
	'\'': '\'',
	'"':  '"',
}

// esc maps escape-worthy bytes to the char that should follow \.
var esc = [256]byte{
	'\a': 'a',
	'\b': 'b',
	'\f': 'f',
	'\n': 'n',
	'\r': 'r',
	'\t': 't',
	'\v': 'v',
	'\\': '\\',
	'\'': '\'',
	'"':  '"',
}

// unquote unquotes the quoted string, returning the actual
// string value, whether the original was triple-quoted,
// whether it was a byte string, and an error describing invalid input.
func unquote(quoted string) (s string, triple, isByte bool, err error) {
	// Check for raw prefix: means don't interpret the inner \.
	raw := false
	if strings.HasPrefix(quoted, "r") {
		raw = true
		quoted = quoted[1:]
	}
	// Check for bytes prefix.
	if strings.HasPrefix(quoted, "b") {
		isByte = true
		quoted = quoted[1:]
	}

	if len(quoted) < 2 {
		err = fmt.Errorf("string literal too short")
		return
	}

	if quoted[0] != '"' && quoted[0] != '\'' || quoted[0] != quoted[len(quoted)-1] {
		err = fmt.Errorf("string literal has invalid quotes")
		return
	}

	// Check for triple quoted string.
	quote := quoted[0]
	if len(quoted) >= 6 && quoted[1] == quote && quoted[2] == quote && quoted[:3] == quoted[len(quoted)-3:] {
		triple = true
		quoted = quoted[3 : len(quoted)-3]
	} else {
		quoted = quoted[1 : len(quoted)-1]
	}

	// Now quoted is the quoted data, but no quotes.
	// If we're in raw mode or there are no escapes or
	// carriage returns, we're done.
	var unquoteChars string
	if raw {
		unquoteChars = "\r"
	} else {
		unquoteChars = "\\\r"
	}
	if !strings.ContainsAny(quoted, unquoteChars) {
		s = quoted
		return
	}

	// Otherwise process quoted string.
	// Each iteration processes one escape sequence along with the
	// plain text leading up to it.
	buf := new(strings.Builder)
	for {
		// Remove prefix before escape sequence.
		i := strings.IndexAny(quoted, unquoteChars)
		if i < 0 {
			i = len(quoted)
		}
		buf.WriteString(quoted[:i])
		quoted = quoted[i:]

		if len(quoted) == 0 {
			break
		}

		// Process carriage return.
		if quoted[0] == '\r' {
			buf.WriteByte('\n')
			if len(quoted) > 1 && quoted[1] == '\n' {
				quoted = quoted[2:]
			} else {
				quoted = quoted[1:]
			}
			continue
		}

		// Process escape sequence.
		if len(quoted) == 1 {
			err = fmt.Errorf(`truncated escape sequence \`)
			return
		}

		switch quoted[1] {
		default:
			// In Starlark, like Go, a backslash must escape something.
			// (Python still treats unnecessary backslashes literally,
			// but since 3.6 has emitted a deprecation warning.)
			err = fmt.Errorf("invalid escape sequence \\%c", quoted[1])
			return

		case '\n':
			// Ignore the escape and the line break.
			quoted = quoted[2:]

		case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '\'', '"':
			// One-char escape.
			// Escapes are allowed for both kinds of quotation
			// mark, not just the kind in use.
			buf.WriteByte(unesc[quoted[1]])
			quoted = quoted[2:]

		case '0', '1', '2', '3', '4', '5', '6', '7':
			// Octal escape, up to 3 digits, \OOO.
			n := int(quoted[1] - '0')
			quoted = quoted[2:]
			for i := 1; i < 3; i++ {
				if len(quoted) == 0 || quoted[0] < '0' || '7' < quoted[0] {
					break
				}
				n = n*8 + int(quoted[0]-'0')
				quoted = quoted[1:]
			}
			if !isByte && n > 127 {
				err = fmt.Errorf(`non-ASCII octal escape \%o (use \u%04X for the UTF-8 encoding of U+%04X)`, n, n, n)
				return
			}
			if n >= 256 {
				// NOTE: Python silently discards the high bit,
				// so that '\541' == '\141' == 'a'.
				// Let's see if we can avoid doing that in BUILD files.
				err = fmt.Errorf(`invalid escape sequence \%03o`, n)
				return
			}
			buf.WriteByte(byte(n))

		case 'x':
			// Hexadecimal escape, exactly 2 digits, \xXX. [0-127]
			if len(quoted) < 4 {
				err = fmt.Errorf(`truncated escape sequence %s`, quoted)
				return
			}
			n, err1 := strconv.ParseUint(quoted[2:4], 16, 0)
			if err1 != nil {
				err = fmt.Errorf(`invalid escape sequence %s`, quoted[:4])
				return
			}
			if !isByte && n > 127 {
				err = fmt.Errorf(`non-ASCII hex escape %s (use \u%04X for the UTF-8 encoding of U+%04X)`,
					quoted[:4], n, n)
				return
			}
			buf.WriteByte(byte(n))
			quoted = quoted[4:]

		case 'u', 'U':
			// Unicode code point, 4 (\uXXXX) or 8 (\UXXXXXXXX) hex digits.
			sz := 6
			if quoted[1] == 'U' {
				sz = 10
			}
			if len(quoted) < sz {
				err = fmt.Errorf(`truncated escape sequence %s`, quoted)
				return
			}
			n, err1 := strconv.ParseUint(quoted[2:sz], 16, 0)
			if err1 != nil {
				err = fmt.Errorf(`invalid escape sequence %s`, quoted[:sz])
				return
			}
			if n > unicode.MaxRune {
				err = fmt.Errorf(`code point out of range: %s (max \U%08x)`,
					quoted[:sz], n)
				return
			}
			// As in Go, surrogates are disallowed.
			if 0xD800 <= n && n < 0xE000 {
				err = fmt.Errorf(`invalid Unicode code point U+%04X`, n)
				return
			}
			buf.WriteRune(rune(n))
			quoted = quoted[sz:]
		}
	}

	s = buf.String()
	return
}

// indexByte returns the index of the first instance of b in s, or else -1.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

type BytesWriter interface {
	io.Writer
	io.ByteWriter
	io.StringWriter

	WriteRune(r rune) (size int, err error)
}

// Quote returns a Starlark literal that denotes s.
// If b, it returns a bytes literal.
func Quote(s string, b bool) string {
	buf := new(bytes.Buffer)
	buf.Grow(3 * len(s) / 2)
	QuoteWriter(buf, s, b)
	return string(buf.Bytes())
}

func QuoteWriter(w io.Writer, s string, b bool) error {
	const hex = "0123456789abcdef"
	buffer := make([]byte, utf8.UTFMax)

	if b {
		if _, err := w.Write([]byte{'b'}); err != nil {
			return err
		}
	}
	if _, err := w.Write([]byte{'"'}); err != nil {
		return err
	}

	for width := 0; len(s) > 0; s = s[width:] {
		r := rune(s[0])
		width = 1
		if r >= utf8.RuneSelf {
			r, width = utf8.DecodeRuneInString(s)
		}
		if width == 1 && r == utf8.RuneError {
			// String (!b) literals accept \xXX escapes only for ASCII,
			// but we must use them here to represent invalid bytes.
			// The result is not a legal literal.
			if _, err := w.Write([]byte{'\\', 'x', hex[s[0]>>4], hex[s[0]&0xF]}); err != nil {
				return err
			}
			continue
		}
		if r == '"' || r == '\\' { // always backslashed
			if _, err := w.Write([]byte{'\\', byte(r)}); err != nil {
				return err
			}
			continue
		}
		if strconv.IsPrint(r) {
			runeSize := utf8.EncodeRune(buffer, r)
			if _, err := w.Write(buffer[:runeSize]); err != nil {
				return err
			}
			continue
		}
		switch r {
		case '\a':
			if _, err := w.Write([]byte{'\\', 'a'}); err != nil {
				return err
			}
		case '\b':
			if _, err := w.Write([]byte{'\\', 'b'}); err != nil {
				return err
			}
		case '\f':
			if _, err := w.Write([]byte{'\\', 'f'}); err != nil {
				return err
			}
		case '\n':
			if _, err := w.Write([]byte{'\\', 'n'}); err != nil {
				return err
			}
		case '\r':
			if _, err := w.Write([]byte{'\\', 'r'}); err != nil {
				return err
			}
		case '\t':
			if _, err := w.Write([]byte{'\\', 't'}); err != nil {
				return err
			}
		case '\v':
			if _, err := w.Write([]byte{'\\', 'v'}); err != nil {
				return err
			}
		default:
			switch {
			case r < ' ' || r == 0x7f:
				if _, err := w.Write([]byte{'\\', 'x', hex[byte(r)>>4], hex[byte(r)&0xF]}); err != nil {
					return err
				}
			case r > utf8.MaxRune:
				r = 0xFFFD
				fallthrough
			case r < 0x10000:
				if _, err := w.Write([]byte{'\\', 'u'}); err != nil {
					return err
				}
				for s := 12; s >= 0; s -= 4 {
					if _, err := w.Write([]byte{hex[r>>uint(s)&0xF]}); err != nil {
						return err
					}
				}
			default:
				if _, err := w.Write([]byte{'\\', 'U'}); err != nil {
					return err
				}
				for s := 28; s >= 0; s -= 4 {
					if _, err := w.Write([]byte{hex[r>>uint(s)&0xF]}); err != nil {
						return err
					}
				}
			}
		}
	}

	if _, err := w.Write([]byte{'"'}); err != nil {
		return err
	}
	return nil
}
