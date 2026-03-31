package lsp

import "sort"

// ---------------------------------------------------------------------------
// Symbol kinds
// ---------------------------------------------------------------------------

// SymbolKind classifies a symbol's binding origin.
type SymbolKind int

const (
	SkLocal  SymbolKind = iota // local variable
	SkParam                    // function parameter
	SkGlobal                   // global reference (unresolved)
	SkLabel                    // ::label::
)

// ---------------------------------------------------------------------------
// Core data structures
// ---------------------------------------------------------------------------

// FuncSig stores the parameter information for a function symbol.
type FuncSig struct {
	Params    []string
	HasVarArg bool
}

// Symbol is a named binding in the Lua source.
type Symbol struct {
	Name    string
	Kind    SymbolKind
	Def     Span   // span of the defining occurrence
	Refs    []Span // all reference spans (including definition)
	Scope   *Scope
	FuncSig *FuncSig // non-nil when the symbol resolves to a function
}

// Scope is a lexical scope that holds a set of symbols.
type Scope struct {
	Parent   *Scope
	Symbols  map[string]*Symbol
	Children []*Scope
	Sp       Span
}

// Lookup searches this scope and all ancestor scopes for name.
func (sc *Scope) Lookup(name string) *Symbol {
	for s := sc; s != nil; s = s.Parent {
		if sym, ok := s.Symbols[name]; ok {
			return sym
		}
	}
	return nil
}

// symbolRef pairs a byte offset with the symbol it refers to.
type symbolRef struct {
	offset int
	sym    *Symbol
}

// Analysis is the result of walking a parsed Lua block.
type Analysis struct {
	Root     *Scope
	ByOffset []*symbolRef // sorted by offset, covering all def+ref spans
	Globals  map[string][]Span
	Errors   []AnalysisError
}

