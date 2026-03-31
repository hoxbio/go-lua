package lsp

import (
	"fmt"
)

// SyntaxError holds a parse error message and its source location.
type SyntaxError struct {
	Msg string
	Sp  Span
}

func (e SyntaxError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Sp.From.Line+1, e.Sp.From.Col+1, e.Msg)
}

// Parser is a Lua 5.2 recursive-descent parser.
type Parser struct {
	s      *Scanner
	tok    Token
	errors []SyntaxError
}

// NewParser creates a new Parser for the given source string.
func NewParser(src string) *Parser {
	p := &Parser{s: NewScanner(src)}
	p.tok = p.s.Next()
	return p
}

// next advances to the next token.
func (p *Parser) next() {
	p.tok = p.s.Next()
}

// peek returns the next token without consuming it.
func (p *Parser) peek() Token {
	return p.s.Peek()
}

// check returns true if the current token has the given type.
func (p *Parser) check(typ int) bool {
	return p.tok.Type == typ
}

// expect consumes and returns the current token if it matches typ,
// otherwise records an error and returns a synthetic token at the current position.
func (p *Parser) expect(typ int) Token {
	if p.tok.Type == typ {
		t := p.tok
		p.next()
		return t
	}
	msg := fmt.Sprintf("expected %s, got %s", TokenName(typ), TokenName(p.tok.Type))
	p.addError(msg, p.tok.Sp)
	return Token{Type: typ, Sp: p.tok.Sp}
}

// addError records a parse error.
func (p *Parser) addError(msg string, sp Span) {
	p.errors = append(p.errors, SyntaxError{Msg: msg, Sp: sp})
}

// sync skips tokens until a synchronization point is found.
func (p *Parser) sync() {
	for {
		switch p.tok.Type {
		case TokEOF, TokEnd, TokElse, TokElseif, TokUntil, TokReturn, int(';'):
			return
		}
		p.next()
	}
}

// Parse parses the entire source file and returns the top-level block plus any errors.
func (p *Parser) Parse() (*Block, []SyntaxError) {
	block := p.parseBlock()
	if p.tok.Type != TokEOF {
		p.addError(fmt.Sprintf("unexpected token %s", TokenName(p.tok.Type)), p.tok.Sp)
	}
	return block, p.errors
}

// parseBlock parses a sequence of statements (possibly ending with a return statement).
func (p *Parser) parseBlock() *Block {
	from := p.tok.Sp.From
	block := &Block{}
	for {
		// Skip semicolons
		for p.tok.Type == int(';') {
			p.next()
		}
		// Check for block-ending tokens
		if p.isBlockEnd() {
			break
		}
		// Return statement must be the last statement in a block
		if p.tok.Type == TokReturn {
			block.Ret = p.parseReturn()
			// optional trailing semicolon
			if p.tok.Type == int(';') {
				p.next()
			}
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Stmts = append(block.Stmts, stmt)
		}
	}
	// Use the closing keyword's position so the block span covers trailing
	// blank lines (e.g. a blank line before 'end' is inside the block).
	to := p.tok.Sp.From
	block.Sp = Span{From: from, To: to}
	return block
}

// isBlockEnd returns true if the current token ends a block.
func (p *Parser) isBlockEnd() bool {
	switch p.tok.Type {
	case TokEOF, TokEnd, TokElse, TokElseif, TokUntil:
		return true
	}
	return false
}

// parseStatement parses a single statement.
func (p *Parser) parseStatement() Stmt {
	from := p.tok.Sp.From
	switch p.tok.Type {
	case TokDo:
		return p.parseDo(from)
	case TokWhile:
		return p.parseWhile(from)
	case TokRepeat:
		return p.parseRepeat(from)
	case TokIf:
		return p.parseIf(from)
	case TokFor:
		return p.parseFor(from)
	case TokFunction:
		return p.parseFuncStmt(from)
	case TokLocal:
		return p.parseLocal(from)
	case TokGoto:
		return p.parseGoto(from)
	case TokBreak:
		tok := p.tok
		p.next()
		return &BreakStmt{Sp: tok.Sp}
	case TokDColon:
		return p.parseLabel(from)
	default:
		return p.parseExprStat(from)
	}
}

