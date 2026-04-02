package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------//
// Binary integration tests - these test the actual lua-lsp binary
// ---------------------------------------------------------------------------//

const binaryPath = "./lua-lsp"

func TestBinaryExists(t *testing.T) {
	_, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		t.Fatalf("lua-lsp binary not found at %s - run `go build -o lua-lsp ./cmd/lua-lsp`", binaryPath)
	}
}

func TestBinaryVersionFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary --version failed: %v, output: %s", err, string(output))
	}
	if !bytes.Contains(output, []byte("lua-lsp")) {
		t.Errorf("expected output to contain 'lua-lsp', got: %s", string(output))
	}
}

func TestBinaryHelpFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary --help failed: %v, output: %s", err, string(output))
	}
	outputStr := string(output)
	if !strings.Contains(outputStr, "LSP server") && !strings.Contains(outputStr, "lua-lsp") {
		t.Errorf("expected help output to contain server info, got: %s", outputStr)
	}
}

// TestBinaryLSPCycle tests a complete LSP initialize -> shutdown -> exit cycle
func TestBinaryLSPCycle(t *testing.T) {
	cmd := exec.Command(binaryPath)

	// Create pipes for stdin/stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId":    os.Getpid(),
			"rootURI":      "file:///test",
			"capabilities": map[string]interface{}{},
		},
	}

	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Read response
	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read initialize response: %v", err)
	}

	// Verify response
	if resp.ID != 1 {
		t.Errorf("expected id=1, got %v", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("initialize returned error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}

	// Parse the result into a map
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	capabilities := result["capabilities"].(map[string]interface{})

	// Verify basic capabilities
	if _, ok := capabilities["textDocumentSync"]; !ok {
		t.Error("expected textDocumentSync capability")
	}
	if _, ok := capabilities["completionProvider"]; !ok {
		t.Error("expected completionProvider capability")
	}
	if _, ok := capabilities["hoverProvider"]; !ok {
		t.Error("expected hoverProvider capability")
	}
	if _, ok := capabilities["definitionProvider"]; !ok {
		t.Error("expected definitionProvider capability")
	}

	// Send shutdown request
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "shutdown",
	}

	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	// Read shutdown response
	_, err = readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read shutdown response: %v", err)
	}

	// Send exit notification
	exitNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "exit",
	}

	if err := sendJSONRPCRequest(stdin, exitNotif); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("binary exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("binary did not exit in time")
	}
}

// TestBinaryDocumentLifecycle tests document open/close and diagnostics
func TestBinaryDocumentLifecycle(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Wait for initialized notification
	if _, err := readJSONRPCResponse(stdout); err != nil {
		t.Fatalf("failed to read initialized notification: %v", err)
	}

	// Open a document with valid code
	validDoc := `local x = 1
local function foo()
    return x
end
print(foo())`

	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":      validDoc,
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	// Should receive diagnostics notification
	diagResp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read diagnostics: %v", err)
	}

	if diagResp.Method != "textDocument/publishDiagnostics" {
		t.Errorf("expected publishDiagnostics notification, got: %s", diagResp.Method)
	}

	// Parse params into a map
	var diags map[string]interface{}
	if err := json.Unmarshal(diagResp.Params, &diags); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}
	diagnostics := diags["diagnostics"].([]interface{})

	// Check diagnostics have no errors
	for _, d := range diagnostics {
		diag := d.(map[string]interface{})
		if diag["severity"] == 1 {
			t.Errorf("expected no errors in valid code, got: %v", diag)
		}
	}

	// Close the document
	closeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didClose",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, closeReq); err != nil {
		t.Fatalf("failed to send didClose: %v", err)
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout) // ignore response
}

// TestBinaryCompletion tests the completion request
func TestBinaryCompletion(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Open document
	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":       "local myVar = 42\n",
			},
		},
	}
	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	// Consume diagnostics
	readJSONRPCResponse(stdout)

	// Request completion
	completionReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
			"position": map[string]interface{}{
				"line":      1,
				"character": 0,
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, completionReq); err != nil {
		t.Fatalf("failed to send completion: %v", err)
	}

	// Read response
	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read completion response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("completion returned error: %v", resp.Error)
	}

	// Parse result
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	items := result["items"].([]interface{})

	// Check that our local variable is in completions
	found := false
	for _, item := range items {
		i := item.(map[string]interface{})
		if i["label"] == "myVar" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'myVar' in completion items")
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout)
}

