package lsp

import "testing"

// ---- helpers ---------------------------------------------------------------

func analyzeSource(t *testing.T, src string) (*Block, *Analysis) {
	t.Helper()
	p := NewParser(src)
	block, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return block, Analyze(block)
}

// ---- basic locals ----------------------------------------------------------

func TestAnalyzeLocalDef(t *testing.T) {
	src := `local x = 1`
	_, a := analyzeSource(t, src)
	sym := a.SymbolAt(offsetOf(src, "x", 0))
	if sym == nil {
		t.Fatal("symbol 'x' not found")
	}
	if sym.Name != "x" {
		t.Errorf("name = %q, want 'x'", sym.Name)
	}
	if sym.Kind != SkLocal {
		t.Errorf("kind = %v, want SkLocal", sym.Kind)
	}
}

func TestAnalyzeLocalRef(t *testing.T) {
	src := `local x = 1
x = 2`
	_, a := analyzeSource(t, src)
	// Definition at first 'x'
	defOff := offsetOf(src, "x", 0)
	sym := a.SymbolAt(defOff)
	if sym == nil {
		t.Fatal("symbol not found at definition")
	}
	// Should have 2 refs: definition + assignment target
	if len(sym.Refs) < 2 {
		t.Errorf("expected >=2 refs, got %d", len(sym.Refs))
	}
	// Reference at second 'x'
	refOff := offsetOf(src, "x", 1)
	sym2 := a.SymbolAt(refOff)
	if sym2 == nil {
		t.Fatal("symbol not found at reference")
	}
	if sym2 != sym {
		t.Error("definition and reference should point to same symbol")
	}
}

func TestAnalyzeGlobal(t *testing.T) {
	src := `print("hello")`
	_, a := analyzeSource(t, src)
	spans, ok := a.Globals["print"]
	if !ok || len(spans) == 0 {
		t.Error("'print' should be a global reference")
	}
}

func TestAnalyzeLocalShadowsGlobal(t *testing.T) {
	src := `local print = 42
print = 99`
	_, a := analyzeSource(t, src)
	// 'print' should NOT be in globals since it's locally declared
	if _, ok := a.Globals["print"]; ok {
		t.Error("'print' should not be a global (it's declared local)")
	}
	sym := a.SymbolAt(offsetOf(src, "print", 0))
	if sym == nil || sym.Kind != SkLocal {
		t.Error("expected local symbol for 'print'")
	}
}

// ---- scoping ---------------------------------------------------------------

func TestAnalyzeScopeNesting(t *testing.T) {
	src := `local x = 1
do
    local y = 2
end`
	_, a := analyzeSource(t, src)

	xOff := offsetOf(src, "x", 0)
	xSym := a.SymbolAt(xOff)
	if xSym == nil {
		t.Fatal("x not found")
	}

	yOff := offsetOf(src, "y", 0)
	ySym := a.SymbolAt(yOff)
	if ySym == nil {
		t.Fatal("y not found")
	}

	// x and y should be in different scopes
	if xSym.Scope == ySym.Scope {
		t.Error("x and y should be in different scopes")
	}
}

func TestAnalyzeInnerScopeNotVisibleOuter(t *testing.T) {
	src := `do
    local inner = 1
end
inner = 2`
	_, a := analyzeSource(t, src)
	// The second 'inner' should be a global (not found in any scope)
	off := offsetOf(src, "inner", 1)
	sym := a.SymbolAt(off)
	// It's tracked as a global, not a symbol in scope
	if sym != nil {
		// If the second use is linked to the same symbol, that's a bug
		// (scoping should prevent this)
		if sym.Kind != SkGlobal {
			t.Error("outer 'inner' should not resolve to inner scope variable")
		}
	}
	if _, ok := a.Globals["inner"]; !ok {
		t.Error("'inner' outside do..end should be a global")
	}
}

