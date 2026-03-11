package main

import "encoding/json"

// ---------------------------------------------------------------------------
// JSON-RPC base messages
// ---------------------------------------------------------------------------

// RequestMessage is a JSON-RPC request.
type RequestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"` // string | number | null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ResponseMessage is a JSON-RPC response.
type ResponseMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// NotificationMessage is a JSON-RPC notification (no ID).
type NotificationMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ResponseError represents a JSON-RPC error object.
type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ---------------------------------------------------------------------------
// JSON-RPC error codes
// ---------------------------------------------------------------------------

const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// ---------------------------------------------------------------------------
// Core LSP types
// ---------------------------------------------------------------------------

// Position is a zero-based line/character pair.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a start/end position pair.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a URI + range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a text document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier adds a version number.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentItem is a complete text document transferred to the server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams identifies a position inside a document.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// TextEdit is a textual edit applicable to a text document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// ---------------------------------------------------------------------------
// Initialize
// ---------------------------------------------------------------------------

// InitializeParams are sent by the client on the initialize request.
type InitializeParams struct {
	ProcessID             *int        `json:"processId"`
	ClientInfo            *ClientInfo `json:"clientInfo,omitempty"`
	RootURI               *string     `json:"rootUri"`
	InitializationOptions interface{} `json:"initializationOptions,omitempty"`
	Capabilities          interface{} `json:"capabilities,omitempty"`
}

// ClientInfo carries client name/version metadata.
type ClientInfo struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// InitializeResult is the response to the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo carries server name/version metadata.
type ServerInfo struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// ServerCapabilities declares what the server supports.
type ServerCapabilities struct {
	TextDocumentSync           *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	CompletionProvider         *CompletionOptions       `json:"completionProvider,omitempty"`
	HoverProvider              bool                     `json:"hoverProvider,omitempty"`
	SignatureHelpProvider      *SignatureHelpOptions    `json:"signatureHelpProvider,omitempty"`
	DefinitionProvider         bool                     `json:"definitionProvider,omitempty"`
	ReferencesProvider         bool                     `json:"referencesProvider,omitempty"`
	RenameProvider             bool                     `json:"renameProvider,omitempty"`
	DocumentFormattingProvider bool                     `json:"documentFormattingProvider,omitempty"`
}

// TextDocumentSyncOptions describes how text document notifications are sent.
type TextDocumentSyncOptions struct {
	OpenClose bool `json:"openClose,omitempty"`
	Change    int  `json:"change"` // 0=None, 1=Full, 2=Incremental
}

// CompletionOptions configures the completion provider.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
}

// SignatureHelpOptions configures signature help.
type SignatureHelpOptions struct {
	TriggerCharacters   []string `json:"triggerCharacters,omitempty"`
	RetriggerCharacters []string `json:"retriggerCharacters,omitempty"`
}

// ---------------------------------------------------------------------------
// textDocument/didOpen
// ---------------------------------------------------------------------------

// DidOpenTextDocumentParams are the params for textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// ---------------------------------------------------------------------------
// textDocument/didChange
// ---------------------------------------------------------------------------

