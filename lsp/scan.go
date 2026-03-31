package lsp

import (
	"fmt"
	"strconv"
	"strings"
)

// Token type constants.
// Single-char tokens use their rune value directly (e.g. '+' == 43).
// Multi-char and keyword tokens start at 256.
const (
	TokEOF = -(iota + 1)
)

const (
	TokName   = 256 + iota
	TokNumber // numeric literal
	TokString // string literal
	TokConcat // ..
	TokDots   // ...
	TokEq     // ==
	TokGE     // >=
	TokLE     // <=
	TokNE     // ~=
	TokDColon // ::
	// Keywords
	TokAnd
	TokBreak
	TokDo
	TokElse
	TokElseif
	TokEnd
	TokFalse
	TokFor
	TokFunction
	TokGoto
	TokIf
	TokIn
	TokLocal
	TokNil
	TokNot
	TokOr
	TokRepeat
	TokReturn
	TokThen
	TokTrue
	TokUntil
	TokWhile
)

var keywords = map[string]int{
	"and":      TokAnd,
	"break":    TokBreak,
	"do":       TokDo,
	"else":     TokElse,
	"elseif":   TokElseif,
	"end":      TokEnd,
	"false":    TokFalse,
	"for":      TokFor,
	"function": TokFunction,
	"goto":     TokGoto,
	"if":       TokIf,
	"in":       TokIn,
	"local":    TokLocal,
	"nil":      TokNil,
	"not":      TokNot,
	"or":       TokOr,
	"repeat":   TokRepeat,
	"return":   TokReturn,
	"then":     TokThen,
	"true":     TokTrue,
	"until":    TokUntil,
	"while":    TokWhile,
}

// Token represents a single lexical token.
type Token struct {
	Type int
	Val  string
	Num  float64
	Sp   Span
}

// TokenName returns a human-readable name for a token type.
func TokenName(t int) string {
	switch t {
	case TokEOF:
		return "<eof>"
	case TokName:
		return "<name>"
	case TokNumber:
		return "<number>"
	case TokString:
		return "<string>"
	case TokConcat:
		return ".."
	case TokDots:
		return "..."
	case TokEq:
		return "=="
	case TokGE:
		return ">="
	case TokLE:
		return "<="
	case TokNE:
		return "~="
	case TokDColon:
		return "::"
	case TokAnd:
		return "and"
	case TokBreak:
		return "break"
	case TokDo:
		return "do"
	case TokElse:
		return "else"
	case TokElseif:
		return "elseif"
	case TokEnd:
		return "end"
	case TokFalse:
		return "false"
	case TokFor:
		return "for"
	case TokFunction:
		return "function"
	case TokGoto:
		return "goto"
	case TokIf:
		return "if"
	case TokIn:
		return "in"
	case TokLocal:
		return "local"
	case TokNil:
		return "nil"
	case TokNot:
		return "not"
	case TokOr:
		return "or"
	case TokRepeat:
		return "repeat"
	case TokReturn:
		return "return"
	case TokThen:
		return "then"
	case TokTrue:
		return "true"
	case TokUntil:
		return "until"
	case TokWhile:
		return "while"
	default:
		if t >= 32 && t < 127 {
			return fmt.Sprintf("'%c'", rune(t))
		}
		return fmt.Sprintf("<tok %d>", t)
	}
}

// Scanner is a Lua 5.2 lexer.
type Scanner struct {
	src    []byte
	offset int
	line   int
	col    int
	peek   Token
	peeked bool
}

// NewScanner creates a new Scanner for the given source string.
func NewScanner(src string) *Scanner {
	return &Scanner{src: []byte(src)}
}

// pos returns the current position in the source.
func (s *Scanner) pos() Pos {
	return Pos{Line: s.line, Col: s.col, Offset: s.offset}
}

// current returns the byte at the current offset, or 0 if at EOF.
func (s *Scanner) current() byte {
	if s.offset >= len(s.src) {
		return 0
	}
	return s.src[s.offset]
}

