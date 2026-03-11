package main

import (
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func mustParse(t *testing.T, src string) *Block {
	t.Helper()
	p := NewParser(src)
	block, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return block
}

func parseErrors(src string) []SyntaxError {
	p := NewParser(src)
	_, errs := p.Parse()
	return errs
}

// ---- empty / trivial -------------------------------------------------------

func TestParseEmpty(t *testing.T) {
	block := mustParse(t, "")
	if len(block.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(block.Stmts))
	}
	if block.Ret != nil {
		t.Error("expected no return")
	}
}

func TestParseSemicolon(t *testing.T) {
	block := mustParse(t, ";;;")
	if len(block.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(block.Stmts))
	}
}

// ---- assignment ------------------------------------------------------------

func TestParseSimpleAssign(t *testing.T) {
	block := mustParse(t, "x = 1")
	if len(block.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(block.Stmts))
	}
	a, ok := block.Stmts[0].(*AssignStmt)
	if !ok {
		t.Fatalf("expected *AssignStmt, got %T", block.Stmts[0])
	}
	if len(a.Targets) != 1 || len(a.Values) != 1 {
		t.Errorf("targets=%d values=%d", len(a.Targets), len(a.Values))
	}
	tgt, ok := a.Targets[0].(*NameExpr)
	if !ok || tgt.Name != "x" {
		t.Errorf("target should be NameExpr('x'), got %T", a.Targets[0])
	}
}

func TestParseMultiAssign(t *testing.T) {
	block := mustParse(t, "a, b = 1, 2")
	a := block.Stmts[0].(*AssignStmt)
	if len(a.Targets) != 2 || len(a.Values) != 2 {
		t.Errorf("targets=%d values=%d", len(a.Targets), len(a.Values))
	}
}

// ---- local -----------------------------------------------------------------

func TestParseLocalNoValue(t *testing.T) {
	block := mustParse(t, "local x")
	s, ok := block.Stmts[0].(*LocalStmt)
	if !ok {
		t.Fatalf("expected *LocalStmt, got %T", block.Stmts[0])
	}
	if len(s.Names) != 1 || s.Names[0].Name != "x" {
		t.Errorf("names = %v", s.Names)
	}
	if len(s.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(s.Values))
	}
}

func TestParseLocalWithValue(t *testing.T) {
	block := mustParse(t, "local y = 42")
	s := block.Stmts[0].(*LocalStmt)
	if len(s.Values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(s.Values))
	}
	num, ok := s.Values[0].(*NumberExpr)
	if !ok || num.Value != 42 {
		t.Errorf("value should be 42, got %T %v", s.Values[0], s.Values[0])
	}
}

func TestParseLocalMulti(t *testing.T) {
	block := mustParse(t, "local a, b, c = 1, 2, 3")
	s := block.Stmts[0].(*LocalStmt)
	if len(s.Names) != 3 || len(s.Values) != 3 {
		t.Errorf("names=%d values=%d", len(s.Names), len(s.Values))
	}
}

// ---- do block --------------------------------------------------------------

func TestParseDoBlock(t *testing.T) {
	block := mustParse(t, "do local x = 1 end")
	do, ok := block.Stmts[0].(*DoStmt)
	if !ok {
		t.Fatalf("expected *DoStmt, got %T", block.Stmts[0])
	}
	if len(do.Body.Stmts) != 1 {
		t.Errorf("body stmts = %d", len(do.Body.Stmts))
	}
}

// ---- while -----------------------------------------------------------------

func TestParseWhile(t *testing.T) {
	block := mustParse(t, "while true do end")
	w, ok := block.Stmts[0].(*WhileStmt)
	if !ok {
		t.Fatalf("expected *WhileStmt, got %T", block.Stmts[0])
	}
	if _, ok := w.Cond.(*TrueExpr); !ok {
		t.Errorf("cond should be TrueExpr, got %T", w.Cond)
	}
	if len(w.Body.Stmts) != 0 {
		t.Errorf("body should be empty")
	}
}

