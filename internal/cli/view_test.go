package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amisonnet8/srwr/vim"
)

func TestParseViewArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    viewArgs
		wantErr string
	}{
		{"nothing: the list", nil, viewArgs{root: "."}, ""},
		{"a tape id", []string{"20260930-0054-why-basic"}, viewArgs{root: ".", tape: "20260930-0054-why-basic"}, ""},
		{"a tape path", []string{".srwr/tapes/20260930-0054-why-basic.tape.jsonl"}, viewArgs{root: ".", tape: "20260930-0054-why-basic"}, ""},
		{"a closed tape path", []string{".srwr/tapes/20260930-0054-why-basic.tape.jsonl.gz"}, viewArgs{root: ".", tape: "20260930-0054-why-basic"}, ""},
		{"live", []string{"--live"}, viewArgs{root: ".", live: true}, ""},
		{"root after the tape", []string{"abc", "--root", "/w"}, viewArgs{root: "/w", tape: "abc"}, ""},
		{"root before the tape", []string{"--root=/w", "abc"}, viewArgs{root: "/w", tape: "abc"}, ""},
		{"live and root", []string{"--root", "/w", "--live"}, viewArgs{root: "/w", live: true}, ""},
		{"live with a tape", []string{"--live", "abc"}, viewArgs{}, "一緒に指定できません"},
		{"two tapes", []string{"a", "b"}, viewArgs{}, "余分な引数"},
		{"unknown option", []string{"--nope"}, viewArgs{}, "知らないオプション"},
		{"root without a value", []string{"--root"}, viewArgs{}, "--root には"},
		{"a tape that could inject a command", []string{"x|quit"}, viewArgs{}, "名前が正しくありません"},
		{"a tape with a space", []string{"a b"}, viewArgs{}, "名前が正しくありません"},
		{"a tape starting with a dot", []string{".hidden"}, viewArgs{}, "名前が正しくありません"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseViewArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// fakeVim writes a script that stands in for Vim. Asked the feature question (-es -S script), it writes what the test
// wants into the file the script would have written. Started for real, it records its arguments and directory.
func fakeVim(t *testing.T, answer string, exit int) (argsFile, pwdFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for Vim is a shell script")
	}
	dir := t.TempDir()
	argsFile, pwdFile = filepath.Join(dir, "args"), filepath.Join(dir, "pwd")
	script := `#!/bin/sh
case " $* " in
*" -es "*)
  while [ $# -gt 0 ]; do [ "$1" = "-S" ] && script="$2"; shift; done
  out=$(sed -n "s/.*writefile(l, '\(.*\)').*/\1/p" "$script")
  printf '%s' "$FAKE_VIM_ANSWER" > "$out"
  exit 0 ;;
esac
printf '%s\n' "$@" > "$FAKE_VIM_ARGS"
pwd > "$FAKE_VIM_PWD"
exit "$FAKE_VIM_EXIT"
`
	path := filepath.Join(dir, "vim")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // a script for the test to run
		t.Fatal(err)
	}
	t.Setenv("SRWR_VIM", path)
	t.Setenv("FAKE_VIM_ANSWER", answer)
	t.Setenv("FAKE_VIM_ARGS", argsFile)
	t.Setenv("FAKE_VIM_PWD", pwdFile)
	t.Setenv("FAKE_VIM_EXIT", map[bool]string{true: "0", false: "7"}[exit == 0])
	return argsFile, pwdFile
}

const allFeatures = "patch=1\nvim9script=1\nchannel=1\njob=1\ntextprop=1\n"

func TestCheckVim(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		missing []string
	}{
		{"everything there", allFeatures, nil},
		{"an old Vim", strings.Replace(allFeatures, "patch=1", "patch=0", 1), []string{"パッチ " + minVimPatch}},
		{"no job", strings.Replace(allFeatures, "job=1", "job=0", 1), []string{"+job"}},
		{"no textprop and no channel", "patch=1\nvim9script=1\nchannel=0\njob=1\ntextprop=0\n", []string{"+channel", "+textprop"}},
		{"vim-tiny cannot even answer", "", []string{"パッチ " + minVimPatch, "+vim9script", "+channel", "+job", "+textprop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeVim(t, tt.answer, 0)
			vimPath, err := findVim()
			if err != nil {
				t.Fatal(err)
			}
			got, err := checkVim(vimPath)
			if err != nil || !slices.Equal(got, tt.missing) {
				t.Errorf("missing = %v (%v), want %v", got, err, tt.missing)
			}
		})
	}
}

