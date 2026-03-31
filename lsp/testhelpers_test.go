package lsp

// Shared test helpers used by multiple _test.go files.

// offsetOf returns the byte offset of the n-th whole-word occurrence (0-based)
// of word in src, or -1 if not found.
func offsetOf(src, word string, n int) int {
	count := 0
	for i := 0; i <= len(src)-len(word); i++ {
		if src[i:i+len(word)] != word {
			continue
		}
		before := i == 0 || !isWordChar(src[i-1])
		after := i+len(word) >= len(src) || !isWordChar(src[i+len(word)])
		if before && after {
			if count == n {
				return i
			}
			count++
		}
	}
	return -1
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// offsetToPos converts a byte offset to an LSP Position.
func offsetToPos(text string, offset int) Position {
	line := 0
	col := 0
	for i := 0; i < offset && i < len(text); i++ {
		if text[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return Position{Line: line, Character: col}
}
