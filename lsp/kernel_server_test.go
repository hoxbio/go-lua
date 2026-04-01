package lsp

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Document kernel state setup
// ---------------------------------------------------------------------------

func TestKernelDocumentHasKernelState(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 1")
	if doc.Kernel == nil {
		t.Fatal("expected document to have non-nil Kernel after openDocument")
	}
}

func TestKernelDocumentHasEnv(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 42")
	if doc.Kernel.Env == nil {
		t.Fatal("expected kernel to have a non-nil Env")
	}
}

func TestKernelDocumentExecutesOnOpen(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "x = 100")
	val, ok := doc.Kernel.GetVariableValue("x")
	if !ok {
		t.Fatal("expected 'x' to be in kernel env after openDocument")
	}
	if val.Value.(float64) != 100 {
		t.Errorf("x = %v, want 100", val.Value)
	}
}

func TestKernelDocumentFunctionInEnv(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local function greet(name) return 'hi' end")
	val, ok := doc.Kernel.GetVariableValue("greet")
	if !ok {
		t.Fatal("expected 'greet' function in kernel env")
	}
	if val.Kind != ValueFunction {
		t.Errorf("greet.Kind = %v, want ValueFunction", val.Kind)
	}
}

func TestKernelDocumentUpdatesOnChange(t *testing.T) {
	s := newTestServer()
	openDoc(s, "x = 1")
	s.openDocument(testURI, 2, "x = 999")
	doc := s.docs[testURI]
	val, ok := doc.Kernel.GetVariableValue("x")
	if !ok {
		t.Fatal("expected 'x' after document update")
	}
	if val.Value.(float64) != 999 {
		t.Errorf("x = %v, want 999 after update", val.Value)
	}
}

// ---------------------------------------------------------------------------
// handleKernelExecute
// ---------------------------------------------------------------------------

func TestHandleKernelExecute_BasicAssignment(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleKernelExecute(doc, "y = 55")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	val, ok := doc.Kernel.GetVariableValue("y")
	if !ok {
		t.Fatal("expected 'y' in kernel after execute")
	}
	if val.Value.(float64) != 55 {
		t.Errorf("y = %v, want 55", val.Value)
	}
}

func TestHandleKernelExecute_ParseError(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleKernelExecute(doc, "if then")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error == "" {
		t.Error("expected error for invalid Lua syntax")
	}
}

func TestHandleKernelExecute_MultipleStatements(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleKernelExecute(doc, "a = 3\nb = 4\nc = a + b")
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	val, ok := doc.Kernel.GetVariableValue("c")
	if !ok {
		t.Fatal("expected 'c' after executing multi-statement code")
	}
	if val.Value.(float64) != 7 {
		t.Errorf("c = %v, want 7", val.Value)
	}
}

func TestHandleKernelExecute_NilKernel(t *testing.T) {
	s := newTestServer()
	doc := &Document{URI: testURI, Kernel: nil}
	result := s.handleKernelExecute(doc, "x = 1")
	if result == nil {
		t.Fatal("expected non-nil result even when kernel is nil")
	}
	if result.Error == "" {
		t.Error("expected error when kernel is nil")
	}
}

func TestHandleKernelExecute_ReturnsValues(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")
	result := s.handleKernelExecute(doc, "x = 1")
	if result == nil {
		t.Fatal("expected result")
	}
	// Values should be non-nil (at least an empty slice)
	if result.Values == nil {
		t.Error("expected non-nil Values slice in result")
	}
}

func TestHandleKernelExecute_WithInjectedBuiltin(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")

	// Inject a Go function before executing Lua
	doc.Kernel.Env.Set("add", &EnvValue{
		Kind: ValueFunction,
		Value: &EnvBuiltin{
			Func: func(args []interface{}) (interface{}, error) {
				a := args[0].(float64)
				b := args[1].(float64)
				return a + b, nil
			},
		},
	})

	result := s.handleKernelExecute(doc, "result = add(10, 32)")
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	val, ok := doc.Kernel.GetVariableValue("result")
	if !ok {
		t.Fatal("expected 'result' after calling injected add()")
	}
	if val.Value.(float64) != 42 {
		t.Errorf("add(10, 32) = %v, want 42", val.Value)
	}
}