// TestBinaryHover tests the hover request
func TestBinaryHover(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Open document
	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":       "local myVar = 42\n",
			},
		},
	}
	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	readJSONRPCResponse(stdout) // diagnostics

	// Request hover on 'myVar' (line 0, around char 6)
	hoverReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/hover",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
			"position": map[string]interface{}{
				"line":      0,
				"character": 6,
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, hoverReq); err != nil {
		t.Fatalf("failed to send hover: %v", err)
	}

	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read hover response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("hover returned error: %v", resp.Error)
	}

	if resp.Result == nil {
		t.Fatal("expected non-nil hover result")
	}

	// Parse result
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	content := result["contents"].(map[string]interface{})
	value := content["value"].(string)

	if !strings.Contains(value, "myVar") {
		t.Errorf("hover should mention 'myVar', got: %s", value)
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout)
}

// TestBinarySignatureHelp tests the signature help request
func TestBinarySignatureHelp(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Open document
	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":       "local function greet(name) return 'hi' end\n",
			},
		},
	}
	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	readJSONRPCResponse(stdout) // diagnostics

	// Request signature help at the call site
	sigHelpReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/signatureHelp",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
			"position": map[string]interface{}{
				"line":      1,
				"character": 7, // right after "greet("
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, sigHelpReq); err != nil {
		t.Fatalf("failed to send signature help: %v", err)
	}

	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read signature help response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("signature help returned error: %v", resp.Error)
	}

	if resp.Result == nil {
		t.Fatal("expected non-nil signature help result")
	}

	// Parse result
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	signatures := result["signatures"].([]interface{})

	if len(signatures) == 0 {
		t.Fatal("expected at least one signature")
	}

	sig := signatures[0].(map[string]interface{})
	if !strings.Contains(sig["label"].(string), "greet") {
		t.Errorf("signature should contain 'greet', got: %s", sig["label"])
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout)
}

// TestBinaryDefinition tests the go-to-definition request
func TestBinaryDefinition(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Open document
	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":       "local myFunc = function() end\nmyFunc()\n",
			},
		},
	}
	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	readJSONRPCResponse(stdout) // diagnostics

	// Request definition at the call site (line 1, "myFunc()")
	defReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
			"position": map[string]interface{}{
				"line":      1,
				"character": 0,
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, defReq); err != nil {
		t.Fatalf("failed to send definition: %v", err)
	}

	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read definition response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("definition returned error: %v", resp.Error)
	}

	// Parse result
	var result []interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one definition location")
	}

	loc := result[0].(map[string]interface{})
	if loc["uri"] != "file:///test.lua" {
		t.Errorf("expected URI 'file:///test.lua', got: %s", loc["uri"])
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout)
}

// TestBinaryFormatting tests the formatting request
func TestBinaryFormatting(t *testing.T) {
	cmd := exec.Command(binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdout.Close()

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	defer cmd.Process.Kill()

	// Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": os.Getpid(),
			"rootURI":   "file:///test",
		},
	}
	if err := sendJSONRPCRequest(stdin, initReq); err != nil {
		t.Fatalf("failed to send initialize: %v", err)
	}

	// Open document with unformatted code
	openReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":       "file:///test.lua",
				"languageId": "lua",
				"version":   1,
				"text":       "local x=1\nlocal y=2\nif x==1 then y=3 end",
			},
		},
	}
	if err := sendJSONRPCRequest(stdin, openReq); err != nil {
		t.Fatalf("failed to send didOpen: %v", err)
	}

	readJSONRPCResponse(stdout) // diagnostics

	// Request formatting
	formatReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/formatting",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///test.lua",
			},
			"options": map[string]interface{}{
				"tabSize":      4,
				"insertSpaces": false,
			},
		},
	}

	if err := sendJSONRPCRequest(stdin, formatReq); err != nil {
		t.Fatalf("failed to send formatting: %v", err)
	}

	resp, err := readJSONRPCResponse(stdout)
	if err != nil {
		t.Fatalf("failed to read formatting response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("formatting returned error: %v", resp.Error)
	}

	// Parse result
	var result []interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one text edit")
	}

	edit := result[0].(map[string]interface{})
	newText := edit["newText"].(string)

	// Check that output contains formatted code with indentation
	if !strings.Contains(newText, "local x = 1") {
		t.Errorf("expected 'local x = 1' in formatted output, got: %s", newText)
	}

	// Shutdown
	shutdownReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	if err := sendJSONRPCRequest(stdin, shutdownReq); err != nil {
		t.Fatalf("failed to send shutdown: %v", err)
	}

	readJSONRPCResponse(stdout)
}

// JSON-RPC helpers

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type responseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func sendJSONRPCRequest(w io.Writer, req map[string]interface{}) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	header := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data)))
	_, err = w.Write(append(header, data...))
	if err != nil {
		return err
	}
	return nil
}

func readJSONRPCResponse(r io.Reader) (*jsonRPCResponse, error) {
	reader := bufio.NewReader(r)

	// Read headers
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
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
				return nil, err
			}
		}
	}

	if contentLength == 0 {
		return nil, io.EOF
	}

	// Read body
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, err
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