// ---- repeat ----------------------------------------------------------------

func TestParseRepeat(t *testing.T) {
	block := mustParse(t, "repeat local x = 1 until x > 0")
	r, ok := block.Stmts[0].(*RepeatStmt)
	if !ok {
		t.Fatalf("expected *RepeatStmt, got %T", block.Stmts[0])
	}
	if len(r.Body.Stmts) != 1 {
		t.Errorf("body stmts = %d", len(r.Body.Stmts))
	}
	if _, ok := r.Cond.(*BinaryExpr); !ok {
		t.Errorf("cond should be BinaryExpr, got %T", r.Cond)
	}
}

// ---- if --------------------------------------------------------------------

func TestParseIfSimple(t *testing.T) {
	block := mustParse(t, "if x then end")
	s, ok := block.Stmts[0].(*IfStmt)
	if !ok {
		t.Fatalf("expected *IfStmt, got %T", block.Stmts[0])
	}
	if len(s.Clauses) != 1 {
		t.Errorf("expected 1 clause, got %d", len(s.Clauses))
	}
	if s.ElseBody != nil {
		t.Error("expected no else")
	}
}

func TestParseIfElseif(t *testing.T) {
	block := mustParse(t, "if a then elseif b then elseif c then else end")
	s := block.Stmts[0].(*IfStmt)
	if len(s.Clauses) != 3 {
		t.Errorf("expected 3 clauses, got %d", len(s.Clauses))
	}
	if s.ElseBody == nil {
		t.Error("expected else body")
	}
}

// ---- for numeric -----------------------------------------------------------

func TestParseForNum(t *testing.T) {
	block := mustParse(t, "for i = 1, 10 do end")
	f, ok := block.Stmts[0].(*ForNumStmt)
	if !ok {
		t.Fatalf("expected *ForNumStmt, got %T", block.Stmts[0])
	}
	if f.Name.Name != "i" {
		t.Errorf("name = %q, want %q", f.Name.Name, "i")
	}
	if f.Step != nil {
		t.Error("expected no step")
	}
}

func TestParseForNumStep(t *testing.T) {
	block := mustParse(t, "for i = 1, 10, 2 do end")
	f := block.Stmts[0].(*ForNumStmt)
	if f.Step == nil {
		t.Fatal("expected step")
	}
	num, ok := f.Step.(*NumberExpr)
	if !ok || num.Value != 2 {
		t.Errorf("step should be 2, got %T %v", f.Step, f.Step)
	}
}

// ---- for generic -----------------------------------------------------------

func TestParseForIn(t *testing.T) {
	block := mustParse(t, "for k, v in pairs(t) do end")
	f, ok := block.Stmts[0].(*ForInStmt)
	if !ok {
		t.Fatalf("expected *ForInStmt, got %T", block.Stmts[0])
	}
	if len(f.Names) != 2 {
		t.Errorf("names = %d, want 2", len(f.Names))
	}
	if f.Names[0].Name != "k" || f.Names[1].Name != "v" {
		t.Errorf("names = %v", f.Names)
	}
}

// ---- function statement ----------------------------------------------------

func TestParseFuncStmt(t *testing.T) {
	block := mustParse(t, "function foo(a, b) return a + b end")
	f, ok := block.Stmts[0].(*FuncStmt)
	if !ok {
		t.Fatalf("expected *FuncStmt, got %T", block.Stmts[0])
	}
	if f.IsLocal {
		t.Error("expected non-local")
	}
	if f.Name == nil || len(f.Name.Parts) != 1 || f.Name.Parts[0].Name != "foo" {
		t.Errorf("name = %v", f.Name)
	}
	if len(f.Func.Params) != 2 {
		t.Errorf("params = %d, want 2", len(f.Func.Params))
	}
}

