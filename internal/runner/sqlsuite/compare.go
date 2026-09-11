package sqlsuite

import (
	"bytes"
	"unicode/utf8"
)

// matches is CQT's verdict (ConsoleBO.saveResults): the rendering and the
// answer, each with every \r and \n taken out, are the same text.
//
// Text, not bytes: CQT reads the answer into a Java String as UTF-8, and a
// byte that is not UTF-8 becomes U+FFFD there. The rendering is valid UTF-8
// already -- it left the executor as String.getBytes("UTF-8") -- so only the
// answer needs decoding. Java's decoder can still count a truncated
// multi-byte sequence as one malformed unit where this counts each byte; none
// of the 18,000-odd .answer files the default configuration reads is malformed
// at all (the 70 that are, are _cci and _ci variants), which is why this does
// not go further.
func matches(rendered, answer []byte) bool {
	return bytes.Equal(stripBreaks(rendered), stripBreaks(asJavaReadsIt(answer)))
}

func stripBreaks(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != '\r' && c != '\n' {
			out = append(out, c)
		}
	}
	return out
}

// asJavaReadsIt is b as new String(b, "UTF-8") would hold it, encoded again: a
// U+FFFD for each byte that does not decode, where bytes.ToValidUTF8 would give
// one for a whole run of them.
func asJavaReadsIt(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out := make([]byte, 0, len(b)+8)
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			out = utf8.AppendRune(out, utf8.RuneError)
		} else {
			out = append(out, b[:size]...)
		}
		b = b[size:]
	}
	return out
}
