package tape

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestDiffApplies(t *testing.T) {
	cases := []struct{ name, before, after string }{
		{"one line changed", "a\nb\nc\n", "a\nB\nc\n"},
		{"insert", "a\nc\n", "a\nb\nc\n"},
		{"delete", "a\nb\nc\n", "a\nc\n"},
		{"first and last line", "a\nb\nc\n", "A\nb\nC\n"},
		{"two places apart", "1\n2\n3\n4\n5\n6\n7\n", "1\nX\n3\n4\n5\nY\nZ\n"},
		{"from empty", "", "a\nb\n"},
		{"to empty", "a\nb\n", ""},
		{"blank last line", "a\n\n", "a\n\n\n"},
		{"all different", "a\nb\n", "c\nd\ne\n"},
		{"repeated lines", "a\na\na\nb\n", "a\nb\na\na\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hunks, ok := Diff(c.before, c.after)
			if !ok {
				t.Fatal("Diff gave up")
			}
			if got := ApplyHunks(c.before, hunks); got != c.after {
				t.Errorf("ApplyHunks = %q, want %q (hunks %+v)", got, c.after, hunks)
			}
		})
	}
}

func TestDiffHunksAreSmall(t *testing.T) {
	hunks, _ := Diff("1\n2\n3\n4\n5\n6\n7\n", "1\nX\n3\n4\n5\nY\nZ\n")
	want := []Hunk{
		{StartLine: 2, EndLine: 2, NewText: "X", NewStartLine: 2, NewEndLine: 2},
		{StartLine: 6, EndLine: 7, NewText: "Y\nZ", NewStartLine: 6, NewEndLine: 7},
	}
	if !reflect.DeepEqual(hunks, want) {
		t.Errorf("hunks = %+v, want %+v", hunks, want)
	}
	ins, _ := Diff("a\nc\n", "a\nb\nc\n")
	if want := []Hunk{{StartLine: 2, EndLine: 1, NewText: "b", NewStartLine: 2, NewEndLine: 2}}; !reflect.DeepEqual(ins, want) {
		t.Errorf("insertion = %+v, want %+v", ins, want)
	}
	del, _ := Diff("a\nb\nc\n", "a\nc\n")
	if want := []Hunk{{StartLine: 2, EndLine: 2, NewText: "", NewStartLine: 2, NewEndLine: 1}}; !reflect.DeepEqual(del, want) {
		t.Errorf("deletion = %+v, want %+v", del, want)
	}
}

func TestDiffRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // a fixed seed, so the test repeats
	for range 300 {
		a := randomText(rng)
		b := mutate(rng, a)
		hunks, ok := Diff(a, b)
		if !ok {
			t.Fatalf("gave up on %q -> %q", a, b)
		}
		if got := ApplyHunks(a, hunks); got != b {
			t.Fatalf("%q -> %q: got %q (hunks %+v)", a, b, got, hunks)
		}
	}
}

func randomText(rng *rand.Rand) string {
	var lines []string
	for range rng.Intn(12) {
		lines = append(lines, letter(rng))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func mutate(rng *rand.Rand, text string) string {
	lines := Lines(text)
	for range rng.Intn(4) {
		i := rng.Intn(len(lines) + 1)
		switch {
		case i < len(lines) && rng.Intn(2) == 0:
			lines = append(lines[:i], lines[i+1:]...)
		default:
			lines = append(lines[:i], append([]string{letter(rng)}, lines[i:]...)...)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestDiffGivesUpOnTooManyChanges(t *testing.T) {
	var a, b []string
	for i := range maxEdits + 10 {
		a = append(a, "a"+string(rune('0'+i%10)))
		b = append(b, "b"+string(rune('0'+i%10)))
	}
	if _, ok := Diff(strings.Join(a, "\n")+"\n", strings.Join(b, "\n")+"\n"); ok {
		t.Error("Diff did not give up")
	}
}

func TestExternalWithHunksRoundTrips(t *testing.T) {
	e := Event{
		Type: TypeExternal, Seq: 4, TS: "2026-10-04T00:00:00.000Z", File: "a.go", Author: &Author{Kind: "external"},
		DetectedBy: "select", ExpectedSha: "sha256:a", ActualSha: "sha256:b",
		Hunks: []Hunk{{StartLine: 3, EndLine: 3, NewText: "b", NewStartLine: 3, NewEndLine: 3}},
	}
	line, err := Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), `"text"`) || !strings.Contains(string(line), `"hunks":[{"startLine":3,"endLine":3,"newText":"b","newStartLine":3,"newEndLine":3}]`) {
		t.Errorf("line = %s", line)
	}
	got, ok := parseLine(line)
	if !ok || !reflect.DeepEqual(got.Hunks, e.Hunks) || got.Text != nil {
		t.Errorf("parsed %+v (ok %v)", got, ok)
	}
	// The state follows it without a snapshot after it.
	s := Build([]Event{
		{Type: TypeSnapshot, Seq: 1, File: "a.go", Text: Str("1\n2\n3\n")},
		{Type: TypeExternal, Seq: 2, File: "a.go", Hunks: e.Hunks},
	})
	if got := s.Files["a.go"].Text; got != "1\n2\nb\n" {
		t.Errorf("text = %q", got)
	}
}

func letter(rng *rand.Rand) string { return string("abcd"[rng.Intn(4)]) }
