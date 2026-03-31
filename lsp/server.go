package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Document store
// ---------------------------------------------------------------------------

// Document holds everything the server knows about an open text document.
type Document struct {
	URI      string
	Version  int
	Text     string
	Block    *Block
	Analysis *Analysis
	Errors   []SyntaxError
}

// Server is the LSP server.
type Server struct {
	docs map[string]*Document // URI -> Document
	in   *bufio.Reader
	out  *bufio.Writer
	mu   sync.Mutex
}

// NewServer creates an LSP server that reads requests from in and writes
// responses to out using standard JSON-RPC framing.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		docs: make(map[string]*Document),
		in:   bufio.NewReader(in),
		out:  bufio.NewWriter(out),
	}
}

// ---------------------------------------------------------------------------
// JSON-RPC I/O
// ---------------------------------------------------------------------------

// readMessage reads one JSON-RPC message from stdin.
// It expects "Content-Length: N\r\n\r\n" framing.
func (s *Server) readMessage() (*RequestMessage, error) {
	var contentLength int
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			_, err = fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := s.in.Read(body); err != nil {
		return nil, err
	}
	var msg RequestMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// writeMessage sends raw bytes to the client with Content-Length framing.
func (s *Server) writeMessage(data []byte) {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.out.WriteString(header)
	s.out.Write(data)
	s.out.Flush()
}

// send sends a successful JSON-RPC response.
func (s *Server) send(id interface{}, result interface{}) {
	idRaw, _ := json.Marshal(id)
	resp := ResponseMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(idRaw),
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	s.writeMessage(data)
}

// sendError sends a JSON-RPC error response.
func (s *Server) sendError(id interface{}, code int, msg string) {
	idRaw, _ := json.Marshal(id)
	resp := ResponseMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(idRaw),
		Error:   &ResponseError{Code: code, Message: msg},
	}
	data, _ := json.Marshal(resp)
	s.writeMessage(data)
}

// notify sends a JSON-RPC notification (no ID).
func (s *Server) notify(method string, params interface{}) {
	paramsRaw, _ := json.Marshal(params)
	notif := NotificationMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  json.RawMessage(paramsRaw),
	}
	data, _ := json.Marshal(notif)
	s.writeMessage(data)
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

// Run starts the server's main message-dispatch loop.
func (s *Server) Run() {
	for {
		msg, err := s.readMessage()
		if err != nil {
			return
		}
		s.dispatch(msg)
	}
}

// dispatch routes a request to the appropriate handler.
func (s *Server) dispatch(msg *RequestMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id interface{}
	if msg.ID != nil {
		_ = json.Unmarshal(msg.ID, &id)
	}

	switch msg.Method {
	case "initialize":
		s.handleInitialize(id, msg.Params)

	case "initialized":
		// no-op notification

	case "shutdown":
		s.send(id, nil)

	case "exit":
		os.Exit(0)

	case "textDocument/didOpen":
		var p DidOpenTextDocumentParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		s.openDocument(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)

	case "textDocument/didChange":
		var p DidChangeTextDocumentParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		if len(p.ContentChanges) > 0 {
			text := p.ContentChanges[len(p.ContentChanges)-1].Text
			s.openDocument(p.TextDocument.URI, p.TextDocument.Version, text)
		}

	case "textDocument/didClose":
		var p DidCloseTextDocumentParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return
		}
		delete(s.docs, p.TextDocument.URI)

	case "textDocument/completion":
		var p CompletionParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleCompletion(doc, p.Position))

	case "textDocument/hover":
		var p HoverParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleHover(doc, p.Position))

	case "textDocument/signatureHelp":
		var p SignatureHelpParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleSignatureHelp(doc, p.Position))

	case "textDocument/definition":
		var p DefinitionParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleDefinition(doc, p.Position))

	case "textDocument/references":
		var p ReferenceParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleReferences(doc, p.Position, p.Context.IncludeDeclaration))

	case "textDocument/rename":
		var p RenameParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleRename(doc, p.Position, p.NewName))

	case "textDocument/formatting":
		var p DocumentFormattingParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleFormatting(doc, p.Options))

	default:
		if id != nil {
			s.sendError(id, ErrCodeMethodNotFound, fmt.Sprintf("method not found: %s", msg.Method))
		}
	}
}

// ---------------------------------------------------------------------------
// Initialize
// ---------------------------------------------------------------------------

func (s *Server) handleInitialize(id interface{}, params json.RawMessage) {
	serverName := "lua-lsp"
	serverVersion := "0.1.0"
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full sync
			},
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{".", ":"},
			},
			HoverProvider: true,
			SignatureHelpProvider: &SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ","},
				RetriggerCharacters: []string{","},
			},
			DefinitionProvider:         true,
			ReferencesProvider:         true,
			RenameProvider:             true,
			DocumentFormattingProvider: true,
		},
		ServerInfo: &ServerInfo{
			Name:    serverName,
			Version: &serverVersion,
		},
	}
	s.send(id, result)
}