func TestParseFuncDotName(t *testing.T) {
	block := mustParse(t, "function a.b.c() end")
	f := block.Stmts[0].(*FuncStmt)
	if len(f.Name.Parts) != 3 {
		t.Errorf("parts = %d, want 3", len(f.Name.Parts))
	}
}

func TestParseFuncMethodName(t *testing.T) {
	block := mustParse(t, "function obj:method() end")
	f := block.Stmts[0].(*FuncStmt)
	if f.Name.Method == nil || f.Name.Method.Name != "method" {
		t.Errorf("method = %v", f.Name.Method)
	}
}

func TestParseLocalFunc(t *testing.T) {
	block := mustParse(t, "local function bar(x) end")
	f, ok := block.Stmts[0].(*FuncStmt)
	if !ok {
		t.Fatalf("expected *FuncStmt, got %T", block.Stmts[0])
	}
	if !f.IsLocal {
		t.Error("expected IsLocal=true")
	}
	if f.LocalName == nil || f.LocalName.Name != "bar" {
		t.Errorf("LocalName = %v", f.LocalName)
	}
}

func TestParseFuncVarArg(t *testing.T) {
	block := mustParse(t, "function f(...) end")
	f := block.Stmts[0].(*FuncStmt)
	if !f.Func.HasVarArg {
		t.Error("expected HasVarArg")
	}
}

// ---- return ----------------------------------------------------------------

func TestParseReturn(t *testing.T) {
	block := mustParse(t, "return 1, 2, 3")
	if block.Ret == nil {
		t.Fatal("expected return statement")
	}
	if len(block.Ret.Values) != 3 {
		t.Errorf("return values = %d, want 3", len(block.Ret.Values))
	}
}

func TestParseReturnEmpty(t *testing.T) {
	block := mustParse(t, "return")
	if block.Ret == nil {
		t.Fatal("expected return statement")
	}
	if len(block.Ret.Values) != 0 {
		t.Errorf("return values = %d, want 0", len(block.Ret.Values))
	}
}

// ---- break / goto / label --------------------------------------------------

func TestParseBreak(t *testing.T) {
	block := mustParse(t, "while true do break end")
	body := block.Stmts[0].(*WhileStmt).Body
	if _, ok := body.Stmts[0].(*BreakStmt); !ok {
		t.Errorf("expected *BreakStmt, got %T", body.Stmts[0])
	}
}

func TestParseGoto(t *testing.T) {
	block := mustParse(t, "goto done")
	g, ok := block.Stmts[0].(*GotoStmt)
	if !ok {
		t.Fatalf("expected *GotoStmt, got %T", block.Stmts[0])
	}
	if g.Label.Name != "done" {
		t.Errorf("label = %q, want %q", g.Label.Name, "done")
	}
}

func TestParseLabel(t *testing.T) {
	block := mustParse(t, "::done::")
	l, ok := block.Stmts[0].(*LabelStmt)
	if !ok {
		t.Fatalf("expected *LabelStmt, got %T", block.Stmts[0])
	}
	if l.Name.Name != "done" {
		t.Errorf("name = %q, want %q", l.Name.Name, "done")
	}
}

// ---- expressions -----------------------------------------------------------

func TestParseNilExpr(t *testing.T) {
	block := mustParse(t, "x = nil")
	a := block.Stmts[0].(*AssignStmt)
	if _, ok := a.Values[0].(*NilExpr); !ok {
		t.Errorf("expected NilExpr, got %T", a.Values[0])
	}
}

func TestParseBoolExprs(t *testing.T) {
	block := mustParse(t, "x = true; y = false")
	a1 := block.Stmts[0].(*AssignStmt)
	a2 := block.Stmts[1].(*AssignStmt)
	if _, ok := a1.Values[0].(*TrueExpr); !ok {
		t.Errorf("expected TrueExpr, got %T", a1.Values[0])
	}
	if _, ok := a2.Values[0].(*FalseExpr); !ok {
		t.Errorf("expected FalseExpr, got %T", a2.Values[0])
	}
}

