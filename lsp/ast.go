package main

// Pos represents a position in source code (0-based line, 0-based col, byte offset)
type Pos struct {
	Line   int // 0-based line
	Col    int // 0-based column (byte offset within line)
	Offset int // byte offset from start of file
}

// Span represents a range in source code (From inclusive, To exclusive of next char)
type Span struct {
	From Pos
	To   Pos
}

func (s Span) ToLSP() Range {
	return Range{
		Start: Position{Line: s.From.Line, Character: s.From.Col},
		End:   Position{Line: s.To.Line, Character: s.To.Col},
	}
}

// Contains reports whether p is within this span (by offset).
func (s Span) Contains(p Pos) bool {
	return p.Offset >= s.From.Offset && p.Offset <= s.To.Offset
}

// Node is the base interface for all AST nodes.
type Node interface {
	Span() Span
}

// Stmt is the interface for all statement nodes.
type Stmt interface {
	Node
	stmtMarker()
	stmtSpan() Span
}

// Expr is the interface for all expression nodes.
type Expr interface {
	Node
	exprMarker()
}

// ---------------------------------------------------------------------------
// Non-statement, non-expression helpers
// ---------------------------------------------------------------------------

// Ident represents an identifier.
type Ident struct {
	Name string
	Sp   Span
}

func (n *Ident) Span() Span { return n.Sp }

// Block is a sequence of statements with an optional trailing return.
type Block struct {
	Stmts []Stmt
	Ret   *ReturnStmt
	Sp    Span
}

func (n *Block) Span() Span { return n.Sp }

// IfClause is a single if/elseif branch (not itself a statement).
type IfClause struct {
	Cond Expr
	Body *Block
	Sp   Span
}

func (n *IfClause) Span() Span { return n.Sp }

// FuncName represents the name portion of a function statement.
type FuncName struct {
	Parts  []*Ident
	Method *Ident
	Sp     Span
}

func (n *FuncName) Span() Span { return n.Sp }

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// AssignStmt: targets = values
type AssignStmt struct {
	Targets []Expr
	Values  []Expr
	Sp      Span
}

func (n *AssignStmt) Span() Span    { return n.Sp }
func (n *AssignStmt) stmtMarker()   {}

// LocalStmt: local names [= values]
type LocalStmt struct {
	Names  []*Ident
	Values []Expr
	Sp     Span
}

func (n *LocalStmt) Span() Span  { return n.Sp }
func (n *LocalStmt) stmtMarker() {}

// DoStmt: do body end
type DoStmt struct {
	Body *Block
	Sp   Span
}

func (n *DoStmt) Span() Span  { return n.Sp }
func (n *DoStmt) stmtMarker() {}

// WhileStmt: while cond do body end
type WhileStmt struct {
	Cond Expr
	Body *Block
	Sp   Span
}

func (n *WhileStmt) Span() Span  { return n.Sp }
func (n *WhileStmt) stmtMarker() {}

// RepeatStmt: repeat body until cond
type RepeatStmt struct {
	Body *Block
	Cond Expr
	Sp   Span
}

func (n *RepeatStmt) Span() Span  { return n.Sp }
func (n *RepeatStmt) stmtMarker() {}

// IfStmt: if ... elseif ... else ... end
type IfStmt struct {
	Clauses  []IfClause
	ElseBody *Block // nil if no else
	Sp       Span
}

func (n *IfStmt) Span() Span  { return n.Sp }
func (n *IfStmt) stmtMarker() {}

// ForNumStmt: for name = start, limit [, step] do body end
type ForNumStmt struct {
	Name  *Ident
	Start Expr
	Limit Expr
	Step  Expr // nil if absent
	Body  *Block
	Sp    Span
}

func (n *ForNumStmt) Span() Span  { return n.Sp }
func (n *ForNumStmt) stmtMarker() {}

// ForInStmt: for names in values do body end
type ForInStmt struct {
	Names  []*Ident
	Values []Expr
	Body   *Block
	Sp     Span
}

func (n *ForInStmt) Span() Span  { return n.Sp }
func (n *ForInStmt) stmtMarker() {}

// FuncStmt: [local] function name(...) body end
type FuncStmt struct {
	IsLocal   bool
	Name      *FuncName // used when IsLocal == false
	LocalName *Ident    // used when IsLocal == true
	Func      *FuncExpr
	Sp        Span
}

func (n *FuncStmt) Span() Span  { return n.Sp }
func (n *FuncStmt) stmtMarker() {}

// ReturnStmt: return [values]
type ReturnStmt struct {
	Values []Expr
	Sp     Span
}

func (n *ReturnStmt) Span() Span  { return n.Sp }
func (n *ReturnStmt) stmtMarker() {}