// ---------------------------------------------------------------------------
// Document lifecycle
// ---------------------------------------------------------------------------

// openDocument parses and analyzes a document, storing it and publishing diagnostics.
func (s *Server) openDocument(uri string, version int, text string) {
	p := NewParser(text)
	block, errs := p.Parse()
	var analysis *Analysis
	if block != nil {
		analysis = Analyze(block)
	}
	doc := &Document{
		URI:      uri,
		Version:  version,
		Text:     text,
		Block:    block,
		Analysis: analysis,
		Errors:   errs, // []SyntaxError
	}
	s.docs[uri] = doc
	s.publishDiagnostics(doc)
}

// publishDiagnostics sends textDocument/publishDiagnostics.
func (s *Server) publishDiagnostics(doc *Document) {
	diags := make([]Diagnostic, 0, len(doc.Errors))
	for _, pe := range doc.Errors {
		diags = append(diags, Diagnostic{
			Range:    pe.Sp.ToLSP(),
			Severity: SeverityError,
			Source:   "lua-lsp",
			Message:  pe.Msg,
		})
	}
	s.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diags,
	})
}

// ---------------------------------------------------------------------------
// posToOffset: LSP Position -> byte offset
// ---------------------------------------------------------------------------

// posToOffset converts an LSP (line, character) position to a byte offset.
func posToOffset(text string, pos Position) int {
	line := 0
	offset := 0
	for offset < len(text) {
		if line == pos.Line {
			col := 0
			for col < pos.Character && offset < len(text) && text[offset] != '\n' {
				offset++
				col++
			}
			return offset
		}
		if text[offset] == '\n' {
			line++
		}
		offset++
	}
	return offset
}

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
}

type stdEntry struct {
	name   string
	detail string
}

var luaStdGlobals = []stdEntry{
	{"print", "function print(...)"},
	{"type", "function type(v)"},
	{"tostring", "function tostring(v)"},
	{"tonumber", "function tonumber(e [, base])"},
	{"pairs", "function pairs(t)"},
	{"ipairs", "function ipairs(t)"},
	{"next", "function next(table [, index])"},
	{"select", "function select(index, ...)"},
	{"unpack", "function unpack(list [, i [, j]])"},
	{"error", "function error(message [, level])"},
	{"assert", "function assert(v [, message])"},
	{"pcall", "function pcall(f, ...)"},
	{"xpcall", "function xpcall(f, msgh, ...)"},
	{"require", "function require(modname)"},
	{"setmetatable", "function setmetatable(table, metatable)"},
	{"getmetatable", "function getmetatable(object)"},
	{"rawget", "function rawget(table, index)"},
	{"rawset", "function rawset(table, index, value)"},
	{"rawequal", "function rawequal(v1, v2)"},
	{"rawlen", "function rawlen(v)"},
	{"collectgarbage", "function collectgarbage([opt [, arg]])"},
	{"dofile", "function dofile([filename])"},
	{"load", "function load(chunk [, chunkname [, mode [, env]]])"},
	{"loadfile", "function loadfile([filename [, mode [, env]]])"},
	{"math", "table math"},
	{"string", "table string"},
	{"table", "table table"},
	{"io", "table io"},
	{"os", "table os"},
	{"package", "table package"},
	{"coroutine", "table coroutine"},
	{"bit32", "table bit32"},
	{"debug", "table debug"},
	{"_G", "global environment table"},
	{"_VERSION", "Lua version string"},
}

func (s *Server) handleCompletion(doc *Document, pos Position) *CompletionList {
	if doc.Analysis == nil {
		return &CompletionList{Items: []CompletionItem{}}
	}

	offset := posToOffset(doc.Text, pos)
	scope := doc.Analysis.ScopeAt(offset)

	seen := make(map[string]bool)
	var items []CompletionItem

	// Walk the scope chain collecting all visible symbols.
	for sc := scope; sc != nil; sc = sc.Parent {
		for name, sym := range sc.Symbols {
			if seen[name] {
				continue
			}
			seen[name] = true
			kind := CIKVariable
			detail := kindString(sym.Kind)
			if sym.FuncSig != nil {
				kind = CIKFunction
				detail = buildFuncDetail(name, sym.FuncSig)
			}
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   kind,
				Detail: detail,
			})
		}
	}

	// Lua keywords.
	for _, kw := range luaKeywords {
		if !seen[kw] {
			seen[kw] = true
			items = append(items, CompletionItem{
				Label:    kw,
				Kind:     CIKKeyword,
				SortText: "~" + kw,
			})
		}
	}

	// Standard library globals.
	for _, g := range luaStdGlobals {
		if seen[g.name] {
			continue
		}
		seen[g.name] = true
		kind := CIKFunction
		if strings.HasPrefix(g.detail, "table ") || g.detail == "global environment table" || g.detail == "Lua version string" {
			kind = CIKVariable
		}
		items = append(items, CompletionItem{
			Label:    g.name,
			Kind:     kind,
			Detail:   g.detail,
			SortText: "~" + g.name,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		si := items[i].SortText
		if si == "" {
			si = items[i].Label
		}
		sj := items[j].SortText
		if sj == "" {
			sj = items[j].Label
		}
		return si < sj
	})

	return &CompletionList{
		IsIncomplete: false,
		Items:        items,
	}
}

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
// Hover
// ---------------------------------------------------------------------------