// peek1 returns the byte one ahead of current, or 0.
func (s *Scanner) peek1() byte {
	if s.offset+1 >= len(s.src) {
		return 0
	}
	return s.src[s.offset+1]
}

// advance moves forward one byte, updating line/col tracking.
func (s *Scanner) advance() byte {
	if s.offset >= len(s.src) {
		return 0
	}
	ch := s.src[s.offset]
	s.offset++
	if ch == '\n' {
		s.line++
		s.col = 0
	} else {
		s.col++
	}
	return ch
}

// Peek returns the next token without consuming it.
func (s *Scanner) Peek() Token {
	if !s.peeked {
		s.peek = s.scan()
		s.peeked = true
	}
	return s.peek
}

// Next consumes and returns the next token.
func (s *Scanner) Next() Token {
	if s.peeked {
		s.peeked = false
		return s.peek
	}
	return s.scan()
}

// scan performs the actual lexing work.
func (s *Scanner) scan() Token {
	s.skipWhitespaceAndComments()

	if s.offset >= len(s.src) {
		p := s.pos()
		return Token{Type: TokEOF, Sp: Span{From: p, To: p}}
	}

	from := s.pos()
	ch := s.current()

	// Identifiers and keywords
	if isLetter(ch) {
		return s.scanName(from)
	}

	// Numbers
	if isDigit(ch) || (ch == '.' && isDigit(s.peek1())) {
		return s.scanNumber(from)
	}

	// Strings
	if ch == '"' || ch == '\'' {
		return s.scanShortString(from)
	}

	// Long strings [[...]]
	if ch == '[' && (s.peek1() == '[' || s.peek1() == '=') {
		level := s.longBracketLevel()
		if level >= 0 {
			return s.scanLongString(from, level)
		}
	}

	// Multi-char operators and single-char tokens
	s.advance()
	switch ch {
	case '.':
		if s.current() == '.' {
			s.advance()
			if s.current() == '.' {
				s.advance()
				return Token{Type: TokDots, Val: "...", Sp: Span{From: from, To: s.pos()}}
			}
			return Token{Type: TokConcat, Val: "..", Sp: Span{From: from, To: s.pos()}}
		}
		return Token{Type: int('.'), Val: ".", Sp: Span{From: from, To: s.pos()}}
	case '=':
		if s.current() == '=' {
			s.advance()
			return Token{Type: TokEq, Val: "==", Sp: Span{From: from, To: s.pos()}}
		}
		return Token{Type: int('='), Val: "=", Sp: Span{From: from, To: s.pos()}}
	case '<':
		if s.current() == '=' {
			s.advance()
			return Token{Type: TokLE, Val: "<=", Sp: Span{From: from, To: s.pos()}}
		}
		return Token{Type: int('<'), Val: "<", Sp: Span{From: from, To: s.pos()}}
	case '>':
		if s.current() == '=' {
			s.advance()
			return Token{Type: TokGE, Val: ">=", Sp: Span{From: from, To: s.pos()}}
		}
		return Token{Type: int('>'), Val: ">", Sp: Span{From: from, To: s.pos()}}
	case '~':
		if s.current() == '=' {
			s.advance()
			return Token{Type: TokNE, Val: "~=", Sp: Span{From: from, To: s.pos()}}
		}
		// bare ~ is not a valid Lua token but return it as-is
		return Token{Type: int('~'), Val: "~", Sp: Span{From: from, To: s.pos()}}
	case ':':
		if s.current() == ':' {
			s.advance()
			return Token{Type: TokDColon, Val: "::", Sp: Span{From: from, To: s.pos()}}
		}
		return Token{Type: int(':'), Val: ":", Sp: Span{From: from, To: s.pos()}}
	default:
		val := string([]byte{ch})
		return Token{Type: int(ch), Val: val, Sp: Span{From: from, To: s.pos()}}
	}
}

