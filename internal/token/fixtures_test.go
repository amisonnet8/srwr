package token

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// The tapes under extension/test/fixtures hold tokens made by the earlier implementation.
// There is no key here, so the HMAC cannot be checked, but everything else must agree with the events.
func TestTokensInRealTapes(t *testing.T) {
	root := filepath.Join("..", "..", "extension", "test", "fixtures")
	a, _ := filepath.Glob(filepath.Join(root, "*.tape.jsonl"))
	b, _ := filepath.Glob(filepath.Join(root, "ui-check", ".srwr", "tapes", "*.tape.jsonl"))
	paths := append(a, b...)

	tokens := 0
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // a fixture path found by Glob
		if err != nil {
			t.Fatal(err)
		}
		st := tape.NewState()
		for _, e := range tape.Parse(data).Events {
			st.Apply(e)
			if e.Selection == nil || (e.Type != tape.TypeLook && e.Type != tape.TypeEdit) {
				continue
			}
			start, end := e.StartLine, e.EndLine
			if e.Type == tape.TypeEdit {
				start, end = e.NewStartLine, e.NewEndLine
			}
			got, _, mac, err := decodeBody(*e.Selection)
			if err != nil {
				t.Errorf("%s seq %d: %v", filepath.Base(path), e.Seq, err)
				continue
			}
			tokens++
			want := Token{
				Seq:       u64(e.Seq),
				StartLine: u64(start),
				EndLine:   u64(end),
				FileHash:  Hash4(e.File),
				TextHash:  Hash4(tape.RangeText(st.Files[e.File].Text, start, end)),
			}
			if got != want || len(mac) != macLen {
				t.Errorf("%s seq %d: token = %+v, want %+v", filepath.Base(path), e.Seq, got, want)
			}
		}
	}
	if tokens < 16 {
		t.Errorf("checked %d tokens, want at least 16", tokens)
	}
}

func u64(n int) uint64 {
	return uint64(n) //nolint:gosec // line numbers and seq in the fixtures are positive
}
