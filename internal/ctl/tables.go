package ctl

// The two tables below are parse.c's, printed from the C source itself rather
// than read off the page: a program that includes parse.c and walks
// next_state[][] and action[][] emitted these literals
// (evidence/isolation-parser.md). Transcribing 240 cells by hand is how a port
// acquires a difference nobody can find later.

type state int

const (
	accept   state = iota // saw the naked semicolon that ends a statement
	copy                  // normal
	oneSlash              // '/' seen; may open a C comment
	inCComment
	oneStar // '*' inside a C comment; may close it
	oneDash // '-' seen; may open an SQL comment
	inSQLComment
	inDQString
	inDQStringEscape
	inSQString
	inSQStringOneSQ // a quote inside a single-quoted string; may be an escape
	inSQStringEscape
	numStates
)

// The character classes the tables are indexed by.
type class int

const (
	clNL class = iota
	clWS
	clSQ
	clDQ
	clEscape
	clSlash
	clStar
	clDash
	clSemi
	clOther
	numClasses
)

type action int

const (
	copyInput action = iota
	copyInputAndInsertWS
	hold
	copyHoldAndInput
	copyHoldAndInsertWS
	insertWS
	skip
	quit
)

var nextState = [numStates][numClasses]state{
	{accept, accept, accept, accept, accept, accept, accept, accept, accept, accept},                                                     // accept
	{copy, copy, inSQString, inDQString, copy, oneSlash, copy, oneDash, accept, copy},                                                    // copy
	{copy, copy, copy, copy, copy, oneSlash, inCComment, copy, accept, copy},                                                             // oneSlash
	{inCComment, inCComment, inCComment, inCComment, inCComment, inCComment, oneStar, inCComment, inCComment, inCComment},                // inCComment
	{inCComment, inCComment, inCComment, inCComment, inCComment, copy, oneStar, inCComment, inCComment, inCComment},                      // oneStar
	{copy, copy, copy, copy, copy, copy, copy, inSQLComment, accept, copy},                                                               // oneDash
	{copy, inSQLComment, inSQLComment, inSQLComment, inSQLComment, inSQLComment, inSQLComment, inSQLComment, inSQLComment, inSQLComment}, // inSQLComment
	{inDQString, inDQString, inDQString, copy, inDQStringEscape, inDQString, inDQString, inDQString, inDQString, inDQString},             // inDQString
	{inDQString, inDQString, inDQString, inDQString, inDQString, inDQString, inDQString, inDQString, inDQString, inDQString},             // inDQStringEscape
	{inSQString, inSQString, inSQStringOneSQ, inSQString, inSQStringEscape, inSQString, inSQString, inSQString, inSQString, inSQString},  // inSQString
	{copy, copy, inSQString, copy, copy, copy, copy, copy, accept, copy},                                                                 // inSQStringOneSQ
	{inSQString, inSQString, inSQString, inSQString, inSQString, inSQString, inSQString, inSQString, inSQString, inSQString},             // inSQStringEscape
}

var actionTable = [numStates][numClasses]action{
	{quit, quit, quit, quit, quit, quit, quit, quit, quit, quit},                                                                                                                   // accept
	{insertWS, insertWS, copyInput, copyInput, copyInput, hold, copyInput, hold, copyInput, copyInput},                                                                             // copy
	{copyHoldAndInsertWS, copyHoldAndInsertWS, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyInput, skip, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput},        // oneSlash
	{skip, skip, skip, skip, skip, skip, skip, skip, skip, skip},                                                                                                                   // inCComment
	{skip, skip, skip, skip, skip, insertWS, skip, skip, skip, skip},                                                                                                               // oneStar
	{copyHoldAndInsertWS, copyHoldAndInsertWS, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, skip, copyHoldAndInput, copyHoldAndInput}, // oneDash
	{insertWS, skip, skip, skip, skip, skip, skip, skip, skip, skip},                                                                                                               // inSQLComment
	{copyInput, copyInput, copyInput, copyInput, hold, copyInput, copyInput, copyInput, copyInput, copyInput},                                                                      // inDQString
	{skip, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput},       // inDQStringEscape
	{copyInput, copyInput, copyInput, copyInput, hold, copyInput, copyInput, copyInput, copyInput, copyInput},                                                                      // inSQString
	{copyInput, copyInput, copyInput, copyInput, copyInput, copyInput, copyInput, copyInput, copyInput, copyInput},                                                                 // inSQStringOneSQ
	{skip, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput, copyHoldAndInput},       // inSQStringEscape
}

// classify is parse.c's init(): every byte is OTHER or WS, and eight are their
// own class. common_char_isspace is C's isspace without the locale.
var classify = func() [256]class {
	var t [256]class
	for i := range t {
		switch i {
		case ' ', '\t', '\r', '\n', '\f', '\v':
			t[i] = clWS
		default:
			t[i] = clOther
		}
	}
	t['\n'] = clNL
	t['\''] = clSQ
	t['"'] = clDQ
	t['\\'] = clEscape
	t['/'] = clSlash
	t['*'] = clStar
	t['-'] = clDash
	t[';'] = clSemi
	return t
}()
