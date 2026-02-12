package lua

import (
	"fmt"
	"os"
	"testing"
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
	OpenLibraries(l)
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Errorf("could not create temp file")
	}
	l.SetStdout(f)

	s := `print("hello")`

	err = DoString(l, s)
	if err != nil {
		t.Errorf("error running test: %s", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}

	res := make([]byte, 10)
	n, err := f.Read(res)
	if err != nil {
		t.Errorf("error reading from stdout file: %s", err)
	}
	if n != 6 {
		t.Errorf("stdout length doesn't match function params: %d, %s", n, res)
	}

}
