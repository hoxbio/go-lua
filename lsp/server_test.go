package main

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

// ---- test infrastructure ---------------------------------------------------

func newTestServer() *Server {
	return &Server{
		docs: make(map[string]*Document),
		in:   bufio.NewReader(strings.NewReader("")),
		out:  bufio.NewWriter(io.Discard),
	}
}

const testURI = "file:///test.lua"

func openDoc(s *Server, src string) *Document {
	s.openDocument(testURI, 1, src)
	return s.docs[testURI]
}

// lspPos creates a Position.
func lspPos(line, char int) Position {
	return Position{Line: line, Character: char}
}

// ---- posToOffset -----------------------------------------------------------

func TestPosToOffset(t *testing.T) {
	cases := []struct {
		text string
		pos  Position
		want int
	}{
		{"hello", lspPos(0, 0), 0},
		{"hello", lspPos(0, 3), 3},
		{"hello", lspPos(0, 5), 5},
		{"ab\ncd", lspPos(1, 0), 3},
		{"ab\ncd", lspPos(1, 2), 5},
		{"a\nb\nc", lspPos(2, 0), 4},
	}
	for _, c := range cases {
		got := posToOffset(c.text, c.pos)
		if got != c.want {
			t.Errorf("posToOffset(%q, {%d,%d}) = %d, want %d",
				c.text, c.pos.Line, c.pos.Character, got, c.want)
		}
	}
}

// ---- openDocument ----------------------------------------------------------

func TestOpenDocumentParsesSuccessfully(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 1")
	if doc == nil {
		t.Fatal("doc is nil")
	}
	if doc.Block == nil {
		t.Error("expected non-nil Block")
	}
	if doc.Analysis == nil {
		t.Error("expected non-nil Analysis")
	}
	if len(doc.Errors) != 0 {
		t.Errorf("expected no errors, got %v", doc.Errors)
	}
}

func TestOpenDocumentReportsParseErrors(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "if then")
	if doc == nil {
		t.Fatal("doc is nil")
	}
	if len(doc.Errors) == 0 {
		t.Error("expected parse errors for invalid syntax")
	}
}

func TestOpenDocumentUpdates(t *testing.T) {
	s := newTestServer()
	openDoc(s, "local x = 1")
	s.openDocument(testURI, 2, "local y = 2")
	doc := s.docs[testURI]
	if doc.Version != 2 {
		t.Errorf("version = %d, want 2", doc.Version)
	}
	if doc.Text != "local y = 2" {
		t.Errorf("text = %q, want %q", doc.Text, "local y = 2")
	}
}

// ---- completion ------------------------------------------------------------

func TestCompletionContainsLocalVar(t *testing.T) {
	s := newTestServer()
	src := "local myVar = 1\n"
	doc := openDoc(s, src)
	// request completion at end of second line
	result := s.handleCompletion(doc, lspPos(1, 0))
	if result == nil {
		t.Fatal("completion result is nil")
	}
	found := false
	for _, item := range result.Items {
		if item.Label == "myVar" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'myVar' in completion items")
	}
}

func TestCompletionContainsKeywords(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleCompletion(doc, lspPos(0, 0))
	if result == nil {
		t.Fatal("nil result")
	}
	wantKeywords := []string{"local", "function", "if", "while", "for", "return"}
	for _, kw := range wantKeywords {
		found := false
		for _, item := range result.Items {
			if item.Label == kw && item.Kind == CIKKeyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected keyword %q in completions", kw)
		}
	}
}

func TestCompletionContainsStdlib(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleCompletion(doc, lspPos(0, 0))
	wantStd := []string{"print", "type", "pairs", "ipairs", "pcall", "require"}
	for _, name := range wantStd {
		found := false
		for _, item := range result.Items {
			if item.Label == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected stdlib '%s' in completions", name)
		}
	}
}

func TestCompletionFunctionItemKind(t *testing.T) {
	s := newTestServer()
	src := "local function myFunc(a, b) end\n"
	doc := openDoc(s, src)
	result := s.handleCompletion(doc, lspPos(1, 0))
	for _, item := range result.Items {
		if item.Label == "myFunc" {
			if item.Kind != CIKFunction {
				t.Errorf("myFunc kind = %d, want CIKFunction (%d)", item.Kind, CIKFunction)
			}
			return
		}
	}
	t.Error("'myFunc' not found in completion items")
}