func TestParseStringExpr(t *testing.T) {
	block := mustParse(t, `x = "hello"`)
	a := block.Stmts[0].(*AssignStmt)
	s, ok := a.Values[0].(*StringExpr)
	if !ok {
		t.Fatalf("expected *StringExpr, got %T", a.Values[0])
	}
	if s.Value != "hello" {
		t.Errorf("val = %q, want %q", s.Value, "hello")
	}
}

func TestParseVarArgExpr(t *testing.T) {
	block := mustParse(t, "function f(...) return ... end")
	f := block.Stmts[0].(*FuncStmt)
	if _, ok := f.Func.Body.Ret.Values[0].(*VarArgExpr); !ok {
		t.Errorf("expected VarArgExpr, got %T", f.Func.Body.Ret.Values[0])
	}
}

// ---- binary expressions and precedence ------------------------------------

func TestParseBinaryAdd(t *testing.T) {
	block := mustParse(t, "x = a + b")
	a := block.Stmts[0].(*AssignStmt)
	bin, ok := a.Values[0].(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", a.Values[0])
	}
	if bin.Op != "+" {
		t.Errorf("op = %q, want %q", bin.Op, "+")
	}
}

func TestParsePrecedenceMulBeforeAdd(t *testing.T) {
	// a + b * c should be (a + (b * c))
	block := mustParse(t, "x = a + b * c")
	a := block.Stmts[0].(*AssignStmt)
	outer := a.Values[0].(*BinaryExpr)
	if outer.Op != "+" {
		t.Errorf("outer op = %q, want +", outer.Op)
	}
	inner, ok := outer.Right.(*BinaryExpr)
	if !ok || inner.Op != "*" {
		t.Errorf("inner should be *, got %T %v", outer.Right, outer.Right)
	}
}

func TestParsePrecedencePow(t *testing.T) {
	// 2^3^4 should be 2^(3^4) (right-associative)
	block := mustParse(t, "x = 2^3^4")
	a := block.Stmts[0].(*AssignStmt)
	outer := a.Values[0].(*BinaryExpr)
	if outer.Op != "^" {
		t.Errorf("outer op = %q, want ^", outer.Op)
	}
	// right should be 3^4
	inner, ok := outer.Right.(*BinaryExpr)
	if !ok || inner.Op != "^" {
		t.Errorf("inner should be ^, got %T", outer.Right)
	}
}

func TestParsePrecedenceConcat(t *testing.T) {
	// "a" .. "b" .. "c" should be right-assoc: "a" .. ("b" .. "c")
	block := mustParse(t, `x = "a" .. "b" .. "c"`)
	a := block.Stmts[0].(*AssignStmt)
	outer := a.Values[0].(*BinaryExpr)
	if outer.Op != ".." {
		t.Errorf("outer op = %q, want ..", outer.Op)
	}
	if _, ok := outer.Right.(*BinaryExpr); !ok {
		t.Errorf("right should be BinaryExpr (right-assoc), got %T", outer.Right)
	}
}

func TestParseUnaryNot(t *testing.T) {
	block := mustParse(t, "x = not true")
	a := block.Stmts[0].(*AssignStmt)
	u, ok := a.Values[0].(*UnaryExpr)
	if !ok || u.Op != "not" {
		t.Errorf("expected UnaryExpr(not), got %T", a.Values[0])
	}
}

func TestParseUnaryMinus(t *testing.T) {
	block := mustParse(t, "x = -1")
	a := block.Stmts[0].(*AssignStmt)
	u, ok := a.Values[0].(*UnaryExpr)
	if !ok || u.Op != "-" {
		t.Errorf("expected UnaryExpr(-), got %T", a.Values[0])
	}
}