func TestFindVim(t *testing.T) {
	t.Setenv("SRWR_VIM", "/no/such/vim")
	if _, err := findVim(); err == nil || !strings.Contains(err.Error(), "SRWR_VIM") {
		t.Errorf("err = %v, want it to name SRWR_VIM", err)
	}
}

func TestPluginAndViewAgreeOnTheOldestVim(t *testing.T) {
	b, err := vim.Files.ReadFile("plugin/srwr.vim")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "patch-"+minVimPatch) {
		t.Errorf("vim/plugin/srwr.vim does not check patch-%s", minVimPatch)
	}
	for _, f := range vimFeatures {
		if !strings.Contains(string(b), "'"+f+"'") {
			t.Errorf("vim/plugin/srwr.vim does not check %s", f)
		}
	}
}

func TestExtractVim(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache", "srwr", "vim")
	dir, err := extractVim(base, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "v1.2.3-") {
		t.Errorf("dir = %s, want the version in front", dir)
	}
	// Every embedded file is there, byte for byte, and nothing else but the mark.
	var n int
	err = fs.WalkDir(vim.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		want, _ := vim.Files.ReadFile(p)
		got, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p))) //nolint:gosec // a path in a temporary directory
		if rerr != nil || string(got) != string(want) {
			t.Errorf("%s: %v, equal = %v", p, rerr, string(got) == string(want))
		}
		return nil
	})
	if err != nil || n < 10 {
		t.Fatalf("walked %d files (%v)", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test")); err == nil {
		t.Error("the tests must not be written out")
	}

	// The same call finds the same directory and writes nothing.
	mark := filepath.Join(dir, "plugin", "srwr.vim")
	before, _ := os.Stat(mark)
	again, err := extractVim(base, "v1.2.3")
	after, _ := os.Stat(mark)
	if err != nil || again != dir || !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("again = %s (%v), want %s untouched", again, err, dir)
	}
	// Another version is another directory.
	other, err := extractVim(base, "v2.0.0")
	if err != nil || other == dir {
		t.Errorf("other = %s (%v)", other, err)
	}
	// A directory left half-written (no mark) is written again.
	if err := os.Remove(filepath.Join(dir, ".complete")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	fixed, err := extractVim(base, "v1.2.3")
	if _, serr := os.Stat(mark); err != nil || fixed != dir || serr != nil {
		t.Errorf("fixed = %s (%v, %v)", fixed, err, serr)
	}
	// No temporary directory is left behind.
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("left behind: %s", e.Name())
		}
	}
	// A version with characters a directory name should not have.
	odd, err := extractVim(base, "v0.0.0-2026+dirty/../x")
	if err != nil || filepath.Dir(odd) != base {
		t.Errorf("odd = %s (%v)", odd, err)
	}
}

func TestExtractVimTogether(t *testing.T) {
	base := filepath.Join(t.TempDir(), "vim")
	var wg sync.WaitGroup
	dirs := make([]string, 8)
	errs := make([]error, 8)
	for i := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dirs[i], errs[i] = extractVim(base, "v1")
		}()
	}
	wg.Wait()
	for i := range dirs {
		if errs[i] != nil || dirs[i] != dirs[0] {
			t.Errorf("%d: %s (%v), want %s", i, dirs[i], errs[i], dirs[0])
		}
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "autoload", "srwr", "replay.vim")); err != nil {
		t.Error(err)
	}
}