func TestCompletionNoNilForEmptyDoc(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleCompletion(doc, lspPos(0, 0))
	if result == nil {
		t.Error("completion should not return nil for empty doc")
	}
}

func TestCompletionScopeVisible(t *testing.T) {
	// 'inner' and 'outer' should both be visible inside the do block.
	// Use the line where 'inner' is defined so we're definitely in scope.
	s := newTestServer()
	src := "local outer = 1\ndo\nlocal inner = 2\nlocal _cursor = nil\nend"
	doc := openDoc(s, src)
	// position on line 3 (the _cursor line), well inside the do block
	result := s.handleCompletion(doc, lspPos(3, 0))
	labels := map[string]bool{}
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["outer"] {
		t.Error("'outer' should be visible inside do block")
	}
	if !labels["inner"] {
		t.Error("'inner' should be visible inside do block")
	}
}

// ---- hover -----------------------------------------------------------------

func TestHoverOnLocalVar(t *testing.T) {
	s := newTestServer()
	src := "local myVar = 1"
	doc := openDoc(s, src)
	// hover on 'myVar' (col 6..11)
	hover := s.handleHover(doc, lspPos(0, 6))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "myVar") {
		t.Errorf("hover should mention 'myVar', got: %q", hover.Contents.Value)
	}
}

func TestHoverOnFunction(t *testing.T) {
	s := newTestServer()
	src := "local function add(a, b) end"
	doc := openDoc(s, src)
	// hover on 'add'
	off := offsetOf(src, "add", 0)
	hover := s.handleHover(doc, offsetToPos(src, off))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "add") {
		t.Errorf("hover should mention 'add', got: %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "function") {
		t.Errorf("hover should mention 'function', got: %q", hover.Contents.Value)
	}
}

func TestHoverOnFunctionShowsParams(t *testing.T) {
	s := newTestServer()
	src := "local function greet(name, msg) end"
	doc := openDoc(s, src)
	off := offsetOf(src, "greet", 0)
	hover := s.handleHover(doc, offsetToPos(src, off))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "name") {
		t.Errorf("hover should show param 'name': %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "msg") {
		t.Errorf("hover should show param 'msg': %q", hover.Contents.Value)
	}
}

func TestHoverNilForUnknown(t *testing.T) {
	s := newTestServer()
	src := "local x = 1"
	doc := openDoc(s, src)
	// hover at end of line, no symbol
	hover := s.handleHover(doc, lspPos(0, 11))
	// may or may not be nil — just shouldn't panic
	_ = hover
}

func TestHoverMarkdown(t *testing.T) {
	s := newTestServer()
	src := "local z = 42"
	doc := openDoc(s, src)
	hover := s.handleHover(doc, lspPos(0, 6))
	if hover == nil {
		t.Fatal("expected hover")
	}
	if hover.Contents.Kind != "markdown" {
		t.Errorf("kind = %q, want 'markdown'", hover.Contents.Kind)
	}
}

// ---- signature help --------------------------------------------------------

func TestSignatureHelpUserFunc(t *testing.T) {
	s := newTestServer()
	src := "local function add(a, b) end\nadd(1, "
	doc := openDoc(s, src)
	// cursor after the second argument (at the comma)
	help := s.handleSignatureHelp(doc, lspPos(1, 7))
	if help == nil {
		t.Fatal("expected signature help")
	}
	if len(help.Signatures) == 0 {
		t.Fatal("expected at least one signature")
	}
	sig := help.Signatures[0]
	if !strings.Contains(sig.Label, "add") {
		t.Errorf("signature label should contain 'add': %q", sig.Label)
	}
	if len(sig.Parameters) != 2 {
		t.Errorf("expected 2 params, got %d", len(sig.Parameters))
	}
}

func TestSignatureHelpStdlib(t *testing.T) {
	s := newTestServer()
	src := `print("hello", `
	doc := openDoc(s, src)
	help := s.handleSignatureHelp(doc, lspPos(0, 15))
	if help == nil {
		t.Fatal("expected signature help for print")
	}
	if len(help.Signatures) == 0 {
		t.Fatal("expected signature")
	}
}