// AnalysisError records a semantic error found during analysis.
type AnalysisError struct {
	Msg string
	Sp  Span
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

// Analyze walks block, builds the scope tree, resolves symbols, and returns
// the completed Analysis.
func Analyze(block *Block) *Analysis {
	a := &Analysis{
		Root:    newScope(nil, block.Sp),
		Globals: make(map[string][]Span),
	}
	a.analyzeBlock(block, a.Root)
	sort.Slice(a.ByOffset, func(i, j int) bool {
		return a.ByOffset[i].offset < a.ByOffset[j].offset
	})
	return a
}

// ---------------------------------------------------------------------------
// Lookup helpers
// ---------------------------------------------------------------------------

// SymbolAt returns the symbol whose definition or any reference span contains
// the given byte offset, or nil if none is found.
func (a *Analysis) SymbolAt(offset int) *Symbol {
	// Binary-search for the first entry whose offset >= the query.
	idx := sort.Search(len(a.ByOffset), func(i int) bool {
		return a.ByOffset[i].offset >= offset
	})
	// Check the entry at idx and the one just before it; a span's start is
	// recorded, so the match may be at idx-1.
	for _, i := range []int{idx - 1, idx} {
		if i < 0 || i >= len(a.ByOffset) {
			continue
		}
		ref := a.ByOffset[i]
		sym := ref.sym
		if sym.Def.Contains(Pos{Offset: offset}) {
			return sym
		}
		for _, sp := range sym.Refs {
			if sp.Contains(Pos{Offset: offset}) {
				return sym
			}
		}
	}
	return nil
}

// ScopeAt returns the innermost scope whose span contains the given byte offset.
func (a *Analysis) ScopeAt(offset int) *Scope {
	return scopeAt(a.Root, offset)
}

func scopeAt(sc *Scope, offset int) *Scope {
	// Prefer a child that contains the offset (innermost wins).
	for _, child := range sc.Children {
		if child.Sp.Contains(Pos{Offset: offset}) {
			return scopeAt(child, offset)
		}
	}
	return sc
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func newScope(parent *Scope, sp Span) *Scope {
	sc := &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
		Sp:      sp,
	}
	if parent != nil {
		parent.Children = append(parent.Children, sc)
	}
	return sc
}

// define creates a new symbol in sc and registers its definition span.
func (a *Analysis) define(sc *Scope, name string, kind SymbolKind, sp Span) *Symbol {
	sym := &Symbol{
		Name:  name,
		Kind:  kind,
		Def:   sp,
		Refs:  []Span{sp},
		Scope: sc,
	}
	sc.Symbols[name] = sym
	a.recordRef(sp, sym)
	return sym
}

// addRef records a reference span for sym.
func (a *Analysis) addRef(sym *Symbol, sp Span) {
	sym.Refs = append(sym.Refs, sp)
	a.recordRef(sp, sym)
}

// recordRef inserts a symbolRef at sp.From.Offset into ByOffset.
func (a *Analysis) recordRef(sp Span, sym *Symbol) {
	a.ByOffset = append(a.ByOffset, &symbolRef{offset: sp.From.Offset, sym: sym})
}

// resolve attempts to find name in the scope chain; on failure it is recorded
// as a global reference.
func (a *Analysis) resolve(sc *Scope, name string, sp Span) {
	if sym := sc.Lookup(name); sym != nil {
		a.addRef(sym, sp)
		return
	}
	// Global
	a.Globals[name] = append(a.Globals[name], sp)
}

// ---------------------------------------------------------------------------
// Block / statement / expression walkers
// ---------------------------------------------------------------------------

func (a *Analysis) analyzeBlock(block *Block, sc *Scope) {
	for _, stmt := range block.Stmts {
		a.analyzeStmt(stmt, sc)
	}
	if block.Ret != nil {
		a.analyzeStmt(block.Ret, sc)
	}
}

func (a *Analysis) analyzeStmt(stmt Stmt, sc *Scope) {
	switch s := stmt.(type) {

	case *AssignStmt:
		for _, t := range s.Targets {
			a.analyzeExpr(t, sc)
		}
		for _, v := range s.Values {
			a.analyzeExpr(v, sc)
		}

	case *LocalStmt:
		// Evaluate RHS in current scope before defining names (Lua semantics).
		for _, v := range s.Values {
			a.analyzeExpr(v, sc)
		}
		for _, ident := range s.Names {
			a.define(sc, ident.Name, SkLocal, ident.Sp)
		}

	case *DoStmt:
		inner := newScope(sc, s.Body.Sp)
		a.analyzeBlock(s.Body, inner)

	case *WhileStmt:
		a.analyzeExpr(s.Cond, sc)
		inner := newScope(sc, s.Body.Sp)
		a.analyzeBlock(s.Body, inner)

	case *RepeatStmt:
		// The condition can see locals declared inside the body.
		inner := newScope(sc, s.Body.Sp)
		a.analyzeBlock(s.Body, inner)
		a.analyzeExpr(s.Cond, inner)

	case *IfStmt:
		for _, clause := range s.Clauses {
			a.analyzeExpr(clause.Cond, sc)
			inner := newScope(sc, clause.Body.Sp)
			a.analyzeBlock(clause.Body, inner)
		}
		if s.ElseBody != nil {
			inner := newScope(sc, s.ElseBody.Sp)
			a.analyzeBlock(s.ElseBody, inner)
		}

	case *ForNumStmt:
		// Start, Limit, Step are evaluated in the outer scope.
		a.analyzeExpr(s.Start, sc)
		a.analyzeExpr(s.Limit, sc)
		if s.Step != nil {
			a.analyzeExpr(s.Step, sc)
		}
		inner := newScope(sc, s.Body.Sp)
		a.define(inner, s.Name.Name, SkLocal, s.Name.Sp)
		a.analyzeBlock(s.Body, inner)

	case *ForInStmt:
		// Iterator expressions evaluated in outer scope.
		for _, v := range s.Values {
			a.analyzeExpr(v, sc)
		}
		inner := newScope(sc, s.Body.Sp)
		for _, ident := range s.Names {
			a.define(inner, ident.Name, SkLocal, ident.Sp)
		}
		a.analyzeBlock(s.Body, inner)

	case *FuncStmt:
		if s.IsLocal {
			// local function f(...)
			sym := a.define(sc, s.LocalName.Name, SkLocal, s.LocalName.Sp)
			sig := a.analyzeFuncExpr(s.Func, sc)
			sym.FuncSig = sig
		} else {
			// function a.b.c:method(...)
			// The first part is a plain name reference.
			parts := s.Name.Parts
			if len(parts) > 0 {
				a.resolve(sc, parts[0].Name, parts[0].Sp)
			}
			// Remaining parts are field accesses — just record them as refs
			// to themselves (no scope lookup needed for field names).
			for _, p := range parts[1:] {
				// Field name identifiers: record but don't scope-resolve.
				_ = p
			}
			// Analyse the function body; if there is a method receiver,
			// inject an implicit "self" parameter.
			inner := newScope(sc, s.Func.Body.Sp)
			if s.Name.Method != nil {
				selfSpan := s.Name.Method.Sp
				a.define(inner, "self", SkParam, selfSpan)
			}
			for _, param := range s.Func.Params {
				a.define(inner, param.Name, SkParam, param.Sp)
			}
			if s.Func.HasVarArg {
				// vararg is not a named symbol
			}
			a.analyzeBlock(s.Func.Body, inner)
		}

	case *ReturnStmt:
		for _, v := range s.Values {
			a.analyzeExpr(v, sc)
		}

	case *BreakStmt:
		// nothing to resolve

	case *GotoStmt:
		// Try to resolve the label in the scope chain.
		if sym := sc.Lookup(s.Label.Name); sym != nil && sym.Kind == SkLabel {
			a.addRef(sym, s.Label.Sp)
		}
		// Unresolved gotos are not recorded as globals.

	case *LabelStmt:
		a.define(sc, s.Name.Name, SkLabel, s.Name.Sp)

	case *ExprStmt:
		a.analyzeExpr(s.X, sc)
	}
}

// analyzeFuncExpr analyses the body of a function literal, defining params in
// a fresh child scope, and returns the FuncSig for the symbol.
func (a *Analysis) analyzeFuncExpr(fn *FuncExpr, sc *Scope) *FuncSig {
	inner := newScope(sc, fn.Body.Sp)
	sig := &FuncSig{HasVarArg: fn.HasVarArg}
	for _, param := range fn.Params {
		a.define(inner, param.Name, SkParam, param.Sp)
		sig.Params = append(sig.Params, param.Name)
	}
	a.analyzeBlock(fn.Body, inner)
	return sig
}

func (a *Analysis) analyzeExpr(expr Expr, sc *Scope) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {

	case *NilExpr, *TrueExpr, *FalseExpr, *VarArgExpr, *NumberExpr, *StringExpr:
		// literals — nothing to resolve

	case *NameExpr:
		a.resolve(sc, e.Name, e.Sp)

	case *IndexExpr:
		a.analyzeExpr(e.Table, sc)
		a.analyzeExpr(e.Key, sc)

	case *FieldExpr:
		a.analyzeExpr(e.Table, sc)
		// e.Field is an identifier used as a field name, not a scope lookup.

	case *UnaryExpr:
		a.analyzeExpr(e.Operand, sc)

	case *BinaryExpr:
		a.analyzeExpr(e.Left, sc)
		a.analyzeExpr(e.Right, sc)

	case *CallExpr:
		a.analyzeExpr(e.Func, sc)
		// e.Method, if set, is a method name accessed via ":", not a scope lookup.
		for _, arg := range e.Args {
			a.analyzeExpr(arg, sc)
		}

	case *FuncExpr:
		sig := a.analyzeFuncExpr(e, sc)
		_ = sig // anonymous function; no symbol to attach sig to

	case *TableExpr:
		for _, field := range e.Fields {
			if field.Key != nil {
				a.analyzeExpr(field.Key, sc)
			}
			a.analyzeExpr(field.Value, sc)
		}

	case *ParenExpr:
		a.analyzeExpr(e.Inner, sc)
	}
}
