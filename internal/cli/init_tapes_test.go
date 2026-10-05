package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/setup"
	"github.com/amisonnet8/srwr/internal/tape"
)

func initOut(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	opts := setup.Options{
		Now:      func() time.Time { return time.Date(2026, 10, 3, 17, 12, 4, 0, time.Local) },
		LookPath: func(string) (string, error) { return "/usr/bin/srwr", nil },
	}
	code := initWith(append(args, "--root", root), &out, &errOut, opts)
	return code, strings.ReplaceAll(out.String(), root, "/home/me/project"), strings.ReplaceAll(errOut.String(), root, "/home/me/project")
}

func gitDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

// The outputs the person approved at the UI gate of R9.
func TestInitOutputs(t *testing.T) {
	empty := `作業場：/home/me/project

  作った   .srwr/                鍵 .srwr/key を作りました
  作った   .mcp.json             srwr mcp を登録しました
  作った   .claude/settings.json hook を登録し、Edit・Write などを禁止しました（厳格モード）
  作った   .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

準備できました。Claude Code を開き直すと、look / edit / replace / new が使えます。
緩いモード（Edit・Write を禁止しない）にするときは、srwr init --lenient。
`
	root := gitDir(t)
	if code, out, _ := initOut(t, root); code != 0 || out != empty {
		t.Errorf("empty workspace: code %d\n%s", code, out)
	}
	again := `作業場：/home/me/project

  変更なし .srwr/
  変更なし .mcp.json
  変更なし .claude/settings.json
  変更なし .gitignore

すでに準備できています。何も書き換えませんでした。
`
	if code, out, _ := initOut(t, root); code != 0 || out != again {
		t.Errorf("second run: code %d\n%s", code, out)
	}
	lenient := `作業場：/home/me/project

  変更なし .srwr/
  変更なし .mcp.json
  追記した .claude/settings.json Edit・Write などの禁止を外しました（緩いモード）
  変更なし .gitignore

書き換える前の内容は .srwr/init-backup/20261003-171204/ に残しました。
緩いモードでは、Edit は replace（理由なし）として記録され、Write は external として見えます。
`
	if code, out, _ := initOut(t, root, "--lenient"); code != 0 || out != lenient {
		t.Errorf("lenient: code %d\n%s", code, out)
	}

	existing := gitDir(t)
	write(t, existing, ".mcp.json", `{"mcpServers":{"a":{},"b":{}}}`)
	write(t, existing, ".claude/settings.json", `{"model":"x"}`)
	wantExisting := `作業場：/home/me/project

  作った   .srwr/                鍵 .srwr/key を作りました
  追記した .mcp.json             srwr mcp を登録しました（ほかのサーバー 2 件はそのまま）
  追記した .claude/settings.json hook と禁止を足しました（既存の設定はそのまま）
  作った   .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

書き換える前の内容は .srwr/init-backup/20261003-171204/ に残しました。
準備できました。Claude Code を開き直すと、look / edit / replace / new が使えます。
`
	if code, out, _ := initOut(t, existing); code != 0 || out != wantExisting {
		t.Errorf("existing: code %d\n%s", code, out)
	}

	broken := gitDir(t)
	write(t, broken, ".claude/settings.json", "{\n \"a\":1\n \"b\":2}")
	code, out, errOut := initOut(t, broken)
	wantErr := "srwr init: .claude/settings.json を JSON として読めません（3 行目付近）\n何も書き換えていません。直してから、もう一度実行してください。\n"
	if code != 1 || out != "" || errOut != wantErr {
		t.Errorf("broken: code %d out %q err %q", code, out, errOut)
	}
}

func TestInitWarnsWhenSrwrIsNotOnPath(t *testing.T) {
	var out, errOut bytes.Buffer
	root := t.TempDir()
	code := initWith([]string{"--root", root}, &out, &errOut, setup.Options{LookPath: func(string) (string, error) { return "", errors.New("no") }})
	if code != 0 || !strings.Contains(out.String(), "注意：srwr が PATH にありません") {
		t.Errorf("code %d\n%s", code, out.String())
	}
}