// parseDo parses a do...end block.
func (p *Parser) parseDo(from Pos) *DoStmt {
	p.next() // consume 'do'
	body := p.parseBlock()
	end := p.expect(TokEnd)
	return &DoStmt{Body: body, Sp: Span{From: from, To: end.Sp.To}}
}

// parseWhile parses a while...do...end statement.
func (p *Parser) parseWhile(from Pos) *WhileStmt {
	p.next() // consume 'while'
	cond := p.parseExpr()
	p.expect(TokDo)
	body := p.parseBlock()
	end := p.expect(TokEnd)
	return &WhileStmt{Cond: cond, Body: body, Sp: Span{From: from, To: end.Sp.To}}
}

// parseRepeat parses a repeat...until statement.
func (p *Parser) parseRepeat(from Pos) *RepeatStmt {
	p.next() // consume 'repeat'
	body := p.parseBlock()
	p.expect(TokUntil)
	cond := p.parseExpr()
	sp := spanOf(cond)
	return &RepeatStmt{Body: body, Cond: cond, Sp: Span{From: from, To: sp.To}}
}

// parseIf parses an if...elseif...else...end statement.
func (p *Parser) parseIf(from Pos) *IfStmt {
	p.next() // consume 'if'
	cond := p.parseExpr()
	p.expect(TokThen)
	body := p.parseBlock()
	stmt := &IfStmt{
		Clauses: []IfClause{{Cond: cond, Body: body, Sp: Span{From: spanOf(cond).From, To: body.Sp.To}}},
	}
	for p.tok.Type == TokElseif {
		eiFrom := p.tok.Sp.From
		p.next() // consume 'elseif'
		eic := p.parseExpr()
		p.expect(TokThen)
		eib := p.parseBlock()
		stmt.Clauses = append(stmt.Clauses, IfClause{
			Cond: eic,
			Body: eib,
			Sp:   Span{From: eiFrom, To: eib.Sp.To},
		})
	}
	if p.tok.Type == TokElse {
		p.next() // consume 'else'
		stmt.ElseBody = p.parseBlock()
	}
	end := p.expect(TokEnd)
	stmt.Sp = Span{From: from, To: end.Sp.To}
	return stmt
}

// parseFor parses either a numeric or generic for loop.
func (p *Parser) parseFor(from Pos) Stmt {
	p.next() // consume 'for'
	name := p.expectName()
	if p.tok.Type == int('=') {
		return p.parseForNum(from, name)
	}
	return p.parseForIn(from, name)
}

// parseForNum parses a numeric for loop: for name = start, limit [, step] do block end
func (p *Parser) parseForNum(from Pos, name *Ident) *ForNumStmt {
	p.expect(int('='))
	start := p.parseExpr()
	p.expect(int(','))
	limit := p.parseExpr()
	var step Expr
	if p.tok.Type == int(',') {
		p.next()
		step = p.parseExpr()
	}
	p.expect(TokDo)
	body := p.parseBlock()
	end := p.expect(TokEnd)
	return &ForNumStmt{
		Name:  name,
		Start: start,
		Limit: limit,
		Step:  step,
		Body:  body,
		Sp:    Span{From: from, To: end.Sp.To},
	}
}

// parseForIn parses a generic for loop: for namelist in exprlist do block end
func (p *Parser) parseForIn(from Pos, firstName *Ident) *ForInStmt {
	names := []*Ident{firstName}
	for p.tok.Type == int(',') {
		p.next()
		names = append(names, p.expectName())
	}
	p.expect(TokIn)
	values := p.parseExprList()
	p.expect(TokDo)
	body := p.parseBlock()
	end := p.expect(TokEnd)
	return &ForInStmt{
		Names:  names,
		Values: values,
		Body:   body,
		Sp:     Span{From: from, To: end.Sp.To},
	}
}

// parseFuncStmt parses a function statement: function funcname funcbody
func (p *Parser) parseFuncStmt(from Pos) *FuncStmt {
	p.next() // consume 'function'
	fname := p.parseFuncName()
	funcExpr := p.parseFuncBody(from)
	return &FuncStmt{
		IsLocal: false,
		Name:    fname,
		Func:    funcExpr,
		Sp:      Span{From: from, To: funcExpr.Sp.To},
	}
}