func TestParseUnaryLen(t *testing.T) {
	block := mustParse(t, "x = #t")
	a := block.Stmts[0].(*AssignStmt)
	u, ok := a.Values[0].(*UnaryExpr)
	if !ok || u.Op != "#" {
		t.Errorf("expected UnaryExpr(#), got %T", a.Values[0])
	}
}

// ---- field / index access --------------------------------------------------

func TestParseFieldExpr(t *testing.T) {
	block := mustParse(t, "x = t.field")
	a := block.Stmts[0].(*AssignStmt)
	f, ok := a.Values[0].(*FieldExpr)
	if !ok {
		t.Fatalf("expected *FieldExpr, got %T", a.Values[0])
	}
	if f.Field.Name != "field" {
		t.Errorf("field = %q, want %q", f.Field.Name, "field")
	}
}

func TestParseIndexExpr(t *testing.T) {
	block := mustParse(t, "x = t[1]")
	a := block.Stmts[0].(*AssignStmt)
	idx, ok := a.Values[0].(*IndexExpr)
	if !ok {
		t.Fatalf("expected *IndexExpr, got %T", a.Values[0])
	}
	num, ok := idx.Key.(*NumberExpr)
	if !ok || num.Value != 1 {
		t.Errorf("key should be 1, got %T", idx.Key)
	}
}

// ---- function calls --------------------------------------------------------

func TestParseFuncCall(t *testing.T) {
	block := mustParse(t, "foo(1, 2)")
	es, ok := block.Stmts[0].(*ExprStmt)
	if !ok {
		t.Fatalf("expected *ExprStmt, got %T", block.Stmts[0])
	}
	call, ok := es.X.(*CallExpr)
	if !ok {
		t.Fatalf("expected *CallExpr, got %T", es.X)
	}
	if len(call.Args) != 2 {
		t.Errorf("args = %d, want 2", len(call.Args))
	}
	if call.Method != nil {
		t.Error("expected no method")
	}
}

func TestParseMethodCall(t *testing.T) {
	block := mustParse(t, "obj:method(x)")
	es := block.Stmts[0].(*ExprStmt)
	call := es.X.(*CallExpr)
	if call.Method == nil || call.Method.Name != "method" {
		t.Errorf("method = %v, want 'method'", call.Method)
	}
}

func TestParseFuncCallNoArgs(t *testing.T) {
	block := mustParse(t, "f()")
	es := block.Stmts[0].(*ExprStmt)
	call := es.X.(*CallExpr)
	if len(call.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(call.Args))
	}
}

func TestParseFuncCallStringArg(t *testing.T) {
	block := mustParse(t, `require "module"`)
	es := block.Stmts[0].(*ExprStmt)
	call := es.X.(*CallExpr)
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*StringExpr); !ok {
		t.Errorf("expected StringExpr arg, got %T", call.Args[0])
	}
}

// ---- table constructors ----------------------------------------------------

func TestParseEmptyTable(t *testing.T) {
	block := mustParse(t, "x = {}")
	a := block.Stmts[0].(*AssignStmt)
	tbl, ok := a.Values[0].(*TableExpr)
	if !ok {
		t.Fatalf("expected *TableExpr, got %T", a.Values[0])
	}
	if len(tbl.Fields) != 0 {
		t.Errorf("fields = %d, want 0", len(tbl.Fields))
	}
}

func TestParseTableListStyle(t *testing.T) {
	block := mustParse(t, "x = {1, 2, 3}")
	a := block.Stmts[0].(*AssignStmt)
	tbl := a.Values[0].(*TableExpr)
	if len(tbl.Fields) != 3 {
		t.Errorf("fields = %d, want 3", len(tbl.Fields))
	}
	for i, f := range tbl.Fields {
		if f.Name != nil || f.Key != nil {
			t.Errorf("field[%d] should be list-style", i)
		}
	}
}