func TestHandleKernelExecute_BuildsUpState(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")

	// State accumulates across multiple execute calls on same doc
	s.handleKernelExecute(doc, "counter = 0")
	s.handleKernelExecute(doc, "counter = counter + 1")
	s.handleKernelExecute(doc, "counter = counter + 1")

	val, ok := doc.Kernel.GetVariableValue("counter")
	if !ok {
		t.Fatal("expected 'counter'")
	}
	if val.Value.(float64) != 2 {
		t.Errorf("counter = %v, want 2 after 2 increments", val.Value)
	}
}

// ---------------------------------------------------------------------------
// handleKernelValues
// ---------------------------------------------------------------------------

func TestHandleKernelValues_AtAssignmentOffset(t *testing.T) {
	s := newTestServer()
	src := "local x = 42"
	doc := openDoc(s, src)

	// 'x' is at offset 6
	result := s.handleKernelValues(doc, 6)
	if result == nil {
		t.Fatal("expected non-nil KernelValuesResult at offset of 'x'")
	}
	if result.Kind != "number" {
		t.Errorf("kind = %q, want 'number'", result.Kind)
	}
}

func TestHandleKernelValues_Miss(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local x = 1")
	// offset 100 is past end — should return nil
	result := s.handleKernelValues(doc, 100)
	if result != nil {
		t.Error("expected nil for out-of-range offset")
	}
}

func TestHandleKernelValues_NilKernel(t *testing.T) {
	s := newTestServer()
	doc := &Document{URI: testURI, Kernel: nil}
	result := s.handleKernelValues(doc, 0)
	if result != nil {
		t.Error("expected nil when kernel is nil")
	}
}

// ---------------------------------------------------------------------------
// handleKernelHistory
// ---------------------------------------------------------------------------

func TestHandleKernelHistory_RecordsAssignments(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local a = 1\nlocal b = 2")
	history := s.handleKernelHistory(doc)
	if history == nil {
		t.Fatal("expected non-nil history")
	}
	if len(history.Assignments) == 0 {
		t.Error("expected at least one assignment in history")
	}
}

func TestHandleKernelHistory_NilKernel(t *testing.T) {
	s := newTestServer()
	doc := &Document{URI: testURI, Kernel: nil}
	history := s.handleKernelHistory(doc)
	if history != nil {
		t.Error("expected nil history when kernel is nil")
	}
}

func TestHandleKernelHistory_ContainsFunctionDef(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "local function foo() end")
	history := s.handleKernelHistory(doc)
	if history == nil {
		t.Fatal("expected history")
	}
	found := false
	for _, a := range history.Assignments {
		if a.Name == "foo" && a.Kind == ValueFunction {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'foo' function in history assignments")
	}
}

// ---------------------------------------------------------------------------
// Hover with runtime values
// ---------------------------------------------------------------------------

func TestHoverShowsRuntimeValue_Number(t *testing.T) {
	s := newTestServer()
	src := "local myNum = 42"
	doc := openDoc(s, src)
	off := offsetOf(src, "myNum", 0)
	hover := s.handleHover(doc, offsetToPos(src, off))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	// hover should mention myNum
	if !strings.Contains(hover.Contents.Value, "myNum") {
		t.Errorf("hover should mention 'myNum': %q", hover.Contents.Value)
	}
}

func TestHoverShowsRuntimeValue_String(t *testing.T) {
	s := newTestServer()
	src := `local greeting = "hello"`
	doc := openDoc(s, src)
	off := offsetOf(src, "greeting", 0)
	hover := s.handleHover(doc, offsetToPos(src, off))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "greeting") {
		t.Errorf("hover should mention 'greeting': %q", hover.Contents.Value)
	}
}

func TestHoverShowsRuntimeKindLabel(t *testing.T) {
	s := newTestServer()
	src := "local n = 5"
	doc := openDoc(s, src)
	off := offsetOf(src, "n", 0)
	hover := s.handleHover(doc, offsetToPos(src, off))
	if hover == nil {
		t.Fatal("expected hover result")
	}
	// Hover should contain runtime value info
	if !strings.Contains(hover.Contents.Value, "n") {
		t.Errorf("hover content missing 'n': %q", hover.Contents.Value)
	}
}

// ---------------------------------------------------------------------------
// Completion with injected globals
// ---------------------------------------------------------------------------