// parseFuncName parses a function name: Name {'.' Name} [':' Name]
func (p *Parser) parseFuncName() *FuncName {
	from := p.tok.Sp.From
	first := p.expectName()
	parts := []*Ident{first}
	for p.tok.Type == int('.') {
		p.next()
		parts = append(parts, p.expectName())
	}
	var method *Ident
	if p.tok.Type == int(':') {
		p.next()
		method = p.expectName()
	}
	to := first.Sp.To
	if method != nil {
		to = method.Sp.To
	} else if len(parts) > 0 {
		to = parts[len(parts)-1].Sp.To
	}
	return &FuncName{Parts: parts, Method: method, Sp: Span{From: from, To: to}}
}

// parseLocal parses a local statement: local function name | local namelist ['=' exprlist]
func (p *Parser) parseLocal(from Pos) Stmt {
	p.next() // consume 'local'
	if p.tok.Type == TokFunction {
		p.next() // consume 'function'
		name := p.expectName()
		funcExpr := p.parseFuncBody(from)
		return &FuncStmt{
			IsLocal:   true,
			LocalName: name,
			Func:      funcExpr,
			Sp:        Span{From: from, To: funcExpr.Sp.To},
		}
	}
	// local namelist ['=' exprlist]
	names := []*Ident{p.expectName()}
	for p.tok.Type == int(',') {
		p.next()
		names = append(names, p.expectName())
	}
	var values []Expr
	var endPos Pos
	if p.tok.Type == int('=') {
		p.next()
		values = p.parseExprList()
		if len(values) > 0 {
			endPos = spanOf(values[len(values)-1]).To
		}
	}
	if endPos == (Pos{}) && len(names) > 0 {
		endPos = names[len(names)-1].Sp.To
	}
	return &LocalStmt{
		Names:  names,
		Values: values,
		Sp:     Span{From: from, To: endPos},
	}
}

// parseGoto parses a goto statement.
func (p *Parser) parseGoto(from Pos) *GotoStmt {
	p.next() // consume 'goto'
	label := p.expectName()
	return &GotoStmt{Label: label, Sp: Span{From: from, To: label.Sp.To}}
}

// parseLabel parses a label statement: '::' Name '::'
func (p *Parser) parseLabel(from Pos) *LabelStmt {
	p.next() // consume '::'
	name := p.expectName()
	close := p.expect(TokDColon)
	return &LabelStmt{Name: name, Sp: Span{From: from, To: close.Sp.To}}
}

// parseReturn parses a return statement.
func (p *Parser) parseReturn() *ReturnStmt {
	from := p.tok.Sp.From
	p.next() // consume 'return'
	var values []Expr
	var endPos Pos
	if !p.isBlockEnd() && p.tok.Type != int(';') && p.tok.Type != TokEOF {
		values = p.parseExprList()
	}
	if len(values) > 0 {
		endPos = spanOf(values[len(values)-1]).To
	} else {
		endPos = p.tok.Sp.From
	}
	return &ReturnStmt{Values: values, Sp: Span{From: from, To: endPos}}
}

// parseExprStat parses an expression statement (assignment or function call).
func (p *Parser) parseExprStat(from Pos) Stmt {
	expr := p.parseSuffixedExpr()
	if expr == nil {
		// Error recovery: addError already called in parsePrimaryExpr
		return nil
	}

	// Check for assignment
	if p.tok.Type == int('=') || p.tok.Type == int(',') {
		targets := []Expr{expr}
		for p.tok.Type == int(',') {
			p.next()
			targets = append(targets, p.parseSuffixedExpr())
		}
		p.expect(int('='))
		values := p.parseExprList()
		endPos := from
		if len(values) > 0 {
			endPos = spanOf(values[len(values)-1]).To
		}
		return &AssignStmt{
			Targets: targets,
			Values:  values,
			Sp:      Span{From: from, To: endPos},
		}
	}

	// Must be a function call expression
	switch expr.(type) {
	case *CallExpr:
		sp := spanOf(expr)
		return &ExprStmt{X: expr, Sp: Span{From: from, To: sp.To}}
	default:
		p.addError("syntax error: expression is not a statement", spanOf(expr))
		sp := spanOf(expr)
		return &ExprStmt{X: expr, Sp: Span{From: from, To: sp.To}}
	}
}