// skipWhitespaceAndComments skips whitespace and Lua comments.
func (s *Scanner) skipWhitespaceAndComments() {
	for s.offset < len(s.src) {
		ch := s.current()
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '\f' || ch == '\v' {
			s.advance()
			continue
		}
		// Check for comment
		if ch == '-' && s.peek1() == '-' {
			s.advance() // -
			s.advance() // -
			// Check for block comment
			if s.current() == '[' {
				level := s.longBracketLevel()
				if level >= 0 {
					s.skipLongString(level)
					continue
				}
			}
			// Single-line comment: skip to end of line
			for s.offset < len(s.src) && s.current() != '\n' {
				s.advance()
			}
			continue
		}
		break
	}
}

// longBracketLevel checks if the current position starts a long bracket
// and returns its level (number of = signs), or -1 if not a long bracket.
// Does NOT consume any characters.
func (s *Scanner) longBracketLevel() int {
	if s.offset >= len(s.src) || s.src[s.offset] != '[' {
		return -1
	}
	i := s.offset + 1
	level := 0
	for i < len(s.src) && s.src[i] == '=' {
		level++
		i++
	}
	if i < len(s.src) && s.src[i] == '[' {
		return level
	}
	return -1
}

// skipLongString skips a long string/comment body (after the opening [=*[ has been detected).
// Consumes the opening bracket sequence and everything up to and including the matching close.
func (s *Scanner) skipLongString(level int) {
	// consume opening [=*[
	s.advance() // [
	for i := 0; i < level; i++ {
		s.advance() // =
	}
	s.advance() // [

	for s.offset < len(s.src) {
		ch := s.advance()
		if ch == ']' {
			cnt := 0
			for s.offset < len(s.src) && s.current() == '=' {
				cnt++
				s.advance()
			}
			if cnt == level && s.offset < len(s.src) && s.current() == ']' {
				s.advance() // closing ]
				return
			}
		}
	}
}

// scanLongString scans a long string literal starting at from.
// level is the number of '=' in the brackets.
func (s *Scanner) scanLongString(from Pos, level int) Token {
	// consume opening [=*[
	s.advance() // [
	for i := 0; i < level; i++ {
		s.advance() // =
	}
	s.advance() // [

	// Skip an immediate newline (Lua spec: first newline is ignored)
	if s.offset < len(s.src) && s.current() == '\n' {
		s.advance()
	} else if s.offset < len(s.src) && s.current() == '\r' {
		s.advance()
		if s.offset < len(s.src) && s.current() == '\n' {
			s.advance()
		}
	}

	var buf strings.Builder
	for s.offset < len(s.src) {
		ch := s.current()
		if ch == ']' {
			s.advance()
			cnt := 0
			var eqBuf strings.Builder
			for s.offset < len(s.src) && s.current() == '=' {
				eqBuf.WriteByte('=')
				cnt++
				s.advance()
			}
			if cnt == level && s.offset < len(s.src) && s.current() == ']' {
				s.advance() // closing ]
				return Token{Type: TokString, Val: buf.String(), Sp: Span{From: from, To: s.pos()}}
			}
			// Not closing bracket: emit the ] and = chars we consumed
			buf.WriteByte(']')
			buf.WriteString(eqBuf.String())
			continue
		}
		s.advance()
		buf.WriteByte(ch)
	}
	// Unterminated long string
	return Token{Type: TokString, Val: buf.String(), Sp: Span{From: from, To: s.pos()}}
}

