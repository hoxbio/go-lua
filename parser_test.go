package lua

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
)

func load(l *State, t *testing.T, fileName string) *luaClosure {
	if err := LoadFile(l, fileName, "bt"); err != nil {
		return nil
	}
	return l.ToValue(-1).(*luaClosure)
}

func TestParser(t *testing.T) {
	l := NewState()
	OpenLibraries(l)
	bin := load(l, t, "fixtures/fib.bin")
	l.Pop(1)
	closure := load(l, t, "fixtures/fib.lua")
	p := closure.prototype
	if p == nil {
		t.Fatal("prototype was nil")
	}
	validate("@fixtures/fib.lua", p.source, "as source file name", t)
	if !p.isVarArg {
		t.Error("expected main function to be var arg, but wasn't")
	}
	if len(closure.upValues) != len(closure.prototype.upValues) {
		t.Error("upvalue count doesn't match", len(closure.upValues), "!=", len(closure.prototype.upValues))
	}
	compareClosures(t, bin, closure)
	l.Call(0, 0)
}

func TestEmptyString(t *testing.T) {
	l := NewState()
	if err := LoadString(l, ""); err != nil {
		t.Fatal(err.Error())
	}
	l.Call(0, 0)
}

func TestParserExhaustively(t *testing.T) {
	_, err := exec.LookPath("luac")
	if err != nil {
		t.Skipf("exhaustively testing the parser requires luac: %s", err)
	}
	l := NewState()
	matches, err := filepath.Glob(filepath.Join("lua-tests", "*.lua"))
	if err != nil {
		t.Fatal(err)
	}
	blackList := map[string]bool{"math.lua": true}
	for _, source := range matches {
		if _, ok := blackList[filepath.Base(source)]; ok {
			continue
		}
		protectedTestParser(l, t, source)
	}
}

func protectedTestParser(l *State, t *testing.T, source string) {
	defer func() {
		if x := recover(); x != nil {
			t.Error(x)
			t.Log(string(debug.Stack()))
		}
	}()
	t.Log("Compiling " + source)
	binary := strings.TrimSuffix(source, ".lua") + ".bin"
	if err := exec.Command("luac", "-o", binary, source).Run(); err != nil {
		t.Fatalf("luac failed to compile %s: %s", source, err)
	}
	t.Log("Parsing " + source)
	bin := load(l, t, binary)
	l.Pop(1)
	src := load(l, t, source)
	l.Pop(1)
	t.Log(source)
	compareClosures(t, src, bin)
}

func expectEqual(t *testing.T, x, y interface{}, m string) {
	if x != y {
		t.Errorf("%s doesn't match: %v, %v\n", m, x, y)
	}
}

func expectDeepEqual(t *testing.T, x, y interface{}, m string) bool {
	if reflect.DeepEqual(x, y) {
		return true
	}
	if reflect.TypeOf(x).Kind() == reflect.Slice && reflect.ValueOf(y).Len() == 0 && reflect.ValueOf(x).Len() == 0 {
		return true
	}
	t.Errorf("%s doesn't match: %v, %v\n", m, x, y)
	return false
}

func compareClosures(t *testing.T, a, b *luaClosure) {
	expectEqual(t, a.upValueCount(), b.upValueCount(), "upvalue count")
	comparePrototypes(t, a.prototype, b.prototype)
}

func comparePrototypes(t *testing.T, a, b *prototype) {
	expectEqual(t, a.isVarArg, b.isVarArg, "var arg")
	expectEqual(t, a.lineDefined, b.lineDefined, "line defined")
	expectEqual(t, a.lastLineDefined, b.lastLineDefined, "last line defined")
	expectEqual(t, a.parameterCount, b.parameterCount, "parameter count")
	expectEqual(t, a.maxStackSize, b.maxStackSize, "max stack size")
	expectEqual(t, a.source, b.source, "source")
	expectEqual(t, len(a.code), len(b.code), "code length")
	if !expectDeepEqual(t, a.code, b.code, "code") {
		for i := range a.code {
			if a.code[i] != b.code[i] {
				t.Errorf("%d: %v != %v\n", a.lineInfo[i], a.code[i], b.code[i])
			}
		}
		for _, i := range []int{3, 197, 198, 199, 200, 201} {
			t.Errorf("%d: %#v, %#v\n", i, a.constants[i], b.constants[i])
		}
		for _, i := range []int{202, 203, 204} {
			t.Errorf("%d: %#v\n", i, b.constants[i])
		}
	}
	if !expectDeepEqual(t, a.constants, b.constants, "constants") {
		for i := range a.constants {
			if a.constants[i] != b.constants[i] {
				t.Errorf("%d: %#v != %#v\n", i, a.constants[i], b.constants[i])
			}
		}
	}
	expectDeepEqual(t, a.lineInfo, b.lineInfo, "line info")
	expectDeepEqual(t, a.upValues, b.upValues, "upvalues")
	expectDeepEqual(t, a.localVariables, b.localVariables, "local variables")
	expectEqual(t, len(a.prototypes), len(b.prototypes), "prototypes length")
	for i := range a.prototypes {
		comparePrototypes(t, &a.prototypes[i], &b.prototypes[i])
	}
}

