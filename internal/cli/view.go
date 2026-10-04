package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/vim"
)

// minVimPatch is the oldest Vim srwr-view.vim is tested with (docs/reference/vim.md): the virtual text above a line
// came in 9.0.0438, and its fixes are in 9.0.0784. The same number is in vim/plugin/srwr.vim.
const minVimPatch = "9.0.0784"

// vimFeatures are the Vim features srwr-view.vim needs.
var vimFeatures = []string{"vim9script", "channel", "job", "textprop"}

// tapeName is what may be handed to Vim as a tape: the ID of a tape, which is a file name without the suffix.
var tapeName = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`)

type viewArgs struct {
	root string
	tape string
	live bool
}

// parseViewArgs reads `[テープ] [--live] [--root <作業場>]`. The flags may come before or after the tape.
func parseViewArgs(args []string) (viewArgs, error) {
	v := viewArgs{root: "."}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--live" || a == "-live":
			v.live = true
		case a == "--root" || a == "-root":
			if i+1 >= len(args) {
				return v, errors.New(lang.Pick("--root needs a workspace directory", "--root には作業場のディレクトリが要ります"))
			}
			i++
			v.root = args[i]
		case strings.HasPrefix(a, "--root="):
			v.root = strings.TrimPrefix(a, "--root=")
		case strings.HasPrefix(a, "-"):
			return v, fmt.Errorf(lang.Pick("unknown option %q", "知らないオプション %q"), a)
		case v.tape != "":
			return v, fmt.Errorf(lang.Pick("unexpected argument %q", "余分な引数 %q"), a)
		default:
			v.tape = a
		}
	}
	if v.live && v.tape != "" {
		return v, errors.New(lang.Pick("--live and a tape cannot be given together", "--live と テープは一緒に指定できません"))
	}
	if v.tape != "" {
		id := tape.IDOf(v.tape)
		if !tapeName.MatchString(id) {
			return v, fmt.Errorf(lang.Pick("%q is not a valid tape name (a tape ID or the path of a .tape.jsonl or .tape.jsonl.gz file)", "テープ %q の名前が正しくありません（テープID か .tape.jsonl・.tape.jsonl.gz のパス）"), v.tape)
		}
		v.tape = id
	}
	return v, nil
}

func runView(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	v, err := parseViewArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view: %v\n", err)
		return 2
	}
	root, err := filepath.Abs(v.root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view: %v\n", err)
		return 1
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: cannot open the workspace %q as a directory\n", "srwr view: 作業場 %q がディレクトリとして開けません\n"), v.root)
		return 1
	}
	vimPath, err := findVim()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view: %v\n", err)
		return 1
	}
	if missing, err := checkVim(vimPath); err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view: %v\n", err)
		return 1
	} else if len(missing) > 0 {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: this Vim (%s) cannot run it. Missing: %s\n"+
			"It needs Vim %s or later with %s. Set the environment variable SRWR_VIM to use another Vim.\n",
			"srwr view: この Vim（%s）では動かせません。足りないもの: %s\n"+
				"Vim %s 以上で、%s が要ります。別の Vim は環境変数 SRWR_VIM で指定できます。\n"),
			vimPath, strings.Join(missing, lang.Pick(", ", "、")), minVimPatch, "+"+strings.Join(vimFeatures, " +"))
		return 1
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: cannot find the cache directory: %v\n", "srwr view: キャッシュのディレクトリが分かりません: %v\n"), err)
		return 1
	}
	dir, err := extractVim(filepath.Join(cache, "srwr", "vim"), Version())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: cannot write the Vim scripts to the cache: %v\n", "srwr view: Vim スクリプトをキャッシュに書き出せません: %v\n"), err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: cannot find where srwr is: %v\n", "srwr view: srwr の場所が分かりません: %v\n"), err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	cmd := exec.Command(vimPath, vimLaunchArgs(dir, exe, v)...) //nolint:gosec // the Vim the user chose with SRWR_VIM, or vim from PATH
	cmd.Dir = root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr view: cannot start Vim: %v\n", "srwr view: Vim を起動できません: %v\n"), err)
		return 1
	}
	return 0
}

// vimQuote makes s a Vim string literal in single quotes.
func vimQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// vimLaunchArgs are the arguments of Vim: the scripts go on 'runtimepath' before the vimrc is read, so the vimrc
// can still set g:srwr_path; the command that opens what the person asked for runs after the plugin is loaded.
func vimLaunchArgs(dir, exe string, v viewArgs) []string {
	cmd := "SrwrOpen"
	switch {
	case v.live:
		cmd = "SrwrLive"
	case v.tape != "":
		cmd = "SrwrOpen " + v.tape
	}
	return []string{
		"--cmd", "let &runtimepath = escape(" + vimQuote(dir) + ", ' ,\\') . ',' . &runtimepath",
		"--cmd", "let g:srwr_path = " + vimQuote(exe),
		"-c", cmd,
	}
}

func findVim() (string, error) {
	name := os.Getenv("SRWR_VIM")
	if name == "" {
		name = "vim"
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf(lang.Pick("Vim not found (%s). Install Vim %s or later, or set SRWR_VIM to its path", "Vim が見つかりません（%s）。Vim %s 以上を入れるか、環境変数 SRWR_VIM で場所を指定してください"), name, minVimPatch)
	}
	return p, nil
}

// checkVim asks Vim itself (not the text of `vim --version`, which follows the locale) whether it is new enough and has
// the features. It returns what is missing. A Vim that cannot even run the question (vim-tiny) is missing everything.
func checkVim(vimPath string) (missing []string, err error) {
	tmp, err := os.MkdirTemp("", "srwr-vimcheck-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	out := filepath.Join(tmp, "out")
	script := filepath.Join(tmp, "check.vim")
	body := "let l = ['patch=' . has('patch-" + minVimPatch + "')]\n" +
		"for f in " + vimList(vimFeatures) + "\n  call add(l, f . '=' . has(f))\nendfor\n" +
		"call writefile(l, " + vimQuote(out) + ")\nqall!\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, vimPath, "-u", "NONE", "-i", "NONE", "-es", "-S", script) //nolint:gosec // the Vim the user chose
	cmd.Stdin = nil
	_ = cmd.Run()                  // a Vim that fails here is judged by what it wrote
	b, readErr := os.ReadFile(out) //nolint:gosec // a file in the temporary directory made above
	got := map[string]bool{}
	if readErr == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(l, "="); ok {
				got[k] = v == "1"
			}
		}
	}
	if !got["patch"] {
		missing = append(missing, lang.Pick("patch ", "パッチ ")+minVimPatch)
	}
	for _, f := range vimFeatures {
		if !got[f] {
			missing = append(missing, "+"+f)
		}
	}
	return missing, nil
}

func vimList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = vimQuote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// extractVim writes the embedded scripts to <base>/<version>-<hash of the files> and returns that directory. The
// directory name changes whenever the scripts do, so an old version never mixes with a new one. What is already there
// and complete is reused; a new one is written in a temporary directory and renamed, so nobody reads a half-written one.
func extractVim(base, version string) (string, error) {
	sum := sha256.New()
	var files []string
	err := fs.WalkDir(vim.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	slices.Sort(files)
	for _, p := range files {
		b, err := vim.Files.ReadFile(p)
		if err != nil {
			return "", err
		}
		sum.Write([]byte(p + "\x00"))
		sum.Write(b)
		sum.Write([]byte{0})
	}
	dir := filepath.Join(base, safeName(version)+"-"+hex.EncodeToString(sum.Sum(nil))[:12])
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(base, ".tmp-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, p := range files {
		b, err := vim.Files.ReadFile(p)
		if err != nil {
			return "", err
		}
		target := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, b, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ".complete"), nil, 0o600); err != nil {
		return "", err
	}
	// Another srwr may have finished while this one wrote: theirs are the same files, and must not be taken away.
	complete := func() bool { _, err := os.Stat(filepath.Join(dir, ".complete")); return err == nil }
	var err2 error
	for try := 0; try < 50; try++ {
		if complete() {
			return dir, nil
		}
		// A directory without .complete (left by something that died) is moved out of the way, not deleted in place: on
		// Windows a directory that is being renamed into place by another srwr cannot be removed.
		if _, err := os.Stat(dir); err == nil {
			trash := filepath.Join(base, ".trash-"+filepath.Base(tmp))
			if os.Rename(dir, trash) == nil {
				_ = os.RemoveAll(trash)
			}
		}
		if err2 = os.Rename(tmp, dir); err2 == nil {
			return dir, nil
		}
		// Someone else is putting theirs in place at the same time (on Windows the rename says "Access is denied").
		time.Sleep(20 * time.Millisecond)
	}
	if complete() {
		return dir, nil
	}
	return "", err2
}

var unsafeName = regexp.MustCompile(`[^0-9A-Za-z._+-]`)

func safeName(s string) string { return unsafeName.ReplaceAllString(s, "_") }