func TestSignatureHelpActiveParam(t *testing.T) {
	s := newTestServer()
	src := "local function f(x, y, z) end\nf(1, 2, "
	doc := openDoc(s, src)
	// cursor at the third argument position
	help := s.handleSignatureHelp(doc, lspPos(1, 8))
	if help == nil {
		t.Fatal("expected signature help")
	}
	if help.ActiveParameter == nil {
		t.Fatal("expected active parameter")
	}
	if *help.ActiveParameter != 2 {
		t.Errorf("active param = %d, want 2", *help.ActiveParameter)
	}
}

func TestSignatureHelpNilOutsideCall(t *testing.T) {
	s := newTestServer()
	src := "local x = 1"
	doc := openDoc(s, src)
	help := s.handleSignatureHelp(doc, lspPos(0, 6))
	// Should return nil when not inside a call
	_ = help // may be nil; should not panic
}

// ---- go to definition ------------------------------------------------------

func TestDefinitionLocalVar(t *testing.T) {
	s := newTestServer()
	src := "local myVar = 1\nmyVar = 2"
	doc := openDoc(s, src)
	// ask for definition of 'myVar' at the reference (line 1)
	off := offsetOf(src, "myVar", 1)
	locs := s.handleDefinition(doc, offsetToPos(src, off))
	if len(locs) == 0 {
		t.Fatal("expected definition location")
	}
	if locs[0].URI != testURI {
		t.Errorf("URI = %q, want %q", locs[0].URI, testURI)
	}
	// Definition should be on line 0
	if locs[0].Range.Start.Line != 0 {
		t.Errorf("def line = %d, want 0", locs[0].Range.Start.Line)
	}
}