// DidChangeTextDocumentParams are the params for textDocument/didChange.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent describes a change to a text document.
// When Range is nil the entire document is replaced.
type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength *int   `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

// ---------------------------------------------------------------------------
// textDocument/didClose
// ---------------------------------------------------------------------------

// DidCloseTextDocumentParams are the params for textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// ---------------------------------------------------------------------------
// textDocument/completion
// ---------------------------------------------------------------------------

// CompletionParams are the params for textDocument/completion.
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *CompletionContext     `json:"context,omitempty"`
}

// CompletionContext provides additional context when requesting completions.
type CompletionContext struct {
	TriggerKind      int     `json:"triggerKind"`
	TriggerCharacter *string `json:"triggerCharacter,omitempty"`
}

// CompletionList holds a list of completion items.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// CompletionItem is a single completion suggestion.
type CompletionItem struct {
	Label               string      `json:"label"`
	Kind                int         `json:"kind,omitempty"`
	Detail              string      `json:"detail,omitempty"`
	Documentation       interface{} `json:"documentation,omitempty"` // string | MarkupContent
	InsertText          string      `json:"insertText,omitempty"`
	InsertTextFormat    int         `json:"insertTextFormat,omitempty"` // 1=PlainText, 2=Snippet
	TextEdit            *TextEdit   `json:"textEdit,omitempty"`
	AdditionalTextEdits []TextEdit  `json:"additionalTextEdits,omitempty"`
	CommitCharacters    []string    `json:"commitCharacters,omitempty"`
	SortText            string      `json:"sortText,omitempty"`
	FilterText          string      `json:"filterText,omitempty"`
	Preselect           bool        `json:"preselect,omitempty"`
	Data                interface{} `json:"data,omitempty"`
}

// CompletionItemKind constants.
const (
	CIKText     = 1
	CIKMethod   = 2
	CIKFunction = 3
	CIKField    = 5
	CIKVariable = 6
	CIKKeyword  = 14
	CIKSnippet  = 15
)

// ---------------------------------------------------------------------------
// textDocument/hover
// ---------------------------------------------------------------------------

// HoverParams are the params for textDocument/hover.
type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// Hover is the result of a hover request.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent represents human-readable content with a kind (plaintext or markdown).
type MarkupContent struct {
	Kind  string `json:"kind"`  // "plaintext" | "markdown"
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// textDocument/signatureHelp
// ---------------------------------------------------------------------------

// SignatureHelpParams are the params for textDocument/signatureHelp.
type SignatureHelpParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// SignatureHelp is the result of a signatureHelp request.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature *int                   `json:"activeSignature,omitempty"`
	ActiveParameter *int                   `json:"activeParameter,omitempty"`
}

// SignatureInformation represents the signature of a callable.
type SignatureInformation struct {
	Label         string               `json:"label"`
	Documentation interface{}          `json:"documentation,omitempty"` // string | MarkupContent
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

// ParameterInformation represents a single parameter within a signature.
type ParameterInformation struct {
	Label         interface{} `json:"label"` // string | [startOffset, endOffset]
	Documentation interface{} `json:"documentation,omitempty"`
}

// ---------------------------------------------------------------------------
// textDocument/definition
// ---------------------------------------------------------------------------

// DefinitionParams are the params for textDocument/definition.
type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ---------------------------------------------------------------------------
// textDocument/references
// ---------------------------------------------------------------------------

// ReferenceParams are the params for textDocument/references.
type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

// ReferenceContext controls whether the declaration is included in results.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ---------------------------------------------------------------------------
// textDocument/rename
// ---------------------------------------------------------------------------

// RenameParams are the params for textDocument/rename.
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// WorkspaceEdit describes changes to many resources in the workspace.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []interface{}         `json:"documentChanges,omitempty"`
}

// ---------------------------------------------------------------------------
// textDocument/formatting
// ---------------------------------------------------------------------------

// DocumentFormattingParams are the params for textDocument/formatting.
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

// FormattingOptions controls how the formatter behaves.
type FormattingOptions struct {
	TabSize                int  `json:"tabSize"`
	InsertSpaces           bool `json:"insertSpaces"`
	TrimTrailingWhitespace bool `json:"trimTrailingWhitespace,omitempty"`
	InsertFinalNewline     bool `json:"insertFinalNewline,omitempty"`
	TrimFinalNewlines      bool `json:"trimFinalNewlines,omitempty"`
}

// ---------------------------------------------------------------------------
// textDocument/publishDiagnostics
// ---------------------------------------------------------------------------

// PublishDiagnosticsParams are sent from server to client.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnostic represents a compiler/analysis diagnostic.
type Diagnostic struct {
	Range    Range   `json:"range"`
	Severity int     `json:"severity,omitempty"`
	Code     *string `json:"code,omitempty"`
	Source   string  `json:"source,omitempty"`
	Message  string  `json:"message"`
}

// DiagnosticSeverity constants.
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)