func TestParseTableNameStyle(t *testing.T) {
	block := mustParse(t, "x = {a = 1, b = 2}")
	a := block.Stmts[0].(*AssignStmt)
	tbl := a.Values[0].(*TableExpr)
	if len(tbl.Fields) != 2 {
		t.Errorf("fields = %d, want 2", len(tbl.Fields))
	}
	if tbl.Fields[0].Name == nil || tbl.Fields[0].Name.Name != "a" {
		t.Errorf("field[0] name = %v, want 'a'", tbl.Fields[0].Name)
	}
}

func TestParseTableKeyStyle(t *testing.T) {
	block := mustParse(t, `x = {["key"] = "val"}`)
	a := block.Stmts[0].(*AssignStmt)
	tbl := a.Values[0].(*TableExpr)
	if len(tbl.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(tbl.Fields))
	}
	if tbl.Fields[0].Key == nil {
		t.Error("expected key expression")
	}
}

// ---- function expression ---------------------------------------------------

func TestParseFuncExpr(t *testing.T) {
	block := mustParse(t, "x = function(a, b) return a end")
	a := block.Stmts[0].(*AssignStmt)
	f, ok := a.Values[0].(*FuncExpr)
	if !ok {
		t.Fatalf("expected *FuncExpr, got %T", a.Values[0])
	}
	if len(f.Params) != 2 {
		t.Errorf("params = %d, want 2", len(f.Params))
	}
}

// ---- parens ----------------------------------------------------------------

func TestParseParenExpr(t *testing.T) {
	block := mustParse(t, "x = (a + b)")
	a := block.Stmts[0].(*AssignStmt)
	_, ok := a.Values[0].(*ParenExpr)
	if !ok {
		t.Errorf("expected *ParenExpr, got %T", a.Values[0])
	}
}

// ---- positions -------------------------------------------------------------

func TestParsePositionLocalStmt(t *testing.T) {
	block := mustParse(t, "local x = 1")
	s := block.Stmts[0].(*LocalStmt)
	if s.Sp.From.Line != 0 || s.Sp.From.Col != 0 {
		t.Errorf("local stmt From = %+v, want {0,0}", s.Sp.From)
	}
}

func TestParsePositionIdentifier(t *testing.T) {
	// "local x" — 'x' is at col 6
	block := mustParse(t, "local x")
	s := block.Stmts[0].(*LocalStmt)
	name := s.Names[0]
	if name.Sp.From.Col != 6 {
		t.Errorf("'x' col = %d, want 6", name.Sp.From.Col)
	}
}

func TestParsePositionMultiLine(t *testing.T) {
	src := "local a\nlocal b"
	block := mustParse(t, src)
	if len(block.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(block.Stmts))
	}
	s2 := block.Stmts[1].(*LocalStmt)
	if s2.Sp.From.Line != 1 {
		t.Errorf("second stmt line = %d, want 1", s2.Sp.From.Line)
	}
}

// ---- error recovery --------------------------------------------------------

func TestParseErrorRecovery(t *testing.T) {
	// Missing 'then' after condition — parser should recover and continue
	_, errs := NewParser("if x end").Parse()
	if len(errs) == 0 {
		t.Error("expected at least one parse error")
	}
}

func TestParseErrorMultiple(t *testing.T) {
	// Deliberately broken code
	errs := parseErrors("if then end while do end")
	if len(errs) == 0 {
		t.Error("expected parse errors for broken code")
	}
}

func TestParseValidCodeNoErrors(t *testing.T) {
	src := `
local function fib(n)
    if n <= 1 then
        return n
    end
    return fib(n - 1) + fib(n - 2)
end
print(fib(10))
`
	errs := parseErrors(src)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

// ---- complex program -------------------------------------------------------

func TestParseComplexProgram(t *testing.T) {
	src := `
local t = {}
for i = 1, 10 do
    t[i] = i * i
end
for k, v in pairs(t) do
    print(k, v)
end
local function sum(a, b, ...)
    local s = a + b
    return s
end
`
	block := mustParse(t, src)
	if len(block.Stmts) == 0 {
		t.Error("expected non-empty block")
	}
}