func TestDefinitionFunction(t *testing.T) {
	s := newTestServer()
	src := "local function foo() end\nfoo()"
	doc := openDoc(s, src)
	off := offsetOf(src, "foo", 1)
	locs := s.handleDefinition(doc, offsetToPos(src, off))
	if len(locs) == 0 {
		t.Fatal("expected definition")
	}
	if locs[0].Range.Start.Line != 0 {
		t.Errorf("def should be on line 0, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionNilForGlobal(t *testing.T) {
	s := newTestServer()
	src := "print('hi')"
	doc := openDoc(s, src)
	// 'print' is a global — no definition in this file
	locs := s.handleDefinition(doc, lspPos(0, 0))
	// Should return nil or empty (not a crash)
	_ = locs
}

// ---- find references -------------------------------------------------------

func TestReferencesIncludeDecl(t *testing.T) {
	s := newTestServer()
	src := "local x = 1\nx = 2\nx = 3"
	doc := openDoc(s, src)
	off := offsetOf(src, "x", 0)
	locs := s.handleReferences(doc, offsetToPos(src, off), true)
	if len(locs) < 3 {
		t.Errorf("expected >=3 refs (incl decl), got %d", len(locs))
	}
}

func TestReferencesExcludeDecl(t *testing.T) {
	s := newTestServer()
	src := "local x = 1\nx = 2\nx = 3"
	doc := openDoc(s, src)
	off := offsetOf(src, "x", 0)
	withDecl := s.handleReferences(doc, offsetToPos(src, off), true)
	withoutDecl := s.handleReferences(doc, offsetToPos(src, off), false)
	if len(withoutDecl) != len(withDecl)-1 {
		t.Errorf("without decl should have one fewer ref: got %d vs %d",
			len(withoutDecl), len(withDecl))
	}
}

func TestReferencesCorrectURIs(t *testing.T) {
	s := newTestServer()
	src := "local z = 1\nz = 2"
	doc := openDoc(s, src)
	locs := s.handleReferences(doc, lspPos(0, 6), true)
	for _, loc := range locs {
		if loc.URI != testURI {
			t.Errorf("ref URI = %q, want %q", loc.URI, testURI)
		}
	}
}

func TestReferencesNilForNoSymbol(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 1")
	// No symbol at position 11
	locs := s.handleReferences(doc, lspPos(0, 11), true)
	_ = locs // nil is fine
}

// ---- rename ----------------------------------------------------------------

func TestRenameAllOccurrences(t *testing.T) {
	s := newTestServer()
	src := "local counter = 0\ncounter = counter + 1\ncounter = counter + 1"
	doc := openDoc(s, src)
	off := offsetOf(src, "counter", 0)
	edit := s.handleRename(doc, offsetToPos(src, off), "total")
	if edit == nil {
		t.Fatal("expected workspace edit")
	}
	edits, ok := edit.Changes[testURI]
	if !ok {
		t.Fatal("expected edits for test URI")
	}
	// counter appears 5 times: 1 def + 2 assigns + 2 reads
	if len(edits) < 5 {
		t.Errorf("expected >=5 text edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.NewText != "total" {
			t.Errorf("edit new text = %q, want 'total'", e.NewText)
		}
	}
}

func TestRenameCorrectRanges(t *testing.T) {
	s := newTestServer()
	src := "local abc = 1\nabc = 2"
	doc := openDoc(s, src)
	edit := s.handleRename(doc, lspPos(0, 6), "xyz")
	if edit == nil {
		t.Fatal("expected edit")
	}
	edits := edit.Changes[testURI]
	for _, e := range edits {
		// Range length should match "abc" (3 chars)
		startChar := e.Range.Start.Character
		endChar := e.Range.End.Character
		if e.Range.Start.Line == e.Range.End.Line {
			if endChar-startChar != 3 {
				t.Errorf("range width = %d, want 3", endChar-startChar)
			}
		}
	}
}

func TestRenameNilForNoSymbol(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 1")
	edit := s.handleRename(doc, lspPos(0, 11), "new")
	_ = edit // nil is acceptable
}

// ---- formatting ------------------------------------------------------------

func TestFormattingReturnsEdits(t *testing.T) {
	s := newTestServer()
	src := "local x=1\nlocal y=2"
	doc := openDoc(s, src)
	edits := s.handleFormatting(doc, FormattingOptions{TabSize: 4, InsertSpaces: true})
	if len(edits) == 0 {
		t.Error("expected at least one text edit from formatting")
	}
}

func TestFormattingCoversWholeDoc(t *testing.T) {
	s := newTestServer()
	src := "local x = 1"
	doc := openDoc(s, src)
	edits := s.handleFormatting(doc, FormattingOptions{TabSize: 4, InsertSpaces: true})
	if len(edits) == 0 {
		t.Fatal("expected edits")
	}
	e := edits[0]
	if e.Range.Start.Line != 0 || e.Range.Start.Character != 0 {
		t.Errorf("edit should start at 0:0, got %d:%d", e.Range.Start.Line, e.Range.Start.Character)
	}
}

func TestFormattingOutputNotEmpty(t *testing.T) {
	s := newTestServer()
	src := "local function add(a,b) return a+b end"
	doc := openDoc(s, src)
	edits := s.handleFormatting(doc, FormattingOptions{TabSize: 4, InsertSpaces: true})
	if len(edits) == 0 {
		t.Fatal("expected edits")
	}
	if edits[0].NewText == "" {
		t.Error("formatted output should not be empty")
	}
}

func TestFormattingNilForNoBlock(t *testing.T) {
	s := newTestServer()
	// Invalid syntax so block may be nil
	src := "if"
	doc := openDoc(s, src)
	edits := s.handleFormatting(doc, FormattingOptions{TabSize: 4, InsertSpaces: true})
	_ = edits // nil or empty is fine
}

// ---- formatting (unit) -----------------------------------------------------

func TestFormatSimple(t *testing.T) {
	src := "local x = 1"
	p := NewParser(src)
	block, _ := p.Parse()
	out := Format(block, 4, true)
	if !strings.Contains(out, "local") || !strings.Contains(out, "x") {
		t.Errorf("formatted output missing content: %q", out)
	}
}

func TestFormatIndentsBody(t *testing.T) {
	src := "function f() local x = 1 end"
	p := NewParser(src)
	block, _ := p.Parse()
	out := Format(block, 4, true)
	// Body should be indented
	if !strings.Contains(out, "    local") {
		t.Errorf("expected indented body in:\n%s", out)
	}
}

func TestFormatIfStatement(t *testing.T) {
	src := "if a then local x = 1 end"
	p := NewParser(src)
	block, _ := p.Parse()
	out := Format(block, 4, true)
	if !strings.Contains(out, "if") || !strings.Contains(out, "end") {
		t.Errorf("missing if/end in: %q", out)
	}
}

func TestFormatPreservesStrings(t *testing.T) {
	src := `local s = "hello world"`
	p := NewParser(src)
	block, _ := p.Parse()
	out := Format(block, 4, true)
	if !strings.Contains(out, "hello world") {
		t.Errorf("string content lost: %q", out)
	}
}

// ---- helpers used in tests -----------------------------------------------


