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

// Document holds everything the server knows about an open text document with kernel state
type Document struct {
	URI      string
	Version  int
	Text     string
	Block    *Block
	Analysis *Analysis
	Kernel   *KernelState
	Errors   []SyntaxError
}

// Server is the LSP server with kernel state support
type Server struct {
	docs map[string]*Document
	in   *bufio.Reader
	out  *bufio.Writer
	mu   sync.Mutex
}

// NewServer creates an LSP server with kernel state
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		docs: make(map[string]*Document),
		in:   bufio.NewReader(in),
		out:  bufio.NewWriter(out),
	}
}

// readMessage reads one JSON-RPC message from stdin
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

// writeMessage sends raw bytes to the client with Content-Length framing
func (s *Server) writeMessage(data []byte) {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.out.WriteString(header)
	s.out.Write(data)
	s.out.Flush()
}

// send sends a successful JSON-RPC response
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

// sendError sends a JSON-RPC error response
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

// notify sends a JSON-RPC notification (no ID)
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

// Run starts the server's main message-dispatch loop
func (s *Server) Run() {
	for {
		msg, err := s.readMessage()
		if err != nil {
			return
		}
		s.dispatch(msg)
	}
}

// dispatch routes a request to the appropriate handler
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
	case "lua/kernel/execute":
		var p KernelExecuteParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.sendError(id, ErrCodeInvalidRequest, "document not found")
			return
		}
		result := s.handleKernelExecute(doc, p.Code)
		s.send(id, result)
	case "lua/kernel/values":
		var p KernelValuesParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleKernelValues(doc, p.Offset))
	case "lua/kernel/history":
		var p KernelHistoryParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, err.Error())
			return
		}
		doc, ok := s.docs[p.TextDocument.URI]
		if !ok {
			s.send(id, nil)
			return
		}
		s.send(id, s.handleKernelHistory(doc))
	default:
		if id != nil {
			s.sendError(id, ErrCodeMethodNotFound, fmt.Sprintf("method not found: %s", msg.Method))
		}
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(id interface{}, params json.RawMessage) {
	serverName := "lua-lsp"
	serverVersion := "0.2.0"
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1,
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

// openDocument creates a document with kernel state
func (s *Server) openDocument(uri string, version int, text string) {
	doc := s.createDocument(uri, version, text)
	s.docs[uri] = doc
	s.publishDiagnostics(doc)
}

// createDocument creates a new document with full kernel state
func (s *Server) createDocument(uri string, version int, text string) *Document {
	p := NewParser(text)
	block, errs := p.Parse()

	var analysis *Analysis
	if block != nil {
		analysis = Analyze(block)
	}

	kernel := NewKernelState(uri, text, version)
	if block != nil {
		kernel.Block = block
		kernel.Analysis = analysis
		kernel.ExecuteBlock(block)
	}

	doc := &Document{
		URI:      uri,
		Version:  version,
		Text:     text,
		Block:    block,
		Analysis: analysis,
		Kernel:   kernel,
		Errors:   errs,
	}

	return doc
}

// publishDiagnostics sends textDocument/publishDiagnostics
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

// posToOffset converts an LSP (line, character) position to a byte offset
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

// Lua keywords for completion
var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
}

// Standard library globals
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

// handleCompletion handles textDocument/completion with runtime values
func (s *Server) handleCompletion(doc *Document, pos Position) *CompletionList {
	if doc.Analysis == nil {
		return &CompletionList{Items: []CompletionItem{}}
	}

	offset := posToOffset(doc.Text, pos)
	scope := doc.Analysis.ScopeAt(offset)

	seen := make(map[string]bool)
	var items []CompletionItem

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

	if doc.Kernel != nil {
		kernelScope := getKernelScopeAtOffset(doc.Kernel, offset)
		for sc := kernelScope; sc != nil; sc = sc.Parent {
			for name, val := range sc.Symbols {
				if !seen[name] {
					seen[name] = true
					kind := getKernelValueKind(val.Kind)
					detail := getKernelValueDetail(val.Value)
					items = append(items, CompletionItem{
						Label:  name,
						Kind:   kind,
						Detail: detail,
					})
				}
			}
		}
	}

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

// getKernelScopeAtOffset finds kernel scope at offset
func getKernelScopeAtOffset(kernel *KernelState, offset int) *Env {
	for _, assignment := range kernel.history.Assignments {
		if assignment.Span.Contains(Pos{Offset: offset}) {
			return kernel.Env
		}
	}
	return nil
}

// getKernelValueKind converts ValueKind to CompletionItemKind
func getKernelValueKind(kind ValueKind) CompletionItemKind {
	switch kind {
	case ValueFunction:
		return CIKFunction
	case ValueTable:
		return CIKClass
	default:
		return CIKVariable
	}
}

// getKernelValueDetail gets a detail string for kernel value
func getKernelValueDetail(val interface{}) string {
	if val == nil {
		return "nil"
	}
	switch val.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case *EnvTable:
		return "table"
	case *EnvFunction:
		return "function"
	}
	return fmt.Sprintf("%T", val)
}

// handleHover handles textDocument/hover with runtime values
func (s *Server) handleHover(doc *Document, pos Position) *Hover {
	if doc.Analysis == nil {
		return nil
	}
	offset := posToOffset(doc.Text, pos)
	sym := doc.Analysis.SymbolAt(offset)

	if sym != nil {
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

		if doc.Kernel != nil {
			if val, ok := doc.Kernel.GetVariableValue(sym.Name); ok {
				sb.WriteString("\n\n**Runtime value:**\n")
				sb.WriteString(fmt.Sprintf("```lua\n%v\n```", val.Value))
				sb.WriteString("\n**Type:** " + getRuntimeKindName(val.Kind))
			}
		}

		r := sym.Def.ToLSP()
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: sb.String(),
			},
			Range: &r,
		}
	}

	if doc.Kernel != nil {
		if val, ok := doc.Kernel.GetVariableValueAt(offset); ok {
			var sb strings.Builder
			sb.WriteString("```lua\n")
			sb.WriteString(getRuntimeKindName(val.Kind))
			sb.WriteString(" value\n")
			sb.WriteString("```\n\n")
			sb.WriteString(fmt.Sprintf("**Value:** `%v`", val.Value))

			return &Hover{
				Contents: MarkupContent{
					Kind:  "markdown",
					Value: sb.String(),
				},
			}
		}
	}

	return nil
}

// getRuntimeKindName converts ValueKind to string
func getRuntimeKindName(kind ValueKind) string {
	switch kind {
	case ValueNil:
		return "nil"
	case ValueBoolean:
		return "boolean"
	case ValueNumber:
		return "number"
	case ValueString:
		return "string"
	case ValueTable:
		return "table"
	case ValueFunction:
		return "function"
	default:
		return "unknown"
	}
}

// handleSignatureHelp handles textDocument/signatureHelp
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

// handleDefinition handles textDocument/definition
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

// handleReferences handles textDocument/references
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

// handleRename handles textDocument/rename
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

// handleFormatting handles textDocument/formatting
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
