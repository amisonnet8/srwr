package cli

import (
	"strings"
	"unicode"
)

// cellWidth is how many columns of a terminal s takes: wide characters (Japanese) take two.
func cellWidth(s string) int {
	n := 0
	for _, r := range s {
		if isWide(r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func isWide(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		(r >= 0xFF00 && r <= 0xFF60) || (r >= 0x3000 && r <= 0x303F) ||
		(r >= 0xFFE0 && r <= 0xFFE6)
}

// padRight pads s with spaces to width columns (it is not cut if it is longer).
func padRight(s string, width int) string {
	if d := width - cellWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
