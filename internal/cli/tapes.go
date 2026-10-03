package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// tapeRow is what the list shows of one tape.
type tapeRow struct {
	id      string
	started time.Time
	updated time.Time
	events  int
	files   int
	size    int64
}

// runTapes is srwr tapes [new | prune (--keep N | --older-than 30d) | path <id>] [--root <dir>].
func runTapes(args []string, stdout, stderr io.Writer) int {
	// --root may come anywhere; what is left is the verb and its arguments.
	fs := flag.NewFlagSet("srwr tapes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", lang.Pick("the workspace directory", "作業場のディレクトリ"))
	keep := fs.Int("keep", -1, lang.Pick("prune: keep the newest N tapes", "prune: 新しい方から N 本を残す"))
	older := fs.String("older-than", "", lang.Pick("prune: delete tapes older than this (for example 30d, 12h)", "prune: これより古いテープを消す（例：30d、12h）"))
	if err := fs.Parse(reorder(args)); err != nil {
		return 2
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr tapes: cannot open the workspace %q as a directory\n", "srwr tapes: 作業場 %q がディレクトリとして開けません\n"), *root)
		return 1
	}
	ws, err := session.Open(*root, session.Options{Version: Version()})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr tapes: %v\n", err)
		return 1
	}
	rest := fs.Args()
	verb := ""
	if len(rest) > 0 {
		verb, rest = rest[0], rest[1:]
	}
	switch verb {
	case "":
		return noExtra("", rest, stderr, func() int { return tapesList(ws, stdout, stderr) })
	case "new":
		return noExtra("new", rest, stderr, func() int { return tapesNew(ws, stdout, stderr) })
	case "prune":
		return noExtra("prune", rest, stderr, func() int { return tapesPrune(ws, *keep, *older, stdout, stderr) })
	case "path":
		if len(rest) != 1 {
			_, _ = fmt.Fprintln(stderr, lang.Pick("Usage: srwr tapes path <tape ID>", "使い方: srwr tapes path <テープID>"))
			return 2
		}
		return tapesPath(ws, rest[0], stdout, stderr)
	}
	_, _ = fmt.Fprintf(stderr, lang.Pick("srwr tapes: unknown action %q (new, prune, path)\n", "srwr tapes: 知らない操作 %q（new・prune・path）\n"), verb)
	return 2
}

func noExtra(verb string, rest []string, stderr io.Writer, f func() int) int {
	if len(rest) > 0 {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr tapes %s: unexpected argument %q\n", "srwr tapes %s: 余分な引数 %q\n"), verb, rest[0])
		return 2
	}
	return f()
}

// reorder moves the flags in front of the words, so that srwr tapes prune --keep 3 parses with the flag package.
func reorder(args []string) []string {
	var flags, words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) && (a == "--root" || a == "-root" || a == "--keep" || a == "-keep" || a == "--older-than" || a == "-older-than") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		words = append(words, a)
	}
	return append(flags, words...)
}

func tapeRows(ws *session.Workspace) ([]tapeRow, error) {
	dir := filepath.Dir(ws.TapePath("x"))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []tapeRow
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), tape.FileSuffix)
		if !ok || e.IsDir() || !tape.ValidID(id) {
			continue
		}
		row := tapeRow{id: id}
		b, err := os.ReadFile(ws.TapePath(id))
		if err != nil {
			return nil, err
		}
		row.size = int64(len(b))
		res := tape.Parse(b)
		st := tape.Build(res.Events)
		row.files = len(st.Files)
		for _, ev := range res.Events {
			if ev.Type == tape.TypeHeader {
				row.started, _ = time.Parse(time.RFC3339, ev.StartedAt)
				continue
			}
			row.events++
			if t, err := time.Parse(time.RFC3339, ev.TS); err == nil {
				row.updated = t
			}
		}
		if info, err := e.Info(); err == nil && row.updated.IsZero() {
			row.updated = info.ModTime()
		}
		if row.started.IsZero() {
			row.started = row.updated
		}
		rows = append(rows, row)
	}
	// Newest first by the time the tape started. The ID starts with a time too, but older tapes were named in the local zone.
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].started.Equal(rows[j].started) {
			return rows[i].started.After(rows[j].started)
		}
		return rows[i].id > rows[j].id
	})
	return rows, nil
}

func kb(n int64) string { return fmt.Sprintf("%4.1f KB", float64(n)/1024) }

func tapesList(ws *session.Workspace, stdout, stderr io.Writer) int {
	rows, err := tapeRows(ws)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr tapes: %v\n", err)
		return 1
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(stdout, lang.Pick("No tapes.", "テープはありません。"))
		return 0
	}
	cur, _ := ws.Current()
	_, _ = fmt.Fprintln(stdout, "  "+padRight(lang.Pick("Tape", "テープ"), 20)+padRight(lang.Pick("Started", "開始"), timeCol())+padRight(lang.Pick("Last update", "最後の更新"), timeCol())+padRight(lang.Pick("Events", "イベント"), 10)+padRight(lang.Pick("Files", "ファイル"), 10)+lang.Pick("Size", "大きさ"))
	var total int64
	for _, r := range rows {
		total += r.size
		line := "  " + padRight(r.id, 20) + padRight(shortTime(r.started), timeCol()) + padRight(shortTime(r.updated), timeCol()) +
			padRight(strconv.Itoa(r.events), 10) + padRight(strconv.Itoa(r.files), 10) + kb(r.size)
		if r.id == cur {
			line += lang.Pick("  <- current session", "  ← 今のセッション")
		}
		_, _ = fmt.Fprintln(stdout, line)
	}
	_, _ = fmt.Fprintf(stdout, lang.Pick("\n%s (%s in all). Replay one with srwr view <tape>; share one with srwr tapes path <tape>.\n", "\n%s（合計 %s）。再生は srwr view <テープ>、共有は srwr tapes path <テープ>。\n"), count(len(rows)), strings.TrimSpace(kb(total)))
	return 0
}