func TestStatementLines(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		lines []int
	}{
		{
			name:  "single assignment",
			code:  `x = 10`,
			lines: []int{1},
		},
		{
			name:  "two assignments",
			code:  "x = 10\ny = 20",
			lines: []int{1, 2},
		},
		{
			name:  "three assignments",
			code:  "x = 10\ny = 20\nz = x + y",
			lines: []int{1, 2, 3},
		},
		{
			name:  "function call",
			code:  "x = 10\nprint(x)",
			lines: []int{1, 2},
		},
		{
			name:  "multiline function",
			code:  "function greet(name)\n  return \"Hello, \" .. name\nend",
			lines: []int{1},
		},
		{
			name:  "function then call",
			code:  "function greet(name)\n  return \"Hello, \" .. name\nend\ngreet(\"World\")",
			lines: []int{1, 4},
		},
		{
			name:  "if block",
			code:  "x = 10\nif x > 5 then\n  print(x)\nend\ny = 20",
			lines: []int{1, 2, 5},
		},
		{
			name:  "if else block",
			code:  "if true then\n  x = 1\nelse\n  x = 2\nend",
			lines: []int{1},
		},
		{
			name:  "for numeric",
			code:  "for i = 1, 10 do\n  print(i)\nend",
			lines: []int{1},
		},
		{
			name:  "for in",
			code:  "t = {1,2,3}\nfor k, v in pairs(t) do\n  print(k, v)\nend",
			lines: []int{1, 2},
		},
		{
			name:  "while loop",
			code:  "x = 0\nwhile x < 10 do\n  x = x + 1\nend\nprint(x)",
			lines: []int{1, 2, 5},
		},
		{
			name:  "repeat until",
			code:  "x = 0\nrepeat\n  x = x + 1\nuntil x > 10",
			lines: []int{1, 2},
		},
		{
			name:  "local variable",
			code:  "local x = 10\nlocal y = 20\nprint(x + y)",
			lines: []int{1, 2, 3},
		},
		{
			name:  "local function",
			code:  "local function foo()\n  return 42\nend\nfoo()",
			lines: []int{1, 4},
		},
		{
			name:  "nested blocks",
			code:  "function outer()\n  function inner()\n    return 1\n  end\n  return inner()\nend\nouter()",
			lines: []int{1, 7},
		},
		{
			name:  "semicolons",
			code:  "x = 1; y = 2; z = 3",
			lines: []int{1, 1, 1},
		},
		{
			name:  "return statement",
			code:  "x = 10\nreturn x",
			lines: []int{1, 2},
		},
		{
			name:  "do block",
			code:  "x = 1\ndo\n  local y = 2\n  x = y\nend\nprint(x)",
			lines: []int{1, 2, 6},
		},
		{
			name:  "empty input",
			code:  "",
			lines: []int{},
		},
		{
			name: "complex program",
			code: `x = 10
y = 20
function add(a, b)
  return a + b
end
result = add(x, y)
if result > 25 then
  print("big")
else
  print("small")
end
print("done")`,
			lines: []int{1, 2, 3, 6, 7, 12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewState()
			OpenLibraries(l)

			got, err := l.StatementLines(tt.code)
			if err != nil {
				t.Fatalf("StatementLines(%q) error: %v", tt.code, err)
			}
			if len(got) == 0 {
				got = []int{}
			}
			if !reflect.DeepEqual(got, tt.lines) {
				t.Errorf("StatementLines(%q)\n  got  %v\n  want %v", tt.code, got, tt.lines)
			}
		})
	}
}

