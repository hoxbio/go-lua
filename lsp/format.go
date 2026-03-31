package lsp

import (
	"fmt"
	"strings"
)

// Format pretty-prints a parsed Lua AST block.
// tabSize controls the number of spaces per indent level; if useSpaces is false
// a literal tab character is used instead regardless of tabSize.
func Format(block *Block, tabSize int, useSpaces bool) string {
	f := &formatter{
		tabSize:   tabSize,
		useSpaces: useSpaces,
	}
	f.writeBlock(block, false)
	return f.sb.String()
}

// formatter holds the state used while printing the AST.
type formatter struct {
	sb        strings.Builder
	depth     int
	tabSize   int
	useSpaces bool
}

// indent writes the current indentation prefix.
func (f *formatter) indent() {
	if f.useSpaces {
		f.sb.WriteString(strings.Repeat(" ", f.depth*f.tabSize))
	} else {
		f.sb.WriteString(strings.Repeat("\t", f.depth))
	}
}

func (f *formatter) write(s string) { f.sb.WriteString(s) }

func (f *formatter) writeLine(s string) {
	f.indent()
	f.sb.WriteString(s)
	f.sb.WriteByte('\n')
}

func (f *formatter) newline() { f.sb.WriteByte('\n') }

// writeBlock writes the statements of a block at the current indent depth.
// If inline is true the surrounding do/end keywords are already written by
// the caller and no extra indent is added.
func (f *formatter) writeBlock(block *Block, inline bool) {
	if !inline {
		f.depth++
	}
	for _, stmt := range block.Stmts {
		f.writeStmt(stmt)
	}
	if block.Ret != nil {
		f.writeStmt(block.Ret)
	}
	if !inline {
		f.depth--
	}
}

// writeStmt writes a single statement followed by a newline.
func (f *formatter) writeStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *AssignStmt:
		f.indent()
		f.writeExprList(s.Targets)
		f.write(" = ")
		f.writeExprList(s.Values)
		f.newline()

	case *LocalStmt:
		f.indent()
		f.write("local ")
		for i, ident := range s.Names {
			if i > 0 {
				f.write(", ")
			}
			f.write(ident.Name)
		}
		if len(s.Values) > 0 {
			f.write(" = ")
			f.writeExprList(s.Values)
		}
		f.newline()

	case *DoStmt:
		f.writeLine("do")
		f.writeBlock(s.Body, false)
		f.writeLine("end")

	case *WhileStmt:
		f.indent()
		f.write("while ")
		f.writeExpr(s.Cond)
		f.write(" do")
		f.newline()
		f.writeBlock(s.Body, false)
		f.writeLine("end")

	case *RepeatStmt:
		f.writeLine("repeat")
		f.writeBlock(s.Body, false)
		f.indent()
		f.write("until ")
		f.writeExpr(s.Cond)
		f.newline()

	case *IfStmt:
		for i, clause := range s.Clauses {
			f.indent()
			if i == 0 {
				f.write("if ")
			} else {
				f.write("elseif ")
			}
			f.writeExpr(clause.Cond)
			f.write(" then")
			f.newline()
			f.writeBlock(clause.Body, false)
		}
		if s.ElseBody != nil {
			f.writeLine("else")
			f.writeBlock(s.ElseBody, false)
		}
		f.writeLine("end")

	case *ForNumStmt:
		f.indent()
		f.write("for ")
		f.write(s.Name.Name)
		f.write(" = ")
		f.writeExpr(s.Start)
		f.write(", ")
		f.writeExpr(s.Limit)
		if s.Step != nil {
			f.write(", ")
			f.writeExpr(s.Step)
		}
		f.write(" do")
		f.newline()
		f.writeBlock(s.Body, false)
		f.writeLine("end")

	case *ForInStmt:
		f.indent()
		f.write("for ")
		for i, ident := range s.Names {
			if i > 0 {
				f.write(", ")
			}
			f.write(ident.Name)
		}
		f.write(" in ")
		f.writeExprList(s.Values)
		f.write(" do")
		f.newline()
		f.writeBlock(s.Body, false)
		f.writeLine("end")

	case *FuncStmt:
		f.indent()
		if s.IsLocal {
			f.write("local function ")
			f.write(s.LocalName.Name)
		} else {
			f.write("function ")
			f.writeFuncName(s.Name)
		}
		f.writeFuncSignature(s.Func)
		f.newline()
		f.writeBlock(s.Func.Body, false)
		f.writeLine("end")

	case *ReturnStmt:
		f.indent()
		f.write("return")
		if len(s.Values) > 0 {
			f.write(" ")
			f.writeExprList(s.Values)
		}
		f.newline()

	case *BreakStmt:
		f.writeLine("break")

	case *GotoStmt:
		f.indent()
		f.write("goto ")
		f.write(s.Label.Name)
		f.newline()

	case *LabelStmt:
		f.indent()
		f.write("::")
		f.write(s.Name.Name)
		f.write("::")
		f.newline()

	case *ExprStmt:
		f.indent()
		f.writeExpr(s.X)
		f.newline()
	}
}

// writeFuncName writes a FuncName node.
func (f *formatter) writeFuncName(fn *FuncName) {
	for i, part := range fn.Parts {
		if i > 0 {
			f.write(".")
		}
		f.write(part.Name)
	}
	if fn.Method != nil {
		f.write(":")
		f.write(fn.Method.Name)
	}
}