// scanShortString scans a quoted string literal.
func (s *Scanner) scanShortString(from Pos) Token {
	quote := s.advance() // consume opening ' or "
	var buf strings.Builder
	for s.offset < len(s.src) {
		ch := s.current()
		if ch == '\n' || ch == '\r' {
			// Unterminated string
			break
		}
		if byte(ch) == quote {
			s.advance() // closing quote
			return Token{Type: TokString, Val: buf.String(), Sp: Span{From: from, To: s.pos()}}
		}
		if ch == '\\' {
			s.advance() // consume backslash
			esc := s.advance()
			switch esc {
			case 'a':
				buf.WriteByte('\a')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'v':
				buf.WriteByte('\v')
			case '\\':
				buf.WriteByte('\\')
			case '\'':
				buf.WriteByte('\'')
			case '"':
				buf.WriteByte('"')
			case '\n', '\r':
				// line break in string
				if esc == '\r' && s.offset < len(s.src) && s.current() == '\n' {
					s.advance()
				}
				buf.WriteByte('\n')
			case 'x':
				// \xhh - two hex digits
				h1 := s.advance()
				h2 := s.advance()
				val, err := strconv.ParseUint(string([]byte{h1, h2}), 16, 8)
				if err == nil {
					buf.WriteByte(byte(val))
				}
			case 'z':
				// \z skips following whitespace
				for s.offset < len(s.src) {
					c := s.current()
					if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
						s.advance()
					} else {
						break
					}
				}
			default:
				if isDigit(esc) {
					// \ddd - up to 3 decimal digits
					digits := []byte{esc}
					for len(digits) < 3 && s.offset < len(s.src) && isDigit(s.current()) {
						digits = append(digits, s.advance())
					}
					val, err := strconv.ParseUint(string(digits), 10, 8)
					if err == nil {
						buf.WriteByte(byte(val))
					}
				} else {
					buf.WriteByte(esc)
				}
			}
			continue
		}
		s.advance()
		buf.WriteByte(ch)
	}
	return Token{Type: TokString, Val: buf.String(), Sp: Span{From: from, To: s.pos()}}
}

// scanName scans an identifier or keyword.
func (s *Scanner) scanName(from Pos) Token {
	start := s.offset
	for s.offset < len(s.src) && isLetterOrDigit(s.current()) {
		s.advance()
	}
	name := string(s.src[start:s.offset])
	tok := Token{Val: name, Sp: Span{From: from, To: s.pos()}}
	if kw, ok := keywords[name]; ok {
		tok.Type = kw
	} else {
		tok.Type = TokName
	}
	return tok
}

// scanNumber scans a numeric literal.
func (s *Scanner) scanNumber(from Pos) Token {
	start := s.offset
	isHex := false

	if s.current() == '0' && (s.peek1() == 'x' || s.peek1() == 'X') {
		isHex = true
		s.advance() // 0
		s.advance() // x
		for s.offset < len(s.src) && isHexDigit(s.current()) {
			s.advance()
		}
		// hex float part
		if s.offset < len(s.src) && s.current() == '.' {
			s.advance()
			for s.offset < len(s.src) && isHexDigit(s.current()) {
				s.advance()
			}
		}
		// hex exponent p/P
		if s.offset < len(s.src) && (s.current() == 'p' || s.current() == 'P') {
			s.advance()
			if s.offset < len(s.src) && (s.current() == '+' || s.current() == '-') {
				s.advance()
			}
			for s.offset < len(s.src) && isDigit(s.current()) {
				s.advance()
			}
		}
	} else {
		for s.offset < len(s.src) && isDigit(s.current()) {
			s.advance()
		}
		if s.offset < len(s.src) && s.current() == '.' {
			s.advance()
			for s.offset < len(s.src) && isDigit(s.current()) {
				s.advance()
			}
		}
		if s.offset < len(s.src) && (s.current() == 'e' || s.current() == 'E') {
			s.advance()
			if s.offset < len(s.src) && (s.current() == '+' || s.current() == '-') {
				s.advance()
			}
			for s.offset < len(s.src) && isDigit(s.current()) {
				s.advance()
			}
		}
	}

	raw := string(s.src[start:s.offset])
	var num float64
	if isHex {
		// Try integer first, then float
		if iv, err := strconv.ParseInt(raw, 0, 64); err == nil {
			num = float64(iv)
		} else if uv, err := strconv.ParseUint(raw, 0, 64); err == nil {
			num = float64(uv)
		} else {
			// hex float - use ParseFloat which handles 0x... in Go 1.13+
			if fv, err := strconv.ParseFloat(raw, 64); err == nil {
				num = fv
			}
		}
	} else {
		if fv, err := strconv.ParseFloat(raw, 64); err == nil {
			num = fv
		}
	}

	return Token{Type: TokNumber, Val: raw, Num: num, Sp: Span{From: from, To: s.pos()}}
}

// Character classification helpers.

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isLetterOrDigit(ch byte) bool {
	return isLetter(ch) || isDigit(ch)
}
