package lsp

import "strings"

// ---------------------------------------------------------------------------
// Symbol/kind display helpers
// ---------------------------------------------------------------------------

func kindString(k SymbolKind) string {
	switch k {
	case SkLocal:
		return "local"
	case SkParam:
		return "parameter"
	case SkGlobal:
		return "global"
	case SkLabel:
		return "label"
	}
	return ""
}

func buildFuncDetail(name string, sig *FuncSig) string {
	var sb strings.Builder
	sb.WriteString("function ")
	sb.WriteString(name)
	sb.WriteByte('(')
	for i, p := range sig.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(p)
	}
	if sig.HasVarArg {
		if len(sig.Params) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("...")
	}
	sb.WriteByte(')')
	return sb.String()
}

// ---------------------------------------------------------------------------
// AST walk helpers for signature help
// ---------------------------------------------------------------------------

// findCallAtOffset searches the AST for the innermost CallExpr enclosing
// the given offset. Returns the call and the argument index at offset.
func findCallAtOffset(block *Block, offset int) (*CallExpr, int) {
	var best *CallExpr
	var bestArgIdx int

	var walkExpr func(e Expr)
	var walkStmt func(st Stmt)
	var walkBlock func(b *Block)

	walkExpr = func(e Expr) {
		if e == nil {
			return
		}
		switch ex := e.(type) {
		case *CallExpr:
			sp := ex.Sp
			if offset >= sp.From.Offset && offset <= sp.To.Offset {
				best = ex
				bestArgIdx = argIndexAt(ex, offset)
			}
			walkExpr(ex.Func)
			for _, arg := range ex.Args {
				walkExpr(arg)
			}
		case *BinaryExpr:
			walkExpr(ex.Left)
			walkExpr(ex.Right)
		case *UnaryExpr:
			walkExpr(ex.Operand)
		case *FieldExpr:
			walkExpr(ex.Table)
		case *IndexExpr:
			walkExpr(ex.Table)
			walkExpr(ex.Key)
		case *TableExpr:
			for _, f := range ex.Fields {
				if f.Key != nil {
					walkExpr(f.Key)
				}
				walkExpr(f.Value)
			}
		case *FuncExpr:
			walkBlock(ex.Body)
		case *ParenExpr:
			walkExpr(ex.Inner)
		}
	}

	walkStmt = func(st Stmt) {
		if st == nil {
			return
		}
		switch s := st.(type) {
		case *AssignStmt:
			for _, t := range s.Targets {
				walkExpr(t)
			}
			for _, v := range s.Values {
				walkExpr(v)
			}
		case *LocalStmt:
			for _, v := range s.Values {
				walkExpr(v)
			}
		case *DoStmt:
			walkBlock(s.Body)
		case *WhileStmt:
			walkExpr(s.Cond)
			walkBlock(s.Body)
		case *RepeatStmt:
			walkBlock(s.Body)
			walkExpr(s.Cond)
		case *IfStmt:
			for _, cl := range s.Clauses {
				walkExpr(cl.Cond)
				walkBlock(cl.Body)
			}
			if s.ElseBody != nil {
				walkBlock(s.ElseBody)
			}
		case *ForNumStmt:
			walkExpr(s.Start)
			walkExpr(s.Limit)
			if s.Step != nil {
				walkExpr(s.Step)
			}
			walkBlock(s.Body)
		case *ForInStmt:
			for _, v := range s.Values {
				walkExpr(v)
			}
			walkBlock(s.Body)
		case *FuncStmt:
			walkBlock(s.Func.Body)
		case *ReturnStmt:
			for _, v := range s.Values {
				walkExpr(v)
			}
		case *ExprStmt:
			walkExpr(s.X)
		}
	}

	walkBlock = func(b *Block) {
		if b == nil {
			return
		}
		for _, st := range b.Stmts {
			walkStmt(st)
		}
		if b.Ret != nil {
			walkStmt(b.Ret)
		}
	}

	walkBlock(block)
	return best, bestArgIdx
}

// argIndexAt returns which argument slot the offset falls into.
func argIndexAt(call *CallExpr, offset int) int {
	if len(call.Args) == 0 {
		return 0
	}
	for i, arg := range call.Args {
		if arg == nil {
			continue
		}
		if offset <= arg.Span().To.Offset {
			return i
		}
	}
	return len(call.Args) - 1
}

// callFuncName extracts a simple name from the Func field of a call.
func callFuncName(call *CallExpr) string {
	switch f := call.Func.(type) {
	case *NameExpr:
		return f.Name
	case *FieldExpr:
		if base, ok := f.Table.(*NameExpr); ok {
			return base.Name + "." + f.Field.Name
		}
	}
	return ""
}

// stdlibSig returns a pre-built SignatureInformation for well-known Lua stdlib functions.
func stdlibSig(name string) *SignatureInformation {
	type sigDef struct {
		label  string
		params []string
	}
	sigs := map[string]sigDef{
		"print":        {"print(...)", []string{"..."}},
		"type":         {"type(v)", []string{"v"}},
		"tostring":     {"tostring(v)", []string{"v"}},
		"tonumber":     {"tonumber(e [, base])", []string{"e", "base"}},
		"pairs":        {"pairs(t)", []string{"t"}},
		"ipairs":       {"ipairs(t)", []string{"t"}},
		"next":         {"next(table [, index])", []string{"table", "index"}},
		"select":       {"select(index, ...)", []string{"index", "..."}},
		"unpack":       {"unpack(list [, i [, j]])", []string{"list", "i", "j"}},
		"error":        {"error(message [, level])", []string{"message", "level"}},
		"assert":       {"assert(v [, message])", []string{"v", "message"}},
		"pcall":        {"pcall(f, ...)", []string{"f", "..."}},
		"xpcall":       {"xpcall(f, msgh, ...)", []string{"f", "msgh", "..."}},
		"require":      {"require(modname)", []string{"modname"}},
		"setmetatable": {"setmetatable(table, metatable)", []string{"table", "metatable"}},
		"getmetatable": {"getmetatable(object)", []string{"object"}},
		"rawget":       {"rawget(table, index)", []string{"table", "index"}},
		"rawset":       {"rawset(table, index, value)", []string{"table", "index", "value"}},
		"rawequal":     {"rawequal(v1, v2)", []string{"v1", "v2"}},
		"rawlen":       {"rawlen(v)", []string{"v"}},
	}
	if def, ok := sigs[name]; ok {
		params := make([]ParameterInformation, len(def.params))
		for i, p := range def.params {
			params[i] = ParameterInformation{Label: p}
		}
		return &SignatureInformation{Label: def.label, Parameters: params}
	}
	return nil
}