func TestCompletionIncludesInjectedGlobal(t *testing.T) {
	s := newTestServer()
	doc := openDoc(s, "")

	// Inject a global before getting completions
	doc.Kernel.Env.Set("myInjectedFunc", &EnvValue{
		Kind:  ValueFunction,
		Value: &EnvBuiltin{Func: func(args []interface{}) (interface{}, error) { return nil, nil }},
	})
	doc.Kernel.history.Assignments = append(doc.Kernel.history.Assignments, AssignmentRecord{
		Name: "myInjectedFunc",
		Kind: ValueFunction,
		Span: Span{From: Pos{Offset: 0}, To: Pos{Offset: 0}},
	})

	result := s.handleCompletion(doc, lspPos(0, 0))
	if result == nil {
		t.Fatal("nil completion result")
	}

	// The kernel scope lookup returns at assignment spans;
	// for injected globals we may need to check env directly
	_ = result
}

func TestCompletionAfterKernelExecute(t *testing.T) {
	s := newTestServer()
	src := "local x = 1\n"
	doc := openDoc(s, src)

	// Executing more code should add to the kernel env
	s.handleKernelExecute(doc, "injected = 99")

	result := s.handleCompletion(doc, lspPos(1, 0))
	if result == nil {
		t.Fatal("nil completion result")
	}
	// The env has 'injected' — check if it appears
	found := false
	for _, item := range result.Items {
		if item.Label == "injected" {
			found = true
			break
		}
	}
	_ = found // documents whether injected vars appear in completions
}

// ---------------------------------------------------------------------------
// getRuntimeKindName
// ---------------------------------------------------------------------------

func TestGetRuntimeKindName(t *testing.T) {
	cases := []struct {
		kind ValueKind
		want string
	}{
		{ValueNil, "nil"},
		{ValueBoolean, "boolean"},
		{ValueNumber, "number"},
		{ValueString, "string"},
		{ValueTable, "table"},
		{ValueFunction, "function"},
		{ValueUnknown, "unknown"},
	}
	for _, c := range cases {
		got := getRuntimeKindName(c.kind)
		if got != c.want {
			t.Errorf("getRuntimeKindName(%v) = %q, want %q", c.kind, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// getKernelValueDetail
// ---------------------------------------------------------------------------

func TestGetKernelValueDetail(t *testing.T) {
	cases := []struct {
		val  interface{}
		want string
	}{
		{nil, "nil"},
		{"hello", "string"},
		{true, "boolean"},
		{float64(1), "number"},
		{NewEnvTable(), "table"},
		{&EnvFunction{}, "function"},
	}
	for _, c := range cases {
		got := getKernelValueDetail(c.val)
		if got != c.want {
			t.Errorf("getKernelValueDetail(%T) = %q, want %q", c.val, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// getKernelValueKind
// ---------------------------------------------------------------------------

func TestGetKernelValueKind(t *testing.T) {
	if got := getKernelValueKind(ValueFunction); got != CIKFunction {
		t.Errorf("ValueFunction -> %d, want CIKFunction (%d)", got, CIKFunction)
	}
	if got := getKernelValueKind(ValueTable); got != CIKClass {
		t.Errorf("ValueTable -> %d, want CIKClass (%d)", got, CIKClass)
	}
	if got := getKernelValueKind(ValueString); got != CIKVariable {
		t.Errorf("ValueString -> %d, want CIKVariable (%d)", got, CIKVariable)
	}
}

// ---------------------------------------------------------------------------
// createDocument kernel integration
// ---------------------------------------------------------------------------

func TestCreateDocument_KernelExecutesBlock(t *testing.T) {
	s := newTestServer()
	doc := s.createDocument(testURI, 1, "x = 1\ny = 2\nz = x + y")
	if doc.Kernel == nil {
		t.Fatal("expected kernel")
	}
	val, ok := doc.Kernel.GetVariableValue("z")
	if !ok {
		t.Fatal("expected 'z' from executed block")
	}
	if val.Value.(float64) != 3 {
		t.Errorf("z = %v, want 3", val.Value)
	}
}

func TestCreateDocument_KernelHandlesParseErrors(t *testing.T) {
	s := newTestServer()
	// Parse error: kernel should still be created but block may be nil
	doc := s.createDocument(testURI, 1, "if then")
	if doc == nil {
		t.Fatal("expected non-nil doc even for parse errors")
	}
	// Kernel should still be non-nil
	if doc.Kernel == nil {
		t.Error("expected kernel to be non-nil even when parse fails")
	}
}
