package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/trace"
)

// runTrace is srwr trace [--root <dir>] [--tape <id>] [--mark] [--json] [<file>]: it reads a text with unified diffs (git show,
// git diff, git log -p; the file, or the standard input) and tells which operation on the tapes wrote each added line.
// It runs nothing and writes nothing.
func runTrace(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("srwr trace", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", lang.Pick("the workspace directory", "作業場のディレクトリ"))
	only := fs.String("tape", "", lang.Pick("look only at this tape", "このテープだけを見る"))
	mark := fs.Bool("mark", false, lang.Pick("print the text as it is, with the why of each hunk after its @@ line", "テキストをそのまま出し、各 hunk の @@ の行の次に理由を足す"))
	asJSON := fs.Bool("json", false, lang.Pick("print JSON", "JSON で出す"))
	if err := fs.Parse(reorder(args)); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr trace: unexpected argument %q\n", "srwr trace: 余分な引数 %q\n"), fs.Arg(1))
		return 2
	}
	if *mark && *asJSON {
		_, _ = fmt.Fprint(stderr, lang.Pick("srwr trace: --mark and --json cannot be used together\n", "srwr trace: --mark と --json は一緒に使えません\n"))
		return 2
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr trace: cannot open the workspace %q as a directory\n", "srwr trace: 作業場 %q がディレクトリとして開けません\n"), *root)
		return 1
	}
	var in []byte
	var err error
	if fs.NArg() == 1 {
		in, err = os.ReadFile(fs.Arg(0))
	} else {
		in, err = io.ReadAll(stdin)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr trace: %v\n", err)
		return 1
	}
	ws, err := session.Open(*root, session.Options{Version: Version(), Now: Now})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr trace: %v\n", err)
		return 1
	}
	ops := trace.Collect(filepath.Dir(ws.TapePath("x")), *only)
	text := string(in)
	commits := trace.ParseDiff(text)
	r := traceResult{ops: ops}
	for _, c := range commits {
		tc := tracedCommit{Commit: c}
		for _, f := range c.Files {
			if f.Path == "" || f.Path == ".srwr" || strings.HasPrefix(f.Path, ".srwr/") {
				continue // a deleted file, and what srwr keeps for itself
			}
			tc.files = append(tc.files, tracedFile{path: f.Path, diff: f, segs: trace.Attribute(ops, f)})
		}
		r.commits = append(r.commits, tc)
	}
	switch {
	case *mark:
		writeMarked(stdout, text, r)
	case *asJSON:
		return writeTraceJSON(stdout, stderr, r)
	default:
		writeTrace(stdout, r, len(ops) == 0)
	}
	return 0
}

type tracedFile struct {
	path string
	diff trace.FileDiff
	segs []trace.Segment
}

type tracedCommit struct {
	trace.Commit
	files []tracedFile
}

type traceResult struct {
	ops     []trace.Op
	commits []tracedCommit
}

// segLabel is "lines 67-69" / "line 5" (in Japanese "67〜69行" / "5行").
func segLabel(from, to int) string {
	if from == to {
		return lang.Pick(fmt.Sprintf("line %d", from), fmt.Sprintf("%d行", from))
	}
	return lang.Pick(fmt.Sprintf("lines %d-%d", from, to), fmt.Sprintf("%d〜%d行", from, to))
}

func noTape(n int) string {
	if lang.Ja() {
		return fmt.Sprintf("テープに無い：追加 %d 行", n)
	}
	if n == 1 {
		return "not on a tape: 1 added line"
	}
	return fmt.Sprintf("not on a tape: %d added lines", n)
}

func opLine(o *trace.Op) string {
	t := ""
	if !o.Time.IsZero() {
		t = o.Time.Local().Format("15:04")
	}
	return fmt.Sprintf("%-4s %s #%d   %s", o.Type, o.Tape, o.Seq, t)
}

func whyLine(o *trace.Op) string {
	if o.Why == "" {
		return lang.Pick("(no why: made through a hook)", "（理由なし：hook の記録）")
	}
	return lang.Pick("why: ", "理由: ") + o.Why
}