// parseExprList parses a comma-separated list of expressions.
func (p *Parser) parseExprList() []Expr {
	exprs := []Expr{p.parseExpr()}
	for p.tok.Type == int(',') {
		p.next()
		exprs = append(exprs, p.parseExpr())
	}
	return exprs
}

// expectName expects and consumes a TokName token, returning an Ident.
func (p *Parser) expectName() *Ident {
	tok := p.expect(TokName)
	return &Ident{Name: tok.Val, Sp: tok.Sp}
}

// ---------------------------------------------------------------------------
// Expression parsing (operator precedence)
// ---------------------------------------------------------------------------

// opInfo holds left and right precedence for a binary operator.
// right < left means right-associative.
type opInfo struct {
	leftPrec  int
	rightPrec int
}

var binOps = map[int]opInfo{
	TokOr:     {1, 1},
	TokAnd:    {2, 2},
	int('<'):  {3, 3},
	int('>'):  {3, 3},
	TokLE:     {3, 3},
	TokGE:     {3, 3},
	TokNE:     {3, 3},
	TokEq:     {3, 3},
	TokConcat: {4, 3}, // right-assoc
	int('+'):  {5, 5},
	int('-'):  {5, 5},
	int('*'):  {6, 6},
	int('/'):  {6, 6},
	int('%'):  {6, 6},
	int('^'):  {8, 7}, // right-assoc
}

// parseExpr parses an expression.
func (p *Parser) parseExpr() Expr {
	return p.parseExprPrec(0)
}

// parseExprPrec parses a binary expression with a minimum left-precedence threshold.
func (p *Parser) parseExprPrec(minPrec int) Expr {
	left := p.parseUnaryExpr()
	for {
		op, ok := binOps[p.tok.Type]
		if !ok || op.leftPrec <= minPrec {
			break
		}
		opTok := p.tok
		p.next()
		right := p.parseExprPrec(op.rightPrec)
		sp := Span{From: spanOf(left).From, To: spanOf(right).To}
		left = &BinaryExpr{Op: opTok.Val, Left: left, Right: right, Sp: sp}
	}
	return left
}

// parseUnaryExpr parses a unary expression.
func (p *Parser) parseUnaryExpr() Expr {
	from := p.tok.Sp.From
	switch p.tok.Type {
	case TokNot:
		p.next()
		operand := p.parseExprPrec(7) // unary precedence is 7
		return &UnaryExpr{Op: "not", Operand: operand, Sp: Span{From: from, To: spanOf(operand).To}}
	case int('-'):
		p.next()
		operand := p.parseExprPrec(7)
		return &UnaryExpr{Op: "-", Operand: operand, Sp: Span{From: from, To: spanOf(operand).To}}
	case int('#'):
		p.next()
		operand := p.parseExprPrec(7)
		return &UnaryExpr{Op: "#", Operand: operand, Sp: Span{From: from, To: spanOf(operand).To}}
	}
	return p.parseSimpleExpr()
}

// parseSimpleExpr parses a simple (non-operator) expression.
func (p *Parser) parseSimpleExpr() Expr {
	from := p.tok.Sp.From
	_ = from
	switch p.tok.Type {
	case TokNumber:
		tok := p.tok
		p.next()
		return &NumberExpr{Value: tok.Num, Raw: tok.Val, Sp: tok.Sp}
	case TokString:
		tok := p.tok
		p.next()
		return &StringExpr{Value: tok.Val, Sp: tok.Sp}
	case TokNil:
		tok := p.tok
		p.next()
		return &NilExpr{Sp: tok.Sp}
	case TokTrue:
		tok := p.tok
		p.next()
		return &TrueExpr{Sp: tok.Sp}
	case TokFalse:
		tok := p.tok
		p.next()
		return &FalseExpr{Sp: tok.Sp}
	case TokDots:
		tok := p.tok
		p.next()
		return &VarArgExpr{Sp: tok.Sp}
	case TokFunction:
		funcFrom := p.tok.Sp.From
		p.next()
		fe := p.parseFuncBody(funcFrom)
		return fe
	case int('{'):
		return p.parseTableConstructor()
	default:
		return p.parseSuffixedExpr()
	}
}

