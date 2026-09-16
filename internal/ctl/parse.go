// Package ctl reads the .ctl files an isolation case is written in.
//
// The reader is a port of ctltool's parse.c, kept deliberately literal: the
// same twelve states, the same ten character classes, the same two tables, the
// same 1024-byte line buffer. A statement is everything up to the first naked
// semicolon -- naked meaning outside a comment and outside a string -- with
// comments dropped and every run of white space collapsed to one space.
//
// It is a port and not a design, because the 6,865 answer files in the corpus
// are what the old program printed after splitting the input this way. A parser
// that split it differently would be a better parser and the wrong one.
package ctl

import (
	"bufio"
	"io"
	"strings"
)

// rawBufSize is parse.c's RAW_BUF_SIZE: the fgets buffer. It has one visible
// consequence -- a line longer than this counts as two lines -- and Lines
// reports the count the old program reported.
const rawBufSize = 1024

// Reader returns the statements of a .ctl file, in order.
type Reader struct {
	br   *bufio.Reader
	raw  []byte // the current line, and how far into it the last statement ended
	next int    // -- raw_buf, raw_next and raw_limit, which parse.c keeps between calls
	line int
}

// NewReader reads statements from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}

// Lines is parse_line_num: the number of fgets calls made so far, which is the
// line number the controller prints with a statement. A line longer than 1023
// bytes is two of them, as it was.
func (p *Reader) Lines() int { return p.line }

// readRaw is read_raw_fp: one fgets of at most rawBufSize-1 bytes, the newline
// kept. It returns false at end of file, and counts the call either way, as the
// C does.
func (p *Reader) readRaw() bool {
	p.line++
	buf := make([]byte, 0, 128)
	for len(buf) < rawBufSize-1 {
		b, err := p.br.ReadByte()
		if err != nil {
			break
		}
		buf = append(buf, b)
		if b == '\n' {
			break
		}
	}
	p.raw, p.next = buf, 0
	return len(buf) > 0
}

// Next returns the next statement and true, or "" and false at end of input.
// The terminating semicolon is part of the statement; the last statement of a
// file that does not end in one is still returned.
func (p *Reader) Next() (string, bool) {
	var (
		out     strings.Builder
		st      = copy
		pending bool
		held    byte = '?'
	)
	// deposit is the DEPOSIT macro: a run of white space becomes one space,
	// and never a leading one.
	deposit := func(ch byte) {
		if pending {
			if out.Len() != 0 {
				out.WriteByte(' ')
			}
			pending = false
		}
		out.WriteByte(ch)
	}

	for {
		// A line can hold more than one statement -- 2,328 cases in the
		// corpus put two there -- so the next one resumes where this one
		// stopped, in the line already read.
		if p.next >= len(p.raw) && !p.readRaw() {
			// Input exhausted. Whatever has accumulated is the last
			// statement; nothing accumulated is the end.
			if out.Len() > 0 {
				return out.String(), true
			}
			return "", false
		}
		for ; p.next < len(p.raw); p.next++ {
			ch := p.raw[p.next]
			cc := classify[ch]
			was := st
			st = nextState[was][cc]
			switch actionTable[was][cc] {
			case copyInput:
				deposit(ch)
			case copyInputAndInsertWS:
				deposit(ch)
				pending = true
			case hold:
				held = ch
			case copyHoldAndInput:
				deposit(held)
				// The C writes the second character straight into the
				// buffer, past DEPOSIT and so past the pending space.
				out.WriteByte(ch)
			case copyHoldAndInsertWS:
				deposit(held)
				pending = true
			case insertWS:
				pending = true
			case skip:
			case quit:
			}
			if st == accept {
				p.next++
				return out.String(), true
			}
		}
	}
}

// Statements reads r to the end and returns every statement in it.
func Statements(r io.Reader) []string {
	p := NewReader(r)
	var out []string
	for {
		s, ok := p.Next()
		if !ok {
			return out
		}
		out = append(out, s)
	}
}

// NextCompound is qamc_get_compound_stmt (qamccom.c:881): a statement, or the
// several a `{ … };` group holds joined into one, with the count of them. The
// count is what the controller expects back as "is ready" messages.
//
// No case in the corpus uses the braces, but a statement whose first character
// is one -- or whose brace follows a colon -- would have been read this way, so
// it still is.
func (p *Reader) NextCompound() (string, int, bool) {
	s, ok := p.Next()
	if !ok {
		return "", 0, false
	}
	i := strings.IndexByte(s, '{')
	if i < 0 || !(i == 0 || (i >= 1 && s[i-1] == ':') || (i >= 2 && s[i-2] == ':')) {
		return s, 1, true
	}
	var b strings.Builder
	b.WriteString(s[:i])
	b.WriteByte(' ') // the opening brace is destroyed, not kept
	b.WriteString(s[i+1:])
	count := 1
	for {
		next, ok := p.Next()
		if !ok {
			break
		}
		if len(next) > 0 && next[0] == '}' &&
			((len(next) > 1 && next[1] == ';') || (len(next) > 2 && next[2] == ';')) {
			break // the closing brace is not part of the statement
		}
		b.WriteString(next)
		count++
	}
	return b.String(), count, true
}