func TestAnalyzeShadowing(t *testing.T) {
	src := `local x = 1
do
    local x = 2
    x = 3
end`
	_, a := analyzeSource(t, src)

	outerOff := offsetOf(src, "x", 0)
	innerOff := offsetOf(src, "x", 1)
	innerRefOff := offsetOf(src, "x", 2)

	outerSym := a.SymbolAt(outerOff)
	innerSym := a.SymbolAt(innerOff)

	if outerSym == nil || innerSym == nil {
		t.Fatal("symbols not found")
	}
	if outerSym == innerSym {
		t.Error("outer and inner 'x' should be distinct symbols")
	}

	// x = 3 inside the do block should refer to inner x, not outer x
	refSym := a.SymbolAt(innerRefOff)
	if refSym != innerSym {
		t.Error("'x = 3' should reference inner x")
	}
}

// ---- function parameters ---------------------------------------------------

func TestAnalyzeFuncParams(t *testing.T) {
	src := `function foo(a, b, c) return a + b end`
	_, a := analyzeSource(t, src)

	aOff := offsetOf(src, "a", 1) // second 'a' is in the body
	sym := a.SymbolAt(aOff)
	if sym == nil {
		t.Fatal("param 'a' not found in body")
	}
	if sym.Kind != SkParam {
		t.Errorf("expected SkParam, got %v", sym.Kind)
	}
}

func TestAnalyzeFuncSignature(t *testing.T) {
	src := `local function greet(name, greeting) end`
	_, a := analyzeSource(t, src)

	off := offsetOf(src, "greet", 0)
	sym := a.SymbolAt(off)
	if sym == nil {
		t.Fatal("symbol 'greet' not found")
	}
	if sym.FuncSig == nil {
		t.Fatal("expected FuncSig")
	}
	if len(sym.FuncSig.Params) != 2 {
		t.Errorf("params = %v, want [name, greeting]", sym.FuncSig.Params)
	}
	if sym.FuncSig.Params[0] != "name" {
		t.Errorf("param[0] = %q, want 'name'", sym.FuncSig.Params[0])
	}
}

func TestAnalyzeFuncVarArg(t *testing.T) {
	// Use a unique name to avoid partial matches with keywords.
	src := `local function myfunc(aa, ...) end`
	_, a := analyzeSource(t, src)
	sym := a.SymbolAt(offsetOf(src, "myfunc", 0))
	if sym == nil || sym.FuncSig == nil {
		t.Fatal("expected symbol with FuncSig")
	}
	if !sym.FuncSig.HasVarArg {
		t.Error("expected HasVarArg")
	}
	if len(sym.FuncSig.Params) != 1 {
		t.Errorf("params = %v, want [aa]", sym.FuncSig.Params)
	}
}

// ---- for-loop variables ----------------------------------------------------

func TestAnalyzeForNumVar(t *testing.T) {
	src := `for i = 1, 10 do
    local x = i * 2
end`
	_, a := analyzeSource(t, src)

	iOff := offsetOf(src, "i", 0)
	iSym := a.SymbolAt(iOff)
	if iSym == nil {
		t.Fatal("loop var 'i' not found")
	}
	if iSym.Kind != SkLocal {
		t.Errorf("loop var kind = %v, want SkLocal", iSym.Kind)
	}

	// 'i' inside the body should reference the same symbol
	iBodyOff := offsetOf(src, "i", 1)
	iBodySym := a.SymbolAt(iBodyOff)
	if iBodySym != iSym {
		t.Error("'i' in body should be same symbol as loop var")
	}
}

func TestAnalyzeForInVars(t *testing.T) {
	src := `for k, v in pairs(t) do
    print(k, v)
end`
	_, a := analyzeSource(t, src)

	kOff := offsetOf(src, "k", 0)
	kSym := a.SymbolAt(kOff)
	if kSym == nil || kSym.Kind != SkLocal {
		t.Fatal("loop var 'k' not found or wrong kind")
	}

	vOff := offsetOf(src, "v", 0)
	vSym := a.SymbolAt(vOff)
	if vSym == nil || vSym.Kind != SkLocal {
		t.Fatal("loop var 'v' not found or wrong kind")
	}

	// references inside body
	if len(kSym.Refs) < 2 {
		t.Errorf("k should have >=2 refs (def + use), got %d", len(kSym.Refs))
	}
}