// parseSuffixedExpr parses a primary expression followed by suffixes (field, index, call).
func (p *Parser) parseSuffixedExpr() Expr {
	expr := p.parsePrimaryExpr()
	if expr == nil {
		return nil
	}
	for {
		switch p.tok.Type {
		case int('.'):
			p.next()
			field := p.expectName()
			expr = &FieldExpr{
				Table: expr,
				Field: field,
				Sp:    Span{From: spanOf(expr).From, To: field.Sp.To},
			}
		case int('['):
			p.next()
			key := p.parseExpr()
			close := p.expect(int(']'))
			expr = &IndexExpr{
				Table: expr,
				Key:   key,
				Sp:    Span{From: spanOf(expr).From, To: close.Sp.To},
			}
		case int(':'):
			p.next()
			method := p.expectName()
			openParen, args, closeSp := p.parseCallArgs()
			expr = &CallExpr{
				Func:      expr,
				Method:    method,
				Args:      args,
				OpenParen: openParen,
				Sp:        Span{From: spanOf(expr).From, To: closeSp.To},
			}
		case int('('), TokString, int('{'):
			openParen, args, closeSp := p.parseCallArgs()
			expr = &CallExpr{
				Func:      expr,
				Args:      args,
				OpenParen: openParen,
				Sp:        Span{From: spanOf(expr).From, To: closeSp.To},
			}
		default:
			return expr
		}
	}
}

// parsePrimaryExpr parses a Name or parenthesized expression.
func (p *Parser) parsePrimaryExpr() Expr {
	switch p.tok.Type {
	case TokName:
		tok := p.tok
		p.next()
		return &NameExpr{Name: tok.Val, Sp: tok.Sp}
	case int('('):
		from := p.tok.Sp.From
		p.next()
		inner := p.parseExpr()
		close := p.expect(int(')'))
		return &ParenExpr{Inner: inner, Sp: Span{From: from, To: close.Sp.To}}
	default:
		p.addError(fmt.Sprintf("unexpected symbol %s", TokenName(p.tok.Type)), p.tok.Sp)
		p.sync()
		return nil
	}
}

// parseCallArgs parses function call arguments: '(' [exprlist] ')' | string | table
// Returns openParen position, argument list, and close span.
func (p *Parser) parseCallArgs() (Pos, []Expr, Span) {
	switch p.tok.Type {
	case int('('):
		openParen := p.tok.Sp.From
		p.next() // consume '('
		var args []Expr
		if p.tok.Type != int(')') {
			args = p.parseExprList()
		}
		close := p.expect(int(')'))
		return openParen, args, close.Sp
	case TokString:
		tok := p.tok
		p.next()
		arg := &StringExpr{Value: tok.Val, Sp: tok.Sp}
		return tok.Sp.From, []Expr{arg}, tok.Sp
	case int('{'):
		tbl := p.parseTableConstructor()
		sp := spanOf(tbl)
		return sp.From, []Expr{tbl}, sp
	default:
		p.addError("expected function arguments", p.tok.Sp)
		return p.tok.Sp.From, nil, p.tok.Sp
	}
}

// parseFuncBody parses a function body: '(' [parlist] ')' block 'end'
func (p *Parser) parseFuncBody(from Pos) *FuncExpr {
	p.expect(int('('))
	params, hasVarArg := p.parseParList()
	p.expect(int(')'))
	body := p.parseBlock()
	end := p.expect(TokEnd)
	return &FuncExpr{
		Params:    params,
		HasVarArg: hasVarArg,
		Body:      body,
		Sp:        Span{From: from, To: end.Sp.To},
	}
}

// parseParList parses a function parameter list.
func (p *Parser) parseParList() ([]*Ident, bool) {
	var params []*Ident
	hasVarArg := false

	if p.tok.Type == int(')') {
		return params, hasVarArg
	}

	if p.tok.Type == TokDots {
		p.next()
		return params, true
	}

	params = append(params, p.expectName())
	for p.tok.Type == int(',') {
		p.next()
		if p.tok.Type == TokDots {
			p.next()
			hasVarArg = true
			break
		}
		params = append(params, p.expectName())
	}
	return params, hasVarArg
}

