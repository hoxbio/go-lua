package lsp

import (
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func scanAll(src string) []Token {
	s := NewScanner(src)
	var toks []Token
	for {
		t := s.Next()
		toks = append(toks, t)
		if t.Type == TokEOF {
			break
		}
	}
	return toks
}

func scanTypes(src string) []int {
	toks := scanAll(src)
	types := make([]int, len(toks))
	for i, t := range toks {
		types[i] = t.Type
	}
	return types
}

func first(src string) Token {
	return NewScanner(src).Next()
}

// ---- keywords --------------------------------------------------------------

func TestScanKeywords(t *testing.T) {
	cases := []struct {
		src string
		typ int
	}{
		{"and", TokAnd}, {"break", TokBreak}, {"do", TokDo},
		{"else", TokElse}, {"elseif", TokElseif}, {"end", TokEnd},
		{"false", TokFalse}, {"for", TokFor}, {"function", TokFunction},
		{"goto", TokGoto}, {"if", TokIf}, {"in", TokIn},
		{"local", TokLocal}, {"nil", TokNil}, {"not", TokNot},
		{"or", TokOr}, {"repeat", TokRepeat}, {"return", TokReturn},
		{"then", TokThen}, {"true", TokTrue}, {"until", TokUntil},
		{"while", TokWhile},
	}
	for _, c := range cases {
		tok := first(c.src)
		if tok.Type != c.typ {
			t.Errorf("scan %q: got type %d (%s), want %d (%s)",
				c.src, tok.Type, TokenName(tok.Type), c.typ, TokenName(c.typ))
		}
	}
}

// ---- identifiers -----------------------------------------------------------

func TestScanIdentifier(t *testing.T) {
	cases := []string{"x", "foo", "_bar", "CamelCase", "_1", "a123"}
	for _, c := range cases {
		tok := first(c)
		if tok.Type != TokName {
			t.Errorf("scan %q: got %s, want TokName", c, TokenName(tok.Type))
		}
		if tok.Val != c {
			t.Errorf("scan %q: val = %q, want %q", c, tok.Val, c)
		}
	}
}

func TestKeywordIsNotIdentifier(t *testing.T) {
	tok := first("if")
	if tok.Type == TokName {
		t.Error("'if' should not be scanned as TokName")
	}
}

// ---- numbers ---------------------------------------------------------------

func TestScanInteger(t *testing.T) {
	tok := first("42")
	if tok.Type != TokNumber {
		t.Fatalf("got %s, want TokNumber", TokenName(tok.Type))
	}
	if tok.Num != 42 {
		t.Errorf("num = %v, want 42", tok.Num)
	}
}

func TestScanFloat(t *testing.T) {
	cases := []struct {
		src string
		val float64
	}{
		{"3.14", 3.14},
		{"1e10", 1e10},
		{"1.5e-3", 1.5e-3},
		{"0.0", 0.0},
	}
	for _, c := range cases {
		tok := first(c.src)
		if tok.Type != TokNumber {
			t.Errorf("scan %q: got %s, want TokNumber", c.src, TokenName(tok.Type))
			continue
		}
		if tok.Num != c.val {
			t.Errorf("scan %q: num = %v, want %v", c.src, tok.Num, c.val)
		}
	}
}

func TestScanHex(t *testing.T) {
	cases := []struct {
		src string
		val float64
	}{
		{"0xff", 255},
		{"0xFF", 255},
		{"0x10", 16},
		{"0x0", 0},
	}
	for _, c := range cases {
		tok := first(c.src)
		if tok.Type != TokNumber {
			t.Errorf("scan %q: got %s, want TokNumber", c.src, TokenName(tok.Type))
			continue
		}
		if tok.Num != c.val {
			t.Errorf("scan %q: num = %v, want %v", c.src, tok.Num, c.val)
		}
	}
}

// ---- strings ---------------------------------------------------------------

func TestScanDoubleQuotedString(t *testing.T) {
	tok := first(`"hello"`)
	if tok.Type != TokString {
		t.Fatalf("got %s, want TokString", TokenName(tok.Type))
	}
	if tok.Val != "hello" {
		t.Errorf("val = %q, want %q", tok.Val, "hello")
	}
}

func TestScanSingleQuotedString(t *testing.T) {
	tok := first(`'world'`)
	if tok.Type != TokString {
		t.Fatalf("got %s, want TokString", TokenName(tok.Type))
	}
	if tok.Val != "world" {
		t.Errorf("val = %q, want %q", tok.Val, "world")
	}
}

func TestScanStringEscapes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"\n"`, "\n"},
		{`"\t"`, "\t"},
		{`"\r"`, "\r"},
		{`"\\"`, "\\"},
		{`"\""`, "\""},
		{`"\a"`, "\a"},
		{`"\b"`, "\b"},
		{`"\f"`, "\f"},
		{`"\v"`, "\v"},
	}
	for _, c := range cases {
		tok := first(c.src)
		if tok.Type != TokString {
			t.Errorf("scan %q: got %s, want TokString", c.src, TokenName(tok.Type))
			continue
		}
		if tok.Val != c.want {
			t.Errorf("scan %q: val = %q, want %q", c.src, tok.Val, c.want)
		}
	}
}

func TestScanDecimalEscape(t *testing.T) {
	// \065 = 'A'
	tok := first(`"\065"`)
	if tok.Type != TokString {
		t.Fatalf("got %s", TokenName(tok.Type))
	}
	if tok.Val != "A" {
		t.Errorf("val = %q, want %q", tok.Val, "A")
	}
}

func TestScanHexEscape(t *testing.T) {
	// \x41 = 'A'
	tok := first(`"\x41"`)
	if tok.Type != TokString {
		t.Fatalf("got %s", TokenName(tok.Type))
	}
	if tok.Val != "A" {
		t.Errorf("val = %q, want %q", tok.Val, "A")
	}
}

func TestScanLongString(t *testing.T) {
	tok := first("[[hello world]]")
	if tok.Type != TokString {
		t.Fatalf("got %s, want TokString", TokenName(tok.Type))
	}
	if tok.Val != "hello world" {
		t.Errorf("val = %q, want %q", tok.Val, "hello world")
	}
}

func TestScanLongStringLevel1(t *testing.T) {
	tok := first("[==[nested]==]")
	if tok.Type != TokString {
		t.Fatalf("got %s, want TokString", TokenName(tok.Type))
	}
	if tok.Val != "nested" {
		t.Errorf("val = %q, want %q", tok.Val, "nested")
	}
}

func TestScanLongStringStripsLeadingNewline(t *testing.T) {
	tok := first("[[\nhello]]")
	if tok.Type != TokString {
		t.Fatalf("got %s", TokenName(tok.Type))
	}
	if tok.Val != "hello" {
		t.Errorf("val = %q, want %q", tok.Val, "hello")
	}
}

// ---- operators -------------------------------------------------------------

func TestScanMultiCharOps(t *testing.T) {
	cases := []struct {
		src string
		typ int
	}{
		{"..", TokConcat},
		{"...", TokDots},
		{"==", TokEq},
		{">=", TokGE},
		{"<=", TokLE},
		{"~=", TokNE},
		{"::", TokDColon},
	}
	for _, c := range cases {
		tok := first(c.src)
		if tok.Type != c.typ {
			t.Errorf("scan %q: got %s, want %s", c.src, TokenName(tok.Type), TokenName(c.typ))
		}
	}
}

func TestScanSingleCharOps(t *testing.T) {
	for _, ch := range "+-*/^%&|<>(){}[];:,.#=" {
		src := string(ch)
		tok := first(src)
		if tok.Type != int(ch) {
			t.Errorf("scan %q: got %s, want %q", src, TokenName(tok.Type), ch)
		}
	}
}

// ---- comments --------------------------------------------------------------

func TestScanSingleLineCommentSkipped(t *testing.T) {
	// Comment should be skipped; next real token is x
	tok := first("-- this is a comment\nx")
	if tok.Type != TokName || tok.Val != "x" {
		t.Errorf("got %s %q, want TokName 'x'", TokenName(tok.Type), tok.Val)
	}
}

func TestScanBlockCommentSkipped(t *testing.T) {
	tok := first("--[[block comment]] y")
	if tok.Type != TokName || tok.Val != "y" {
		t.Errorf("got %s %q, want TokName 'y'", TokenName(tok.Type), tok.Val)
	}
}

func TestScanBlockCommentLevel2Skipped(t *testing.T) {
	tok := first("--[==[level 2 comment]==] z")
	if tok.Type != TokName || tok.Val != "z" {
		t.Errorf("got %s %q, want TokName 'z'", TokenName(tok.Type), tok.Val)
	}
}

// ---- positions -------------------------------------------------------------

func TestScanPositionFirstToken(t *testing.T) {
	tok := first("hello")
	if tok.Sp.From.Line != 0 || tok.Sp.From.Col != 0 || tok.Sp.From.Offset != 0 {
		t.Errorf("From = %+v, want {0,0,0}", tok.Sp.From)
	}
}

func TestScanPositionSecondLine(t *testing.T) {
	s := NewScanner("x\ny")
	s.Next()        // x
	tok := s.Next() // y
	if tok.Sp.From.Line != 1 {
		t.Errorf("y should be on line 1, got line %d", tok.Sp.From.Line)
	}
	if tok.Sp.From.Col != 0 {
		t.Errorf("y col = %d, want 0", tok.Sp.From.Col)
	}
}

func TestScanPositionOffset(t *testing.T) {
	// "ab cd" — 'cd' starts at offset 3
	s := NewScanner("ab cd")
	s.Next()        // ab
	tok := s.Next() // cd
	if tok.Sp.From.Offset != 3 {
		t.Errorf("cd offset = %d, want 3", tok.Sp.From.Offset)
	}
}

func TestScanEOFPosition(t *testing.T) {
	s := NewScanner("x")
	s.Next() // x
	eof := s.Next()
	if eof.Type != TokEOF {
		t.Fatalf("expected EOF, got %s", TokenName(eof.Type))
	}
}

// ---- peek ------------------------------------------------------------------

func TestScanPeekDoesNotConsume(t *testing.T) {
	s := NewScanner("a b")
	p := s.Peek()
	n := s.Next()
	if p.Type != n.Type || p.Val != n.Val {
		t.Errorf("Peek=%v Next=%v, should be equal", p, n)
	}
}

func TestScanPeekThenNext(t *testing.T) {
	s := NewScanner("one two")
	p := s.Peek()
	if p.Val != "one" {
		t.Errorf("peek val = %q, want %q", p.Val, "one")
	}
	n1 := s.Next()
	if n1.Val != "one" {
		t.Errorf("first next val = %q, want %q", n1.Val, "one")
	}
	n2 := s.Next()
	if n2.Val != "two" {
		t.Errorf("second next val = %q, want %q", n2.Val, "two")
	}
}

// ---- multi-token sequence --------------------------------------------------

func TestScanSequence(t *testing.T) {
	// local x = 42
	types := scanTypes("local x = 42")
	want := []int{TokLocal, TokName, int('='), TokNumber, TokEOF}
	if len(types) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(types), len(want), types)
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("token[%d]: got %s, want %s", i, TokenName(types[i]), TokenName(w))
		}
	}
}
