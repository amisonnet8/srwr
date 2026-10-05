package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) newFile(file, content string) (*NewResult, *Error) {
	return e.c.New(NewInput{File: file, Content: content, Why: "start the config package"})
}

func TestNewCreatesTheFile(t *testing.T) {
	e := newEnv(t)
	res, err := e.newFile("cfg/config.go", "package cfg\n\nfunc Read() {}")
	if err != nil {
		t.Fatal(err)
	}
	if e.read("cfg/config.go") != "package cfg\n\nfunc Read() {}\n" {
		t.Errorf("file = %q", e.read("cfg/config.go"))
	}
	if res.Selection == "" || res.StartLine != 1 || res.EndLine != 3 {
		t.Errorf("result = %+v", res)
	}
	rs := e.replaces()
	if len(rs) != 1 {
		t.Fatalf("%d replaces", len(rs))
	}
	r := rs[0]
	if r.HookTool != "new" || r.Source != tape.SourceMCP || r.From != nil || r.Why == nil || *r.Why != "start the config package" ||
		r.StartLine != 1 || r.EndLine != 0 || r.OldText != "" || r.NewStartLine != 1 || r.NewEndLine != 3 || r.FileShaBefore != "" {
		t.Errorf("replace = %+v", r)
	}
	e.checkTape()
}

func TestNewEmptyAndTrailingLineBreaks(t *testing.T) {
	e := newEnv(t)
	res, err := e.newFile("empty.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.read("empty.txt") != "" || res.EndLine != 0 {
		t.Errorf("empty: %q %+v", e.read("empty.txt"), res)
	}
	if _, err := e.newFile("a.txt", "a\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.newFile("b.txt", "a\n\n"); err != nil {
		t.Fatal(err)
	}
	if e.read("a.txt") != "a\n" || e.read("b.txt") != "a\n\n" {
		t.Errorf("a = %q, b = %q", e.read("a.txt"), e.read("b.txt"))
	}
	e.checkTape()
}

func TestNewRefusesAnExistingFile(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "old\n")
	_, err := e.newFile("a.go", "new\n")
	wantCode(t, err, CodeFileExists)
	if e.read("a.go") != "old\n" || len(e.replaces()) != 0 {
		t.Error("the file was changed")
	}
	f := e.failures()
	if len(f) != 1 || f[0].Tool != "new" || f[0].Code != CodeFileExists || f[0].File == nil || *f[0].File != "a.go" {
		t.Errorf("failures = %+v", f)
	}
	e.noSecretOnTape("new\n")
}

func TestNewInputIsChecked(t *testing.T) {
	e := newEnv(t)
	_, err := e.c.New(NewInput{File: "a.txt", Content: "x", Why: "  "})
	wantCode(t, err, CodeInvalidInput)
	_, err = e.newFile("a.txt", "x\r\ny")
	wantCode(t, err, CodeInvalidInput)
	_, err = e.newFile("", "x")
	wantCode(t, err, CodeInvalidInput)
	_, err = e.newFile("/etc/x", "x")
	wantCode(t, err, CodeInvalidRange)
	_, err = e.newFile("../x", "x")
	wantCode(t, err, CodeInvalidRange)
	if _, statErr := os.Stat(filepath.Join(e.root, "a.txt")); statErr == nil {
		t.Error("a.txt was made")
	}
}

func TestNewDoesNotMakeFilesThatAreNotRecorded(t *testing.T) {
	e := newEnv(t)
	_, err := e.newFile(".env", "TOKEN="+secret)
	wantCode(t, err, CodeIgnoredFile)
	if _, statErr := os.Stat(filepath.Join(e.root, ".env")); statErr == nil {
		t.Error(".env was made")
	}
	e.noSecretOnTape(".env", secret)
	for _, f := range e.failures() {
		if f.File != nil {
			t.Errorf("failure names %q", *f.File)
		}
	}
}

func TestNewThroughALinkOutOfTheWorkspace(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.root, "link")); err != nil {
		t.Skip("symbolic links cannot be made here")
	}
	_, err := e.newFile("link/x.txt", "x")
	wantCode(t, err, CodeInvalidRange)
	_, err = e.newFile("link/deeper/x.txt", "x")
	wantCode(t, err, CodeInvalidRange)
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("something was made outside: %v", entries)
	}
}

func TestNewThroughALinkToAPlaceThatIsNotRecorded(t *testing.T) {
	e := newEnv(t)
	e.write(".srwrignore", "secret/\n")
	e.write("secret/keep", "x")
	if err := os.Symlink("secret", filepath.Join(e.root, "alias")); err != nil {
		t.Skip("symbolic links cannot be made here")
	}
	_, err := e.newFile("alias/x.txt", "x")
	wantCode(t, err, CodeIgnoredFile)
	if _, statErr := os.Stat(filepath.Join(e.root, "secret", "x.txt")); statErr == nil {
		t.Error("a file was made in the place that is not recorded")
	}
}

func TestNewWhereAParentIsAFile(t *testing.T) {
	e := newEnv(t)
	e.write("a", "x\n")
	_, err := e.newFile("a/b.txt", "x")
	wantCode(t, err, CodeInvalidInput)
	_, err = e.newFile("a/b/c.txt", "x")
	wantCode(t, err, CodeInvalidInput)
}

func TestNewAgainAfterTheFileWasDeleted(t *testing.T) {
	e := newEnv(t)
	if _, err := e.newFile("a.txt", "one\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.newFile("a.txt", "two\n"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.kinds(), ","); !strings.Contains(got, "replace,external,replace") {
		t.Errorf("kinds = %s", got)
	}
	if e.read("a.txt") != "two\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	e.checkTape()
}

func TestANewTokenCanBeUsedByReplace(t *testing.T) {
	e := newEnv(t)
	res, err := e.newFile("a.txt", "one\ntwo")
	if err != nil {
		t.Fatal(err)
	}
	e.rep(e.c, res.Selection, "ONE\ntwo\nthree")
	if e.read("a.txt") != "ONE\ntwo\nthree\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	e.checkTape()
}

func TestNewTokenOfAnEmptyFileCanBeUsedByReplace(t *testing.T) {
	e := newEnv(t)
	res, err := e.newFile("a.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	e.rep(e.c, res.Selection, "hello")
	if e.read("a.txt") != "hello\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	e.checkTape()
}

func TestNewOnAnExistingFileTheTapeKnowsDoesNotRecordADeletion(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "old\n")
	e.sel(e.c, "a.go", 1, 1)
	before := len(e.events())
	_, err := e.newFile("a.go", "new\n")
	wantCode(t, err, CodeFileExists)
	for _, ev := range e.events()[before:] {
		if ev.Type == tape.TypeExternal {
			t.Errorf("an external event was recorded: %+v", ev)
		}
	}
}

func TestNewAgainClearsTheDeletedMark(t *testing.T) {
	e := newEnv(t)
	if _, err := e.newFile("a.txt", "one\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.newFile("a.txt", "two\n"); err != nil {
		t.Fatal(err)
	}
	f := tape.Build(e.events()).Files["a.txt"]
	if f == nil || f.Deleted || f.Text != "two\n" {
		t.Errorf("tape state = %+v", f)
	}
}

func TestCreateFileDoesNotOverwriteAFileThatAppeared(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(dest, []byte("theirs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantCode(t, createFile(dest, "mine\n", "a.txt"), CodeFileExists)
	if b, _ := os.ReadFile(dest); string(b) != "theirs\n" { //nolint:gosec // a path in a temporary directory
		t.Errorf("file = %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("a temporary file is left: %v", entries)
	}
}