func (s *Server) handleHover(doc *Document, pos Position) *Hover {
	if doc.Analysis == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)
	sym := doc.Analysis.SymbolAt(offset)
	if sym == nil {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("```lua\n")
	if sym.FuncSig != nil {
		sb.WriteString(buildFuncDetail(sym.Name, sym.FuncSig))
	} else {
		sb.WriteString(kindString(sym.Kind))
		sb.WriteString(" ")
		sb.WriteString(sym.Name)
	}
	sb.WriteString("\n```")

	r := sym.Def.ToLSP()
	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: sb.String(),
		},
		Range: &r,
	}
}

// ---------------------------------------------------------------------------
// Signature Help
// ---------------------------------------------------------------------------

func (s *Server) handleSignatureHelp(doc *Document, pos Position) *SignatureHelp {
	if doc.Analysis == nil || doc.Block == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)

	call, argIdx := findCallAtOffset(doc.Block, offset)
	if call == nil {
		return nil
	}

	funcName := callFuncName(call)
	if funcName == "" {
		return nil
	}

	scope := doc.Analysis.ScopeAt(offset)
	sym := scope.Lookup(funcName)

	var sigInfo SignatureInformation
	var paramCount int

	if sym != nil && sym.FuncSig != nil {
		label := buildFuncDetail(funcName, sym.FuncSig)
		params := make([]ParameterInformation, len(sym.FuncSig.Params))
		for i, p := range sym.FuncSig.Params {
			params[i] = ParameterInformation{Label: p}
		}
		if sym.FuncSig.HasVarArg {
			params = append(params, ParameterInformation{Label: "..."})
		}
		paramCount = len(params)
		sigInfo = SignatureInformation{Label: label, Parameters: params}
	} else {
		stdlib := stdlibSig(funcName)
		if stdlib == nil {
			return nil
		}
		sigInfo = *stdlib
		paramCount = len(stdlib.Parameters)
	}

	activeSig := 0
	activeParam := argIdx
	if paramCount > 0 && activeParam >= paramCount {
		activeParam = paramCount - 1
	}

	return &SignatureHelp{
		Signatures:      []SignatureInformation{sigInfo},
		ActiveSignature: &activeSig,
		ActiveParameter: &activeParam,
	}
}

// findCallAtOffset searches the AST for the innermost CallExpr that encloses
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

// callFuncName extracts a simple dotted name from the Func field of a call.
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

// ---------------------------------------------------------------------------
// Definition
// ---------------------------------------------------------------------------

func (s *Server) handleDefinition(doc *Document, pos Position) []Location {
	if doc.Analysis == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)
	sym := doc.Analysis.SymbolAt(offset)
	if sym == nil {
		return nil
	}
	return []Location{
		{URI: doc.URI, Range: sym.Def.ToLSP()},
	}
}

// ---------------------------------------------------------------------------
// References
// ---------------------------------------------------------------------------

func (s *Server) handleReferences(doc *Document, pos Position, includeDecl bool) []Location {
	if doc.Analysis == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)
	sym := doc.Analysis.SymbolAt(offset)
	if sym == nil {
		return nil
	}
	var locs []Location
	for _, sp := range sym.Refs {
		if !includeDecl && sp == sym.Def {
			continue
		}
		locs = append(locs, Location{URI: doc.URI, Range: sp.ToLSP()})
	}
	return locs
}

// ---------------------------------------------------------------------------
// Rename
// ---------------------------------------------------------------------------

func (s *Server) handleRename(doc *Document, pos Position, newName string) *WorkspaceEdit {
	if doc.Analysis == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)
	sym := doc.Analysis.SymbolAt(offset)
	if sym == nil {
		return nil
	}
	var edits []TextEdit
	for _, sp := range sym.Refs {
		edits = append(edits, TextEdit{
			Range:   sp.ToLSP(),
			NewText: newName,
		})
	}
	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			doc.URI: edits,
		},
	}
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

func (s *Server) handleFormatting(doc *Document, opts FormattingOptions) []TextEdit {
	if doc.Block == nil {
		return nil
	}
	tabSize := opts.TabSize
	if tabSize <= 0 {
		tabSize = 4
	}
	formatted := Format(doc.Block, tabSize, opts.InsertSpaces)

	lines := strings.Split(doc.Text, "\n")
	lastLine := len(lines) - 1
	lastChar := len(lines[lastLine])

	return []TextEdit{
		{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: lastLine, Character: lastChar},
			},
			NewText: formatted,
		},
	}
}
