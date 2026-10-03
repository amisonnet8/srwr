// Package lang picks the language of what srwr shows to a person: English unless SRWR_LANG starts with "ja".
// The text for the AI (MCP tools and errors), hook notes, and the view server's errors stay in English.
package lang

import (
	"fmt"
	"os"
	"strings"
)

// Ja reports whether the person asked for Japanese. It reads the environment on every call, so a test can change it.
func Ja() bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(os.Getenv("SRWR_LANG"))), "ja")
}

// Pick returns ja when Japanese was asked for, en otherwise.
func Pick(en, ja string) string {
	if Ja() {
		return ja
	}
	return en
}

// Sprintf formats the English or the Japanese text with the same arguments.
func Sprintf(en, ja string, args ...any) string {
	return fmt.Sprintf(Pick(en, ja), args...)
}

// PickInt returns ja when Japanese was asked for, en otherwise (for a width, and the like).
func PickInt(en, ja int) int {
	if Ja() {
		return ja
	}
	return en
}