// writeFuncSignature writes "(params)" for a FuncExpr (no body, no 'function' keyword).
func (f *formatter) writeFuncSignature(fe *FuncExpr) {
	f.write("(")
	for i, param := range fe.Params {
		if i > 0 {
			f.write(", ")
		}
		f.write(param.Name)
	}
	if fe.HasVarArg {
		if len(fe.Params) > 0 {
			f.write(", ")
		}
		f.write("...")
	}
	f.write(")")
}

// writeExprList writes a comma-separated list of expressions.
func (f *formatter) writeExprList(exprs []Expr) {
	for i, e := range exprs {
		if i > 0 {
			f.write(", ")
		}
		f.writeExpr(e)
	}
}

// writeExpr writes a single expression.
func (f *formatter) writeExpr(expr Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *NilExpr:
		f.write("nil")
	case *TrueExpr:
		f.write("true")
	case *FalseExpr:
		f.write("false")
	case *VarArgExpr:
		f.write("...")
	case *NumberExpr:
		if e.Raw != "" {
			f.write(e.Raw)
		} else {
			f.write(formatFloat(e.Value))
		}
	case *StringExpr:
		f.write(quoteString(e.Value))
	case *NameExpr:
		f.write(e.Name)
	case *ParenExpr:
		f.write("(")
		f.writeExpr(e.Inner)
		f.write(")")
	case *UnaryExpr:
		f.write(e.Op)
		// 'not' needs a space; '-' and '#' do not require one but it aids readability
		if e.Op == "not" {
			f.write(" ")
		}
		// Wrap in parens if operand is a binary expr to preserve precedence visually
		if _, ok := e.Operand.(*BinaryExpr); ok {
			f.write("(")
			f.writeExpr(e.Operand)
			f.write(")")
		} else {
			f.writeExpr(e.Operand)
		}
	case *BinaryExpr:
		f.writeBinaryExpr(e)
	case *FieldExpr:
		f.writeExpr(e.Table)
		f.write(".")
		f.write(e.Field.Name)
	case *IndexExpr:
		f.writeExpr(e.Table)
		f.write("[")
		f.writeExpr(e.Key)
		f.write("]")
	case *CallExpr:
		f.writeCallExpr(e)
	case *FuncExpr:
		f.write("function")
		f.writeFuncSignature(e)
		f.newline()
		f.writeBlock(e.Body, false)
		f.indent()
		f.write("end")
	case *TableExpr:
		f.writeTableExpr(e)
	}
}

// writeBinaryExpr writes a binary expression, adding parens when needed.
func (f *formatter) writeBinaryExpr(e *BinaryExpr) {
	f.writeExprParens(e.Left, e.Op, true)
	f.write(" ")
	f.write(e.Op)
	f.write(" ")
	f.writeExprParens(e.Right, e.Op, false)
}

// writeExprParens writes expr, wrapping in parens if it's a lower-precedence binary op.
func (f *formatter) writeExprParens(expr Expr, outerOp string, isLeft bool) {
	child, isBin := expr.(*BinaryExpr)
	if isBin {
		outerL, _ := opPrec(outerOp)
		childL, _ := opPrec(child.Op)
		if childL < outerL {
			f.write("(")
			f.writeExpr(expr)
			f.write(")")
			return
		}
	}
	f.writeExpr(expr)
}

// opPrec returns the left and right precedence for a binary operator.
func opPrec(op string) (int, int) {
	switch op {
	case "or":
		return 1, 1
	case "and":
		return 2, 2
	case "<", ">", "<=", ">=", "==", "~=":
		return 3, 3
	case "..":
		return 4, 3
	case "+", "-":
		return 5, 5
	case "*", "/", "%":
		return 6, 6
	case "^":
		return 8, 7
	}
	return 0, 0
}

// writeCallExpr writes a function call expression.
func (f *formatter) writeCallExpr(e *CallExpr) {
	f.writeExpr(e.Func)
	if e.Method != nil {
		f.write(":")
		f.write(e.Method.Name)
	}
	f.write("(")
	f.writeExprList(e.Args)
	f.write(")")
}

// writeTableExpr writes a table constructor.
// Single-field tables are written on one line; multi-field tables use one
// field per line.
func (f *formatter) writeTableExpr(e *TableExpr) {
	if len(e.Fields) == 0 {
		f.write("{}")
		return
	}
	if len(e.Fields) == 1 {
		f.write("{ ")
		f.writeTableField(e.Fields[0])
		f.write(" }")
		return
	}
	f.write("{")
	f.newline()
	f.depth++
	for _, field := range e.Fields {
		f.indent()
		f.writeTableField(field)
		f.write(",")
		f.newline()
	}
	f.depth--
	f.indent()
	f.write("}")
}

// writeTableField writes a single table field.
func (f *formatter) writeTableField(field TableField) {
	if field.Key != nil {
		f.write("[")
		f.writeExpr(field.Key)
		f.write("] = ")
		f.writeExpr(field.Value)
	} else if field.Name != nil {
		f.write(field.Name.Name)
		f.write(" = ")
		f.writeExpr(field.Value)
	} else {
		f.writeExpr(field.Value)
	}
}

// quoteString returns a Lua double-quoted string literal for s.
func quoteString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case 0:
			sb.WriteString(`\0`)
		default:
			if c < 32 {
				sb.WriteString(fmt.Sprintf(`\%d`, c))
			} else {
				sb.WriteByte(c)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// formatFloat formats a float64 as a Lua number literal.
func formatFloat(v float64) string {
	if v == float64(int64(v)) && v >= -1e15 && v <= 1e15 {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}