func TestViewStartsVim(t *testing.T) {
	argsFile, pwdFile := fakeVim(t, allFeatures, 0)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv("LocalAppData", cache)
	root := t.TempDir()

	tests := []struct {
		name string
		args []string
		cmd  string
	}{
		{"the list", []string{"--root", root}, "SrwrOpen"},
		{"a tape", []string{"--root", root, "20260930-0054-why-basic"}, "SrwrOpen 20260930-0054-why-basic"},
		{"live", []string{"--root", root, "--live"}, "SrwrLive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(append([]string{"view"}, tt.args...), "")
			if code != 0 {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			b, err := os.ReadFile(argsFile) //nolint:gosec // a path in a temporary directory
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
			if len(args) != 6 || args[0] != "--cmd" || args[2] != "--cmd" || args[4] != "-c" || args[5] != tt.cmd {
				t.Fatalf("args = %q, want --cmd, --cmd, -c %q", args, tt.cmd)
			}
			if !strings.Contains(args[1], "runtimepath") || !strings.Contains(args[1], "srwr") {
				t.Errorf("runtimepath command = %q", args[1])
			}
			exe, _ := os.Executable()
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			if args[3] != "let g:srwr_path = '"+exe+"'" {
				t.Errorf("g:srwr_path command = %q", args[3])
			}
			pwd, _ := os.ReadFile(pwdFile) //nolint:gosec // a path in a temporary directory
			wantDir, _ := filepath.EvalSymlinks(root)
			gotDir, _ := filepath.EvalSymlinks(strings.TrimSpace(string(pwd)))
			if gotDir != wantDir {
				t.Errorf("Vim started in %q, want the workspace %q", gotDir, wantDir)
			}
		})
	}
}

func TestViewPassesTheExitCodeOn(t *testing.T) {
	fakeVim(t, allFeatures, 7)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if code, _, _ := run([]string{"view", "--root", t.TempDir()}, ""); code != 7 {
		t.Errorf("code = %d, want 7", code)
	}
}

func TestViewRefusesAnOldVim(t *testing.T) {
	fakeVim(t, strings.Replace(allFeatures, "channel=1", "channel=0", 1), 0)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	code, _, stderr := run([]string{"view", "--root", t.TempDir()}, "")
	if code != 1 || !strings.Contains(stderr, "足りないもの: +channel") || !strings.Contains(stderr, minVimPatch) || !strings.Contains(stderr, "SRWR_VIM") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestViewErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"a stray option", []string{"view", "--nope"}, 2, "知らないオプション"},
		{"a root that is missing", []string{"view", "--root", "/no/such/dir/at/all"}, 1, "ディレクトリとして開けません"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(tt.args, "")
			if code != tt.wantCode || !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("code = %d, stderr = %q", code, stderr)
			}
		})
	}
	t.Run("no Vim", func(t *testing.T) {
		t.Setenv("SRWR_VIM", "/no/such/vim")
		code, _, stderr := run([]string{"view", "--root", t.TempDir()}, "")
		if code != 1 || !strings.Contains(stderr, "Vim が見つかりません") {
			t.Errorf("code = %d, stderr = %q", code, stderr)
		}
	})
}

// With a real Vim, without a screen: the scripts are found on 'runtimepath' once extracted, and g:srwr_path is the binary
// srwr gives. (The arguments are the ones srwr starts Vim with; only -es and the question afterwards are added.)
func TestLaunchArgsWithARealVim(t *testing.T) {
	real, err := exec.LookPath("vim")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("no vim")
	}
	dir, err := extractVim(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	args := []string{"-u", "NONE", "-i", "NONE", "-es"}
	args = append(args, vimLaunchArgs(dir, "/the srwr/it's", viewArgs{tape: ""})[:4]...) // the two --cmd; -c runs a command with a server
	args = append(args, "-c", "runtime plugin/srwr.vim",
		"-c", "call writefile([exists(':SrwrOpen') . ' ' . exists(':SrwrLive') . ' ' . exists(':SrwrNext') . ' ' . g:srwr_path], '"+out+"')",
		"-c", "qall!")
	if b, err := exec.Command(real, args...).CombinedOutput(); err != nil { //nolint:gosec // the Vim found in PATH
		t.Fatalf("vim: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != "2 2 2 /the srwr/it's" {
		t.Errorf("Vim answered %q, want the commands to exist and g:srwr_path to be the binary", got)
	}
}

// Many processes extract at once, over and over: whoever is second must not take away what the first has finished.
func TestExtractVimTogetherManyTimes(t *testing.T) {
	for round := range 40 {
		base := filepath.Join(t.TempDir(), "vim")
		var wg sync.WaitGroup
		dirs := make([]string, 16)
		errs := make([]error, 16)
		for i := range dirs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				dirs[i], errs[i] = extractVim(base, "v1")
			}()
		}
		wg.Wait()
		for i := range dirs {
			if errs[i] != nil || dirs[i] != dirs[0] {
				t.Fatalf("round %d, %d: %s (%v), want %s", round, i, dirs[i], errs[i], dirs[0])
			}
		}
		if _, err := os.Stat(filepath.Join(dirs[0], ".complete")); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}