// parseTableConstructor parses a table constructor: '{' [fieldlist] '}'
func (p *Parser) parseTableConstructor() *TableExpr {
	from := p.tok.Sp.From
	p.expect(int('{'))
	var fields []TableField
	for p.tok.Type != int('}') && p.tok.Type != TokEOF {
		field := p.parseTableField()
		fields = append(fields, field)
		if p.tok.Type == int(',') || p.tok.Type == int(';') {
			p.next()
		} else {
			break
		}
	}
	close := p.expect(int('}'))
	return &TableExpr{Fields: fields, Sp: Span{From: from, To: close.Sp.To}}
}

// parseTableField parses a single table field.
func (p *Parser) parseTableField() TableField {
	from := p.tok.Sp.From

	// [expr] = expr
	if p.tok.Type == int('[') {
		p.next()
		key := p.parseExpr()
		p.expect(int(']'))
		p.expect(int('='))
		val := p.parseExpr()
		return TableField{Key: key, Value: val, Sp: Span{From: from, To: spanOf(val).To}}
	}

	// name = expr  (only if followed by '=')
	if p.tok.Type == TokName {
		nameTok := p.tok
		// Look ahead: is next token '='?
		nextTok := p.s.Peek()
		if nextTok.Type == int('=') {
			p.next() // consume name
			p.next() // consume '='
			ident := &Ident{Name: nameTok.Val, Sp: nameTok.Sp}
			val := p.parseExpr()
			return TableField{Name: ident, Value: val, Sp: Span{From: from, To: spanOf(val).To}}
		}
	}

	// expr (list-style)
	val := p.parseExpr()
	return TableField{Value: val, Sp: Span{From: from, To: spanOf(val).To}}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// spanOf returns the Span of an expression node.
func spanOf(e Expr) Span {
	if e == nil {
		return Span{}
	}
	switch v := e.(type) {
	case *NilExpr:
		return v.Sp
	case *TrueExpr:
		return v.Sp
	case *FalseExpr:
		return v.Sp
	case *VarArgExpr:
		return v.Sp
	case *NumberExpr:
		return v.Sp
	case *StringExpr:
		return v.Sp
	case *NameExpr:
		return v.Sp
	case *IndexExpr:
		return v.Sp
	case *FieldExpr:
		return v.Sp
	case *UnaryExpr:
		return v.Sp
	case *BinaryExpr:
		return v.Sp
	case *CallExpr:
		return v.Sp
	case *FuncExpr:
		return v.Sp
	case *TableExpr:
		return v.Sp
	case *ParenExpr:
		return v.Sp
	default:
		return Span{}
	}
}

// getStmtSpan returns the Span of a statement node.
func getStmtSpan(s Stmt) Span {
	if s == nil {
		return Span{}
	}
	return s.stmtSpan()
}

// stmtSpan implementations satisfy the Stmt interface requirement.
func (s *AssignStmt) stmtSpan() Span { return s.Sp }
func (s *LocalStmt) stmtSpan() Span  { return s.Sp }
func (s *DoStmt) stmtSpan() Span     { return s.Sp }
func (s *WhileStmt) stmtSpan() Span  { return s.Sp }
func (s *RepeatStmt) stmtSpan() Span { return s.Sp }
func (s *IfStmt) stmtSpan() Span     { return s.Sp }
func (s *ForNumStmt) stmtSpan() Span { return s.Sp }
func (s *ForInStmt) stmtSpan() Span  { return s.Sp }
func (s *FuncStmt) stmtSpan() Span   { return s.Sp }
func (s *ReturnStmt) stmtSpan() Span { return s.Sp }
func (s *BreakStmt) stmtSpan() Span  { return s.Sp }
func (s *GotoStmt) stmtSpan() Span   { return s.Sp }
func (s *LabelStmt) stmtSpan() Span  { return s.Sp }
func (s *ExprStmt) stmtSpan() Span   { return s.Sp }
