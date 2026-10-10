package trace

import (
	"reflect"
	"testing"
	"time"
)

const showText = `commit 3f9a1c2d4e5f60718293a4b5c6d7e8f901234567
Author: x <x@example.com>
Date:   Sat Oct 10 10:21:00 2026 +0900

    Fix the same-day check

    A body line.

diff --git a/internal/cli/render.go b/internal/cli/render.go
index 111..222 100644
--- a/internal/cli/render.go
+++ b/internal/cli/render.go
@@ -64,7 +64,9 @@ func formatTime(
 	local, today := t.In(loc), now.In(loc)
-	if local.Year() == today.Year() && local.Day() == today.Day() {
+	ly, lm, ld := local.Date()
+	ty, tm, td := today.Date()
+	if ly == ty && lm == tm && ld == td {
 		return local.Format("15:04")
 	}
+	// added alone
 	return local.Format("2006-01-02")
diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,2 @@
+package a
+
commit 0123456789abcdef0123456789abcdef01234567
Author: x <x@example.com>

    Second

diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,1 +0,0 @@
-package old
`

func TestParseDiff(t *testing.T) {
	cs := ParseDiff(showText)
	if len(cs) != 2 {
		t.Fatalf("commits = %d", len(cs))
	}
	if cs[0].SHA != "3f9a1c2d4e5f60718293a4b5c6d7e8f901234567" || cs[0].Subject != "Fix the same-day check" {
		t.Errorf("commit 0 = %q %q", cs[0].SHA, cs[0].Subject)
	}
	if len(cs[0].Files) != 2 || cs[0].Files[0].Path != "internal/cli/render.go" || cs[0].Files[1].Path != "new.go" {
		t.Fatalf("files = %+v", cs[0].Files)
	}
	h := cs[0].Files[0].Hunks[0]
	if h.NewStart != 64 || len(h.Runs) != 2 {
		t.Fatalf("hunk = %+v", h)
	}
	// the first run: 3 added lines after 1 context line and 1 removed line (new lines 65.. since the context is line 64)
	if h.Runs[0].Start != 65 || len(h.Runs[0].Lines) != 3 || h.Runs[0].Lines[0] != "\tly, lm, ld := local.Date()" {
		t.Errorf("run 0 = %+v", h.Runs[0])
	}
	if h.Runs[1].Start != 70 || !reflect.DeepEqual(h.Runs[1].Lines, []string{"\t// added alone"}) {
		t.Errorf("run 1 = %+v", h.Runs[1])
	}
	nf := cs[0].Files[1].Hunks[0]
	if nf.NewStart != 1 || len(nf.Runs) != 1 || nf.Runs[0].Start != 1 || len(nf.Runs[0].Lines) != 2 {
		t.Errorf("new file hunk = %+v", nf)
	}
	if cs[1].Subject != "Second" || len(cs[1].Files) != 1 || cs[1].Files[0].Path != "" {
		t.Errorf("commit 1 = %+v", cs[1])
	}
}

func TestParseDiffPlainAndNoDiff(t *testing.T) {
	plain := "--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,3 @@\n a\n+b\n c\n"
	cs := ParseDiff(plain)
	if len(cs) != 1 || cs[0].SHA != "" || len(cs[0].Files) != 1 || cs[0].Files[0].Path != "x.go" {
		t.Fatalf("plain = %+v", cs)
	}
	if r := cs[0].Files[0].Hunks[0].Runs; len(r) != 1 || r[0].Start != 2 || r[0].Lines[0] != "b" {
		t.Errorf("runs = %+v", r)
	}
	if cs := ParseDiff("hello\nworld\n"); len(cs) != 0 {
		t.Errorf("no diff: %+v", cs)
	}
}

func op(tape string, seq int, file, why string, at int, lines ...string) Op {
	return Op{Tape: tape, Seq: seq, Type: "edit", File: file, Why: why, Time: time.Date(2026, 10, 10, 10, at, 0, 0, time.UTC), Lines: lines}
}

func whyOf(s Segment) string {
	if s.Op == nil {
		return "-"
	}
	return s.Op.Why
}

func TestAttribute(t *testing.T) {
	f := FileDiff{Path: "a.go", Hunks: []Hunk{{Runs: []Run{
		{Start: 10, Lines: []string{"x := 1", "y := 2", "z := 3"}},
		{Start: 20, Lines: []string{"}"}},
		{Start: 30, Lines: []string{"if verylongcondition == other {"}},
		{Start: 40, Lines: []string{"hand written", "also by hand"}},
	}}}}
	ops := []Op{
		op("t1", 1, "a.go", "old", 1, "x := 1", "y := 2"),
		op("t2", 4, "a.go", "new", 5, "x := 1", "y := 2"), // the same lines, later
		op("t2", 5, "a.go", "tail", 6, "z := 3", "}"),
		op("t3", 1, "b.go", "other file", 7, "hand written", "also by hand"),
		op("t2", 6, "a.go", "one long line", 8, "if verylongcondition == other {"),
	}
	got := Attribute(ops, f)
	if len(got) == 0 {
		t.Fatal("no segments")
	}
	// Check the parts that matter, by line.
	by := map[int]string{}
	for _, s := range got {
		for l := s.From; l <= s.To; l++ {
			by[l] = whyOf(s)
		}
	}
	if by[10] != "new" || by[11] != "new" {
		t.Errorf("x, y: %v (a later operation wins)", by)
	}
	if by[12] != "-" {
		t.Errorf("z alone is too short to be believed: %v", by)
	}
	if by[20] != "-" {
		t.Errorf("a lone } is not believed: %v", by)
	}
	if by[30] != "one long line" {
		t.Errorf("a long single line is believed: %v", by)
	}
	if by[40] != "-" || by[41] != "-" {
		t.Errorf("another file's operation must not match: %v", by)
	}
}

func TestAttributeSplitsARun(t *testing.T) {
	f := FileDiff{Path: "a.go", Hunks: []Hunk{{Runs: []Run{{Start: 1, Lines: []string{"first line of A", "second line of A", "by hand", "first line of B", "second line of B"}}}}}}
	ops := []Op{
		op("t", 1, "a.go", "A", 1, "first line of A", "second line of A"),
		op("t", 2, "a.go", "B", 2, "first line of B", "second line of B"),
	}
	got := Attribute(ops, f)
	if len(got) != 3 || whyOf(got[0]) != "A" || got[0].From != 1 || got[0].To != 2 ||
		whyOf(got[1]) != "-" || got[1].From != 3 || got[1].To != 3 ||
		whyOf(got[2]) != "B" || got[2].From != 4 || got[2].To != 5 {
		t.Errorf("segments = %+v", got)
	}
}