// BreakStmt: break
type BreakStmt struct {
	Sp Span
}

func (n *BreakStmt) Span() Span  { return n.Sp }
func (n *BreakStmt) stmtMarker() {}

// GotoStmt: goto label
type GotoStmt struct {
	Label *Ident
	Sp    Span
}

func (n *GotoStmt) Span() Span  { return n.Sp }
func (n *GotoStmt) stmtMarker() {}

// LabelStmt: ::name::
type LabelStmt struct {
	Name *Ident
	Sp   Span
}

func (n *LabelStmt) Span() Span  { return n.Sp }
func (n *LabelStmt) stmtMarker() {}

// ExprStmt wraps a function-call expression used as a statement.
type ExprStmt struct {
	X  Expr
	Sp Span
}

func (n *ExprStmt) Span() Span  { return n.Sp }
func (n *ExprStmt) stmtMarker() {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// NilExpr: nil
type NilExpr struct {
	Sp Span
}

func (n *NilExpr) Span() Span    { return n.Sp }
func (n *NilExpr) exprMarker()   {}

// TrueExpr: true
type TrueExpr struct {
	Sp Span
}

func (n *TrueExpr) Span() Span  { return n.Sp }
func (n *TrueExpr) exprMarker() {}

// FalseExpr: false
type FalseExpr struct {
	Sp Span
}

func (n *FalseExpr) Span() Span  { return n.Sp }
func (n *FalseExpr) exprMarker() {}

// VarArgExpr: ...
type VarArgExpr struct {
	Sp Span
}

func (n *VarArgExpr) Span() Span  { return n.Sp }
func (n *VarArgExpr) exprMarker() {}

// NumberExpr: a numeric literal
type NumberExpr struct {
	Value float64
	Raw   string
	Sp    Span
}

func (n *NumberExpr) Span() Span  { return n.Sp }
func (n *NumberExpr) exprMarker() {}

// StringExpr: a string literal
type StringExpr struct {
	Value string
	Sp    Span
}

func (n *StringExpr) Span() Span  { return n.Sp }
func (n *StringExpr) exprMarker() {}

// NameExpr: a bare name reference
type NameExpr struct {
	Name string
	Sp   Span
}

func (n *NameExpr) Span() Span  { return n.Sp }
func (n *NameExpr) exprMarker() {}

// IndexExpr: table[key]
type IndexExpr struct {
	Table Expr
	Key   Expr
	Sp    Span
}

func (n *IndexExpr) Span() Span  { return n.Sp }
func (n *IndexExpr) exprMarker() {}

// FieldExpr: table.field
type FieldExpr struct {
	Table Expr
	Field *Ident
	Sp    Span
}

func (n *FieldExpr) Span() Span  { return n.Sp }
func (n *FieldExpr) exprMarker() {}

// UnaryExpr: op operand
type UnaryExpr struct {
	Op      string
	Operand Expr
	Sp      Span
}

func (n *UnaryExpr) Span() Span  { return n.Sp }
func (n *UnaryExpr) exprMarker() {}

// BinaryExpr: left op right
type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
	Sp    Span
}

func (n *BinaryExpr) Span() Span  { return n.Sp }
func (n *BinaryExpr) exprMarker() {}

// CallExpr: func(args) or obj:method(args)
type CallExpr struct {
	Func      Expr
	Method    *Ident // non-nil for obj:method(...) calls
	Args      []Expr
	OpenParen Pos
	Sp        Span
}

func (n *CallExpr) Span() Span  { return n.Sp }
func (n *CallExpr) exprMarker() {}

// FuncExpr: function(params) body end
type FuncExpr struct {
	Params    []*Ident
	HasVarArg bool
	Body      *Block
	Sp        Span
}

func (n *FuncExpr) Span() Span  { return n.Sp }
func (n *FuncExpr) exprMarker() {}

// TableExpr: { fields }
type TableExpr struct {
	Fields []TableField
	Sp     Span
}

func (n *TableExpr) Span() Span  { return n.Sp }
func (n *TableExpr) exprMarker() {}

// TableField is a single entry in a table constructor.
// Key==nil && Name==nil  → list-style value
// Name!=nil              → Name = Value style
// Key!=nil               → [Key] = Value style
type TableField struct {
	Key   Expr   // nil for list-style or name=val style
	Name  *Ident // non-nil for name=val style
	Value Expr
	Sp    Span
}

func (n *TableField) Span() Span { return n.Sp }

// ParenExpr: (inner)
type ParenExpr struct {
	Inner Expr
	Sp    Span
}

func (n *ParenExpr) Span() Span  { return n.Sp }
func (n *ParenExpr) exprMarker() {}