// ---- labels ----------------------------------------------------------------

func TestAnalyzeLabel(t *testing.T) {
	src := `::done::
goto done`
	_, a := analyzeSource(t, src)

	labelOff := offsetOf(src, "done", 0)
	sym := a.SymbolAt(labelOff)
	if sym == nil {
		t.Fatal("label 'done' not found")
	}
	if sym.Kind != SkLabel {
		t.Errorf("kind = %v, want SkLabel", sym.Kind)
	}
}

// ---- ScopeAt ---------------------------------------------------------------

func TestAnalyzeScopeAt(t *testing.T) {
	src := `local x = 1
function foo()
    local y = 2
end`
	_, a := analyzeSource(t, src)

	// Inside function body, should be a child scope
	yOff := offsetOf(src, "y", 0)
	sc := a.ScopeAt(yOff)
	if sc == nil {
		t.Fatal("scope not found")
	}

	// Should be able to see y in this scope
	ySym := sc.Symbols["y"]
	if ySym == nil {
		t.Error("'y' should be in inner scope")
	}

	// x should NOT be in the inner scope (it's in parent)
	if _, ok := sc.Symbols["x"]; ok {
		t.Error("'x' should not be directly in inner scope")
	}

	// But lookup should find x via parent chain
	xSym := sc.Lookup("x")
	if xSym == nil {
		t.Error("Lookup('x') from inner scope should find x in parent")
	}
}

// ---- multiple references ---------------------------------------------------

func TestAnalyzeMultipleRefs(t *testing.T) {
	src := `local counter = 0
counter = counter + 1
counter = counter + 1`
	_, a := analyzeSource(t, src)

	off := offsetOf(src, "counter", 0)
	sym := a.SymbolAt(off)
	if sym == nil {
		t.Fatal("symbol not found")
	}
	// def + 2 assignments + 2 reads = 5 refs total
	if len(sym.Refs) < 5 {
		t.Errorf("expected >=5 refs, got %d: %v", len(sym.Refs), sym.Refs)
	}
}

// ---- anonymous function assigned to local ----------------------------------

func TestAnalyzeFuncExprAssigned(t *testing.T) {
	src := `local addfn = function(px, py) return px + py end`
	_, a := analyzeSource(t, src)

	// 'addfn' should be a local
	addOff := offsetOf(src, "addfn", 0)
	addSym := a.SymbolAt(addOff)
	if addSym == nil || addSym.Kind != SkLocal {
		t.Fatal("'addfn' should be a local symbol")
	}

	// 'px' param should be accessible (first occurrence = param definition)
	pxOff := offsetOf(src, "px", 0)
	pxSym := a.SymbolAt(pxOff)
	if pxSym == nil || pxSym.Kind != SkParam {
		t.Errorf("param 'px' should have SkParam, got %v", pxSym)
	}
}

// ---- scope chain lookup ----------------------------------------------------

func TestAnalyzeScopeLookupChain(t *testing.T) {
	sc := &Scope{
		Symbols: map[string]*Symbol{
			"x": {Name: "x", Kind: SkLocal},
		},
	}
	child := &Scope{
		Parent:  sc,
		Symbols: make(map[string]*Symbol),
	}

	sym := child.Lookup("x")
	if sym == nil {
		t.Fatal("expected to find 'x' via parent")
	}
	if sym.Name != "x" {
		t.Errorf("name = %q, want 'x'", sym.Name)
	}

	// Not found
	if child.Lookup("notexist") != nil {
		t.Error("expected nil for unknown name")
	}
}