// makeTape makes a tape the way srwr mcp does, as if the clock said at.
func makeTape(t *testing.T, root string, at time.Time, file string) {
	t.Helper()
	ws, err := session.Open(root, session.Options{Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.Core{WS: ws}
	if _, cerr := c.Look(core.LookInput{File: file, StartLine: 1, EndLine: 1, Why: "見る"}); cerr != nil {
		t.Fatal(cerr)
	}
}

func tapesOut(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	code, out, errOut := run(append([]string{"tapes"}, append(args, "--root", root)...), "")
	return code, out, errOut
}

func TestTapesList(t *testing.T) {
	root := t.TempDir()
	if code, out, _ := tapesOut(t, root); code != 0 || out != "テープはありません。\n" {
		t.Errorf("no tapes: %d %q", code, out)
	}
	write(t, root, "a.txt", "1\n2\n")
	write(t, root, "b.txt", "x\n")
	makeTape(t, root, time.Now().Add(-48*time.Hour), "a.txt")
	makeTape(t, root, time.Now().Add(-24*time.Hour), "b.txt")
	makeTape(t, root, time.Now(), "a.txt") // the current session
	code, out, _ := tapesOut(t, root)
	if code != 0 {
		t.Fatal(code, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("lines = %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "  テープ") || !strings.Contains(lines[0], "大きさ") {
		t.Errorf("header = %q", lines[0])
	}
	row := regexp.MustCompile(`^  \d{8}-\d{4}-[0-9a-z]{4}\s+\d\d/\d\d \d\d:\d\d\s+\d\d/\d\d \d\d:\d\d\s+\d+\s+\d+\s+ ?\d+\.\d KB`)
	for i := 1; i <= 3; i++ {
		if !row.MatchString(lines[i]) {
			t.Errorf("row %d = %q", i, lines[i])
		}
	}
	if !strings.HasSuffix(lines[1], "  ← 今のセッション") || strings.Contains(lines[2], "今のセッション") || strings.Contains(lines[3], "今のセッション") {
		t.Errorf("only the newest tape is the current session:\n%s", out)
	}
	if lines[1][2:22] <= lines[2][2:22] || lines[2][2:22] <= lines[3][2:22] {
		t.Errorf("not newest first:\n%s", out)
	}
	if !strings.HasPrefix(lines[5], "3 本（合計 ") || !strings.Contains(lines[5], "srwr tapes path <テープ>") {
		t.Errorf("summary = %q", lines[5])
	}
	// The columns line up even with the Japanese header.
	for _, i := range []int{0, 1} {
		if got := cellWidth(lines[i][:strings.Index(lines[i], "開始")+len("開始")]); i == 0 && got != 2+20+4 {
			t.Errorf("header column = %d", got)
		}
	}
}

func TestTapesNewEndsTheSession(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	if code, out, _ := tapesOut(t, root, "new"); code != 0 || out != "今のセッションはありません。\n" {
		t.Errorf("no session: %d %q", code, out)
	}
	ws, err := session.Open(root, session.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.Core{WS: ws}
	first, cerr := c.Look(core.LookInput{File: "a.txt", StartLine: 1, EndLine: 1, Why: "見る"})
	if cerr != nil {
		t.Fatal(cerr)
	}
	id, _ := ws.Current()
	code, out, _ := tapesOut(t, root, "new")
	if code != 0 || out != "今のセッション "+id+" を閉じました。次の書き込みから、新しいテープになります。\n" {
		t.Errorf("new: %d %q", code, out)
	}
	// The next write is a new tape, and the token of the old one stops working.
	ws2, _ := session.Open(root, session.Options{})
	c2 := &core.Core{WS: ws2}
	if _, cerr := c2.Look(core.LookInput{File: "a.txt", StartLine: 1, EndLine: 1, Why: "見る"}); cerr != nil {
		t.Fatal(cerr)
	}
	if id2, _ := ws2.Current(); id2 == id || id2 == "" {
		t.Errorf("still on tape %q (was %q)", id2, id)
	}
	_, rerr := c2.Edit(core.EditInput{Selection: first.Selection, NewText: "x", Why: "w"})
	if rerr == nil || rerr.Code != core.CodeInvalidSelection {
		t.Errorf("old token: %v", rerr)
	}
	if got := tapeCount(t, root); got != 2 {
		t.Errorf("tapes = %d, want 2", got)
	}
}

func tapeCount(t *testing.T, root string) int {
	t.Helper()
	return len(tape.IDs(filepath.Join(root, ".srwr", "tapes")))
}

func TestTapesPrune(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	for _, ago := range []time.Duration{50 * 24 * time.Hour, 40 * 24 * time.Hour, 5 * 24 * time.Hour, 0} {
		makeTape(t, root, time.Now().Add(-ago), "a.txt")
	}
	if code, _, errOut := tapesOut(t, root, "prune"); code != 2 || !strings.Contains(errOut, "どちらか1つ") {
		t.Errorf("no option: %d %q", code, errOut)
	}
	if code, _, _ := tapesOut(t, root, "prune", "--keep", "1", "--older-than", "3d"); code != 2 {
		t.Errorf("both options: %d", code)
	}
	if code, _, errOut := tapesOut(t, root, "prune", "--older-than", "3x"); code != 2 || !strings.Contains(errOut, "30d や 12h") {
		t.Errorf("bad age: %d %q", code, errOut)
	}
	if got := tapeCount(t, root); got != 4 {
		t.Fatalf("a rejected prune deleted tapes: %d", got)
	}

	code, out, _ := tapesOut(t, root, "prune", "--older-than", "30d")
	if code != 0 || strings.Count(out, "消しました  ") != 2 || !strings.Contains(out, "2 本を消しました。残り 2 本（") {
		t.Errorf("older-than: %d\n%s", code, out)
	}
	if got := tapeCount(t, root); got != 2 {
		t.Errorf("tapes = %d, want 2", got)
	}
	if _, out, _ := tapesOut(t, root, "prune", "--older-than", "30d"); out != "消すテープはありません。\n" {
		t.Errorf("nothing to prune: %q", out)
	}
	// --keep 0 deletes all but the current session.
	_, out, _ = tapesOut(t, root, "prune", "--keep", "0")
	if strings.Count(out, "消しました  ") != 1 || !strings.Contains(out, "1 本を消しました。残り 1 本（") {
		t.Errorf("keep 0:\n%s", out)
	}
	ws, _ := session.Open(root, session.Options{})
	if _, ok := ws.Current(); !ok || tapeCount(t, root) != 1 {
		t.Error("the current session's tape was deleted")
	}
	// --keep 1 with only the current one left: nothing to delete.
	if _, out, _ := tapesOut(t, root, "prune", "--keep", "1"); out != "消すテープはありません。\n" {
		t.Errorf("keep 1: %q", out)
	}
}

func TestTapesPath(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	makeTape(t, root, time.Now(), "a.txt")
	ws, _ := session.Open(root, session.Options{})
	id, _ := ws.Current()
	code, out, _ := tapesOut(t, root, "path", id)
	abs, _ := filepath.Abs(root)
	if code != 0 || out != filepath.Join(abs, ".srwr", "tapes", id+".tape.jsonl")+"\n" {
		t.Errorf("path: %d %q", code, out)
	}
	for _, bad := range []string{"../../etc/passwd", "nope", "20200101-0000-none"} {
		if code, out, _ := tapesOut(t, root, "path", bad); code != 1 || out != "" {
			t.Errorf("path %q: %d %q", bad, code, out)
		}
	}
	if code, _, _ := tapesOut(t, root, "path"); code != 2 {
		t.Errorf("path without an id: %d", code)
	}
}

func TestTapesPruneKeepCountsTheNewestIncludingTheCurrent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	for _, ago := range []time.Duration{72 * time.Hour, 48 * time.Hour, 24 * time.Hour, 0} {
		makeTape(t, root, time.Now().Add(-ago), "a.txt")
	}
	_, out, _ := tapesOut(t, root, "prune", "--keep", "2")
	if strings.Count(out, "消しました  ") != 2 || !strings.Contains(out, "2 本を消しました。残り 2 本（") {
		t.Errorf("keep 2:\n%s", out)
	}
	if got := tapeCount(t, root); got != 2 {
		t.Errorf("tapes = %d, want 2", got)
	}
}

// Making the tape of the session before it is what closes it: it is a compressed file from then on, and every command that
// reads tapes opens it all the same.
func TestTapesCommandsOpenClosedTapes(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n2\n")
	makeTape(t, root, time.Now().Add(-48*time.Hour), "a.txt")
	makeTape(t, root, time.Now().Add(-24*time.Hour), "a.txt")
	makeTape(t, root, time.Now(), "a.txt")
	dir := filepath.Join(root, ".srwr", "tapes")
	ids := tape.IDs(dir)
	if len(ids) != 3 {
		t.Fatalf("tapes = %v", ids)
	}
	oldest := ids[0]
	gz := filepath.Join(dir, oldest+".tape.jsonl.gz")
	if _, err := os.Stat(gz); err != nil {
		t.Fatalf("the oldest tape is not closed: %v", err)
	}

	// list: the closed tape is there, with its size on disk (smaller than the tape it holds).
	code, out, _ := tapesOut(t, root)
	if code != 0 || !strings.Contains(out, oldest) {
		t.Fatalf("list: %d %q", code, out)
	}
	// path: by ID and by the name of the file.
	for _, arg := range []string{oldest, oldest + ".tape.jsonl.gz"} {
		if code, out, _ := tapesOut(t, root, "path", arg); code != 0 || out != gz+"\n" {
			t.Errorf("path %s: %d %q", arg, code, out)
		}
	}
	// check: the tape is read (outside git it stops on git, not on a missing tape).
	if code, _, errOut := tapesOut(t, root, "check", oldest); strings.Contains(errOut, "がありません") {
		t.Errorf("check does not find the closed tape: %d %q", code, errOut)
	}
	// prune: both forms of a tape go.
	if code, out, _ := tapesOut(t, root, "prune", "--keep", "2"); code != 0 || !strings.Contains(out, oldest) {
		t.Fatalf("prune: %d %q", code, out)
	}
	if _, err := os.Stat(gz); !os.IsNotExist(err) {
		t.Errorf("the closed tape was not deleted: %v", err)
	}
	if got := tape.IDs(dir); len(got) != 2 {
		t.Errorf("tapes = %v", got)
	}
}

// tapes new reports a tape that could not be compressed, and has closed the session all the same.
func TestTapesNewWarnsWhenTheTapeCannotBeCompressed(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	makeTape(t, root, time.Now(), "a.txt")
	ws, _ := session.Open(root, session.Options{})
	id, _ := ws.Current()
	if err := os.MkdirAll(filepath.Join(ws.TapePath(id)+tape.GzSuffix, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := tapesOut(t, root, "new")
	if code != 0 || !strings.Contains(out, "閉じました") || !strings.Contains(errOut, "警告") || !strings.Contains(errOut, id) {
		t.Errorf("new: %d %q %q", code, out, errOut)
	}
	if _, ok := ws.Current(); ok {
		t.Error("the session is still open")
	}
}
