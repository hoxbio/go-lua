package lua

import (
	"fmt"
	"bytes"
	"strings"
	"testing"
	"path/filepath"
	"os"
)

func TestPushFStringPointer(t *testing.T) {
	l := NewState()
	l.PushFString("%p %s", l, "test")

	expected := fmt.Sprintf("%p %s", l, "test")
	actual := CheckString(l, -1)
	if expected != actual {
		t.Errorf("PushFString, expected \"%s\" but found \"%s\"", expected, actual)
	}
}

func TestToBooleanOutOfRange(t *testing.T) {
	l := NewState()
	l.SetTop(0)
	l.PushBoolean(false)
	l.PushBoolean(true)

	for i, want := range []bool{false, true, false, false} {
		idx := 1 + i
		if got := l.ToBoolean(idx); got != want {
			t.Errorf("l.ToBoolean(%d) = %t; want %t", idx, got, want)
		}
	}
}
func TestStdout(t *testing.T) {
	l := NewState()
	var buf bytes.Buffer
	l.SetStdout(&buf)
	OpenLibraries(l)
	err := DoString(l, `print("hello")`)
	if err != nil {
		t.Fatalf("error running test: %s", err)
	}
	if got := buf.String(); got != "hello\n" {
		t.Errorf("expected %q, got %q", "hello\n", got)
	}
}

func TestStdin(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	l.SetStdin(strings.NewReader("42"))
	err := DoString(l, `x = io.read("*n"); assert(x == 42)`)
	if err != nil {
		t.Fatalf("error running test: %s", err)
	}
}

func TestStderr(t *testing.T) {
	l := NewState()
	var buf bytes.Buffer
	l.SetStderr(&buf)
	OpenLibraries(l)
	err := DoString(l, `io.stderr:write("oops")`)
	if err != nil {
		t.Fatalf("error running test: %s", err)
	}
	if got := buf.String(); got != "oops" {
		t.Errorf("expected %q, got %q", "oops", got)
	}
}

func TestSetRootOpenFileInRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.lua"), []byte(`return 42`), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f = io.open("test.lua", "r")
		assert(f, "failed to open test.lua")
		local s = f:read("*a")
		assert(s == "return 42", "expected 'return 42', got " .. tostring(s))
		f:close()
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootBlocksParentTraversal(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	// Create a file in the parent directory
	secret := filepath.Join(parent, "secret.txt")
	os.WriteFile(secret, []byte("sensitive"), 0644)
	defer os.Remove(secret)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f, msg = io.open("../secret.txt", "r")
		assert(f == nil, "should not be able to open ../secret.txt")
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootBlocksAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f, msg = io.open("/etc/passwd", "r")
		assert(f == nil, "should not be able to open /etc/passwd")
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootAllowsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "data.txt"), []byte("nested"), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f = io.open("sub/deep/data.txt", "r")
		assert(f, "failed to open sub/deep/data.txt")
		local s = f:read("*a")
		assert(s == "nested", "expected 'nested', got " .. tostring(s))
		f:close()
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootWriteFile(t *testing.T) {
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f = io.open("output.txt", "w")
		assert(f, "failed to open output.txt for writing")
		f:write("hello from lua")
		f:close()
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("failed to read output.txt: %s", err)
	}
	if string(data) != "hello from lua" {
		t.Errorf("expected 'hello from lua', got %q", string(data))
	}
}

func TestSetRootWriteBlocksTraversal(t *testing.T) {
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f, msg = io.open("../escape.txt", "w")
		assert(f == nil, "should not be able to write outside root")
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootLoadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mod.lua"), []byte(`return 99`), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local f = loadfile("mod.lua")
		assert(f, "failed to loadfile mod.lua")
		local r = f()
		assert(r == 99, "expected 99, got " .. tostring(r))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootDoFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "run.lua"), []byte(`x = 7`), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		dofile("run.lua")
		assert(x == 7, "expected x == 7, got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestNoRootUnrestricted(t *testing.T) {
	f, err := os.CreateTemp("", "noroottest")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("unrestricted")
	f.Close()
	defer os.Remove(f.Name())

	l := NewState()
	OpenLibraries(l)

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		assert(f, "failed to open temp file")
		local s = f:read("*a")
		assert(s == "unrestricted", "expected 'unrestricted', got " .. tostring(s))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestSetRootLines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lines.txt"), []byte("aaa\nbbb\nccc\n"), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	l := NewState()
	l.SetRoot(root)
	OpenLibraries(l)

	err = DoString(l, `
		local result = {}
		for line in io.lines("lines.txt") do
			result[#result + 1] = line
		end
		assert(#result == 3, "expected 3 lines, got " .. #result)
		assert(result[1] == "aaa", "expected aaa, got " .. result[1])
		assert(result[2] == "bbb", "expected bbb, got " .. result[2])
		assert(result[3] == "ccc", "expected ccc, got " .. result[3])
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}