func tapesNew(ws *session.Workspace, stdout, stderr io.Writer) int {
	if _, err := os.Stat(filepath.Dir(ws.TapePath("x"))); err != nil {
		_, _ = fmt.Fprintln(stdout, lang.Pick("There is no current session.", "今のセッションはありません。"))
		return 0
	}
	id, err := ws.EndSession()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr tapes new: %v\n", err)
		return 1
	}
	if id == "" {
		_, _ = fmt.Fprintln(stdout, lang.Pick("There is no current session.", "今のセッションはありません。"))
		return 0
	}
	_, _ = fmt.Fprintf(stdout, lang.Pick("Closed the current session %s. The next write starts a new tape.\n", "今のセッション %s を閉じました。次の書き込みから、新しいテープになります。\n"), id)
	return 0
}

// parseAge reads 30d or 12h.
func parseAge(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, errAge(s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, errAge(s)
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	}
	return 0, errAge(s)
}

func tapesPrune(ws *session.Workspace, keep int, older string, stdout, stderr io.Writer) int {
	if (keep < 0) == (older == "") {
		_, _ = fmt.Fprintln(stderr, lang.Pick("Usage: srwr tapes prune --keep N or --older-than 30d (exactly one of them)", "使い方: srwr tapes prune --keep N または --older-than 30d（どちらか1つ）"))
		return 2
	}
	var age time.Duration
	if older != "" {
		var err error
		if age, err = parseAge(older); err != nil {
			_, _ = fmt.Fprintf(stderr, "srwr tapes prune: %v\n", err)
			return 2
		}
	}
	if _, err := os.Stat(filepath.Dir(ws.TapePath("x"))); err != nil {
		_, _ = fmt.Fprintln(stdout, lang.Pick("No tapes to delete.", "消すテープはありません。"))
		return 0
	}
	var gone []tapeRow
	var left []tapeRow
	// The lock keeps a writer from starting on a tape that is being deleted.
	err := ws.Do(func(tx *session.Tx) error {
		rows, err := tapeRows(ws)
		if err != nil {
			return err
		}
		cur := ""
		if _, statErr := os.Stat(ws.TapePath(tx.TapeID())); statErr == nil {
			cur = tx.TapeID()
		}
		now := time.Now()
		for i, r := range rows {
			doomed := false
			if keep >= 0 {
				doomed = i >= keep
			} else {
				doomed = now.Sub(r.updated) > age
			}
			if doomed && r.id != cur {
				if err := os.Remove(ws.TapePath(r.id)); err != nil {
					return err
				}
				gone = append(gone, r)
			} else {
				left = append(left, r)
			}
		}
		return nil
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr tapes prune: %v\n", err)
		return 1
	}
	if len(gone) == 0 {
		_, _ = fmt.Fprintln(stdout, lang.Pick("No tapes to delete.", "消すテープはありません。"))
		return 0
	}
	var rest int64
	for _, r := range left {
		rest += r.size
	}
	for _, r := range gone {
		_, _ = fmt.Fprintf(stdout, lang.Pick("Deleted  %s  (%s)\n", "消しました  %s  （%s）\n"), r.id, strings.TrimSpace(kb(r.size)))
	}
	_, _ = fmt.Fprintf(stdout, lang.Pick("Deleted %s. %d left (%s).\n", "%sを消しました。残り %d 本（%s）。\n"), count(len(gone)), len(left), strings.TrimSpace(kb(rest)))
	return 0
}

func tapesPath(ws *session.Workspace, id string, stdout, stderr io.Writer) int {
	id = strings.TrimSuffix(filepath.Base(id), tape.FileSuffix)
	if !tape.ValidID(id) {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr tapes path: %q is not a valid tape ID\n", "srwr tapes path: テープID %q が正しくありません\n"), id)
		return 1
	}
	p := ws.TapePath(id)
	if _, err := os.Stat(p); err != nil {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr tapes path: there is no tape %s\n", "srwr tapes path: テープ %s がありません\n"), id)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, p)
	return 0
}

// errAge is the mistake in --older-than.
func errAge(s string) error {
	return fmt.Errorf(lang.Pick("--older-than %q must look like 30d or 12h", "--older-than %q は 30d や 12h の形で指定してください"), s)
}

// count is "1 tape" or "3 tapes" (in Japanese, "3 本").
func count(n int) string {
	if lang.Ja() {
		return fmt.Sprintf("%d 本", n)
	}
	if n == 1 {
		return "1 tape"
	}
	return fmt.Sprintf("%d tapes", n)
}

// shortTime is a time for the list, in the time zone of the machine (TZ): "Oct 03 17:12", or "10/03 17:12" in Japanese.
func shortTime(t time.Time) string {
	return t.Local().Format(lang.Pick("Jan 02 15:04", "01/02 15:04"))
}

// timeCol is the width of a time column: the English time is a little longer than the Japanese one.
func timeCol() int { return lang.PickInt(14, 13) }