func writeTrace(w io.Writer, r traceResult, noTapes bool) {
	if noTapes {
		_, _ = fmt.Fprint(w, lang.Pick("No operation on the tapes of this workspace. Run it in the workspace where the AI worked, or give --root.\n", "この作業場のテープに操作がありません。AI が作業した作業場で実行するか、--root を渡してください。\n"))
	}
	if len(r.commits) == 0 || totalFiles(r) == 0 {
		_, _ = fmt.Fprint(w, lang.Pick("No diff found in the text.\n", "テキストに差分が見つかりません。\n"))
		return
	}
	for _, c := range r.commits {
		if c.SHA != "" {
			_, _ = fmt.Fprintf(w, "%s %s  %s\n", lang.Pick("commit", "コミット"), short(c.SHA), c.Subject)
		}
		var added, fromTape int
		files := 0
		for _, f := range c.files {
			if f.path == "" {
				continue
			}
			files++
			_, _ = fmt.Fprintf(w, "  %s\n", f.path)
			if len(f.segs) == 0 {
				_, _ = fmt.Fprintf(w, "    %s\n", lang.Pick("(no added lines)", "（追加された行はありません）"))
			}
			for _, s := range f.segs {
				n := s.To - s.From + 1
				added += n
				label := padRight(segLabel(s.From, s.To), 14)
				if s.Op == nil {
					_, _ = fmt.Fprintf(w, "    %s (%s)\n", label, noTape(n))
					continue
				}
				fromTape += n
				_, _ = fmt.Fprintf(w, "    %s %s\n      %s\n", label, opLine(s.Op), whyLine(s.Op))
			}
		}
		_, _ = fmt.Fprintf(w, "  %s\n", summary(files, added, fromTape))
	}
}

func summary(files, added, fromTape int) string {
	if lang.Ja() {
		return fmt.Sprintf("%d ファイル、追加 %d 行：テープから %d 行、テープに無い %d 行", files, added, fromTape, added-fromTape)
	}
	f := fmt.Sprintf("%d files", files)
	if files == 1 {
		f = "1 file"
	}
	return fmt.Sprintf("%s, %d added lines: %d from tapes, %d not on a tape", f, added, fromTape, added-fromTape)
}

func totalFiles(r traceResult) int {
	n := 0
	for _, c := range r.commits {
		n += len(c.files)
	}
	return n
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// writeMarked prints the text as it is, and after the @@ line of every hunk the why of the operations that wrote its added lines.
func writeMarked(w io.Writer, text string, r traceResult) {
	after := map[int][]string{} // index of the line of the text -> the lines to put after it
	for _, c := range r.commits {
		for _, f := range c.files {
			for hi, h := range f.diff.Hunks {
				seen := map[*trace.Op]bool{}
				for _, s := range f.segs {
					if s.Hunk != hi || s.Op == nil || seen[s.Op] {
						continue
					}
					seen[s.Op] = true
					why := s.Op.Why
					if why == "" {
						why = lang.Pick("(no why)", "（理由なし）")
					}
					after[h.Line] = append(after[h.Line], fmt.Sprintf("# %s%s  (%s #%d)", lang.Pick("why: ", "理由: "), why, s.Op.Tape, s.Op.Seq))
				}
			}
		}
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if i == len(lines)-1 && l == "" {
			break
		}
		_, _ = fmt.Fprintln(w, l)
		for _, a := range after[i] {
			_, _ = fmt.Fprintln(w, a)
		}
	}
}

type jsonSegment struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Tape string `json:"tape,omitempty"`
	Seq  int    `json:"seq,omitempty"`
	Type string `json:"type,omitempty"`
	Time string `json:"time,omitempty"`
	Why  string `json:"why,omitempty"`
}

type jsonFile struct {
	File     string        `json:"file"`
	Segments []jsonSegment `json:"segments"`
}

type jsonCommit struct {
	Commit  string     `json:"commit,omitempty"`
	Subject string     `json:"subject,omitempty"`
	Files   []jsonFile `json:"files"`
}

func writeTraceJSON(stdout, stderr io.Writer, r traceResult) int {
	out := make([]jsonCommit, 0, len(r.commits))
	for _, c := range r.commits {
		jc := jsonCommit{Commit: c.SHA, Subject: c.Subject, Files: []jsonFile{}}
		for _, f := range c.files {
			if f.path == "" {
				continue
			}
			jf := jsonFile{File: f.path, Segments: []jsonSegment{}}
			for _, s := range f.segs {
				js := jsonSegment{From: s.From, To: s.To}
				if s.Op != nil {
					js.Tape, js.Seq, js.Type, js.Why = s.Op.Tape, s.Op.Seq, s.Op.Type, s.Op.Why
					if !s.Op.Time.IsZero() {
						js.Time = s.Op.Time.UTC().Format("2006-01-02T15:04:05.000Z")
					}
				}
				jf.Segments = append(jf.Segments, js)
			}
			jc.Files = append(jc.Files, jf)
		}
		out = append(out, jc)
	}
	b, err := json.MarshalIndent(map[string]any{"commits": out}, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr trace: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s\n", b)
	return 0
}
