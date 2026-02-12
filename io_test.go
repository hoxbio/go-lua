package lua

import(
	"testing"
	"strings"
	"os"
	"fmt"
)

func TestReadLine(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("hello\nworld\n"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*l")
		assert(x == "hello", "expected hello, got " .. tostring(x))
		y = io.read("*l")
		assert(y == "world", "expected world, got " .. tostring(y))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadLineNoTrailingNewline(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("no newline"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*l")
		assert(x == "no newline", "expected 'no newline', got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadLineKeepNewline(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("hello\n"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*L")
		assert(x == "hello\n", "expected 'hello\\n', got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadNumber(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("42.5"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*n")
		assert(x == 42.5, "expected 42.5, got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadAll(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("all\nthe\nthings"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*a")
		assert(x == "all\nthe\nthings", "expected full content, got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadAllEmpty(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader(""))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*a")
		assert(x == "", "expected empty string, got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadChars(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("abcdefgh"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read(3)
		assert(x == "abc", "expected abc, got " .. tostring(x))
		y = io.read(5)
		assert(y == "defgh", "expected defgh, got " .. tostring(y))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadDefault(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("default line\n"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read()
		assert(x == "default line", "expected 'default line', got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadAtEOF(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader(""))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*l")
		assert(x == nil, "expected nil at EOF, got " .. tostring(x))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestReadLongFormats(t *testing.T) {
	l := NewState()
	l.SetStdin(strings.NewReader("hello\n42"))
	OpenLibraries(l)
	err := DoString(l, `
		x = io.read("*line")
		assert(x == "hello", "expected hello, got " .. tostring(x))
		y = io.read("*number")
		assert(y == 42, "expected 42, got " .. tostring(y))
	`)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekDefault(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("hello world")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		local pos = f:seek("set", 6)
		assert(pos == 6, "expected pos 6, got " .. tostring(pos))
		local s = f:read(5)
		assert(s == "world", "expected 'world', got " .. tostring(s))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekCur(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("abcdefghij")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		f:read(3)
		local pos = f:seek("cur", 0)
		assert(pos == 3, "expected pos 3, got " .. tostring(pos))
		local s = f:read(4)
		assert(s == "defg", "expected 'defg', got " .. tostring(s))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekEnd(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("abcdefghij")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		local pos = f:seek("end", 0)
		assert(pos == 10, "expected pos 10, got " .. tostring(pos))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekDefaultIsCur(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("abcdefghij")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		f:read(5)
		local pos = f:seek()
		assert(pos == 5, "expected pos 5, got " .. tostring(pos))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekThenWrite(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("hello world")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r+")
		f:seek("set", 6)
		f:write("earth")
		f:seek("set", 0)
		local s = f:read("*a")
		assert(s == "hello earth", "expected 'hello earth', got " .. tostring(s))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestFileSeekNegativeOffset(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	f, err := os.CreateTemp("", "seek_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("abcdefghij")
	f.Close()

	err = DoString(l, fmt.Sprintf(`
		local f = io.open(%q, "r")
		local pos = f:seek("end", -3)
		assert(pos == 7, "expected pos 7, got " .. tostring(pos))
		local s = f:read("*a")
		assert(s == "hij", "expected 'hij', got " .. tostring(s))
		f:close()
	`, f.Name()))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}