func TestStatementLinesSyntaxError(t *testing.T) {
	l := NewState()
	OpenLibraries(l)

	_, err := l.StatementLines("if then end end")
	if err == nil {
		t.Error("expected syntax error, got nil")
	}
}

func TestStatementLinesSyntaxErrorNoPanic(t *testing.T) {
	// This test specifically checks that syntax errors don't cause panics
	// due to uninitialized errorFunction. Before the fix, this would panic
	// with "index out of range [-4]" in setErrorObject.
	tests := []struct {
		name          string
		code          string
		errorContains string
	}{
		{
			name:          "standalone expression",
			code:          "x = 10\ny = 20\nx + y",
			errorContains: "syntax error",
		},
		{
			name:          "standalone expression with function",
			code:          "x = 10\ny = 20\nx + y\n\nfunction greet(name)\n  return \"Hello, \" .. name\nend\n\ngreet(\"World\")",
			errorContains: "syntax error",
		},
		{
			name:          "malformed if - missing condition",
			code:          "if then x = 1 end",
			errorContains: "expected",
		},
		{
			name:          "incomplete function",
			code:          "function foo()\n  x = 1",
			errorContains: "'end' expected",
		},
		{
			name:          "invalid assignment target",
			code:          "(x + y) = 10",
			errorContains: "syntax error",
		},
		{
			name:          "unclosed parenthesis",
			code:          "x = (10 + 20",
			errorContains: "expected",
		},
		{
			name:          "unexpected symbol",
			code:          "x = 10\n@@@",
			errorContains: "unexpected symbol",
		},
		{
			name:          "unfinished string",
			code:          "x = \"hello\ny = 20",
			errorContains: "unfinished string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test ensures we get a proper error, not a panic
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("StatementLines panicked: %v", r)
				}
			}()

			l := NewState()
			OpenLibraries(l)
			
			lines, err := l.StatementLines(tt.code)
			
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (lines: %v)", tt.errorContains, lines)
			}
			
			errMsg := err.Error()
			if !strings.Contains(errMsg, tt.errorContains) {
				t.Errorf("expected error containing %q, got %q", tt.errorContains, errMsg)
			}
		})
	}
}

func BenchmarkStatementLines(b *testing.B) {
	benchmarks := []struct {
		name string
		code string
	}{
		{
			name: "5_simple",
			code: "x = 10\ny = 20\nz = x + y\nprint(z)\nreturn z",
		},
		{
			name: "10_mixed",
			code: `x = 10
y = 20
function add(a, b)
  return a + b
end
result = add(x, y)
if result > 25 then
  print("big")
else
  print("small")
end
print("done")`,
		},
		{
			name: "25_lines",
			code: `local x = 0
local y = 0
local t = {}
for i = 1, 10 do
  t[i] = i * i
end
function sum(tbl)
  local s = 0
  for _, v in pairs(tbl) do
    s = s + v
  end
  return s
end
x = sum(t)
if x > 100 then
  y = x - 100
elseif x > 50 then
  y = x - 50
else
  y = x
end
while y > 0 do
  y = y - 1
end
print(x, y)
return x`,
		},
		{
			name: "50_lines",
			code: `local a = 1
local b = 2
local c = 3
local d = 4
local e = 5
local t = {}
for i = 1, 20 do
  t[i] = i
end
function map(tbl, f)
  local r = {}
  for k, v in pairs(tbl) do
    r[k] = f(v)
  end
  return r
end
function filter(tbl, f)
  local r = {}
  for _, v in pairs(tbl) do
    if f(v) then
      r[#r+1] = v
    end
  end
  return r
end
function reduce(tbl, f, init)
  local acc = init
  for _, v in pairs(tbl) do
    acc = f(acc, v)
  end
  return acc
end
local doubled = map(t, function(x) return x * 2 end)
local evens = filter(doubled, function(x) return x % 2 == 0 end)
local total = reduce(evens, function(a, b) return a + b end, 0)
if total > 100 then
  print("large")
elseif total > 50 then
  print("medium")
else
  print("small")
end
repeat
  total = total - 1
until total <= 0
while a < 100 do
  a = a + b
  b = b + 1
end
c = a + b
d = c * 2
e = d - 1
print(a, b, c, d, e, total)
return total`,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			l := NewState()
			OpenLibraries(l)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = l.StatementLines(bm.code)
			}
		})
	}
}
