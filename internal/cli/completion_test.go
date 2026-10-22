package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

func values(cands []candidate) []string {
	var out []string
	for _, c := range cands {
		out = append(out, c.value)
	}
	return out
}

func TestCompleteCandidates(t *testing.T) {
	root := t.TempDir() // no tapes: where tapes would come, nothing does
	tests := []struct {
		name  string
		words []string
		cur   string
		want  []string
	}{
		{"the commands", nil, "", []string{"mcp", "hook", "view-server", "view", "init", "tapes", "trace", "completion"}},
		{"the commands that start with the word", nil, "vi", []string{"view-server", "view"}},
		{"the flags of srwr itself", nil, "-", []string{"--help", "--version"}},
		{"an unknown command gives nothing", []string{"frob"}, "", nil},
		{"the actions of tapes", []string{"tapes"}, "", []string{"new", "prune", "path", "check"}},
		{"the actions of tapes after --root", []string{"tapes", "--root", root}, "p", []string{"prune", "path"}},
		{"nothing after the action new", []string{"tapes", "new"}, "", nil},
		{"the flags of tapes", []string{"tapes"}, "-", []string{"--root"}},
		{"the flags of tapes prune", []string{"tapes", "prune"}, "--", []string{"--keep", "--older-than", "--root"}},
		{"a flag given once is not offered again", []string{"tapes", "prune", "--keep", "3"}, "--", []string{"--older-than", "--root"}},
		{"the value of --keep is not completed", []string{"tapes", "prune", "--keep"}, "", nil},
		{"the value of --keep is not an action", []string{"tapes", "--keep", "3"}, "", []string{"new", "prune", "path", "check"}},
		{"the value of --older-than is not an action", []string{"tapes", "--older-than", "30d"}, "", []string{"new", "prune", "path", "check"}},
		{"the value of --root is left to the shell", []string{"view", "--root"}, "", nil},
		{"the flags of view", []string{"view"}, "-", []string{"--live", "--root"}},
		{"--live does not go with a tape", []string{"view", "20261001-0000-abcd"}, "-", []string{"--root"}},
		{"the flags of init", []string{"init"}, "--l", []string{"--lenient"}},
		{"the flags of mcp", []string{"mcp"}, "-", []string{"--root"}},
		{"the flags of trace", []string{"trace"}, "--", []string{"--as-tape", "--json", "--mark", "--root", "--tape"}},
		{"the file of trace is left to the shell", []string{"trace", "--mark"}, "", nil},
		{"the shells", []string{"completion"}, "", []string{"bash", "zsh", "fish", "powershell"}},
		{"only one shell", []string{"completion", "bash"}, "", nil},
		{"completion has no flags", []string{"completion"}, "-", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := values(complete(tt.words, tt.cur)); !slices.Equal(got, tt.want) {
				t.Errorf("complete(%q, %q) = %q, want %q", tt.words, tt.cur, got, tt.want)
			}
		})
	}
}

func TestJoinEquals(t *testing.T) {
	for _, tt := range []struct{ in, want []string }{
		{[]string{"trace", "--tape", "=", "x", "--mark"}, []string{"trace", "--tape=x", "--mark"}},
		{[]string{"trace", "--tape", "="}, []string{"trace", "--tape="}},
		{[]string{"trace", "a=b"}, []string{"trace", "a=b"}},
		{[]string{"trace", "=", "x"}, []string{"trace", "=", "x"}}, // "=" after a word that is not a flag stays
		{[]string{"--tape", "=", "x"}, []string{"--tape=x"}},       // a flag as the first word
		{[]string{"=", "x"}, []string{"=", "x"}},                   // nothing before "="
	} {
		if got := joinEquals(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("joinEquals(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCompleteTapes(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n2\n")
	makeTape(t, root, time.Now().Add(-time.Hour), "a.txt")
	ws, err := session.Open(root, session.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.StartSession("ドキュメントを先に直す", "言い回しを決める"); err != nil {
		t.Fatal(err)
	}
	makeTape(t, root, time.Now(), "a.txt")
	// The older tape was closed (compressed) by StartSession: the list must hold both kinds.
	ids := tape.IDs(filepath.Join(root, ".srwr", "tapes"))
	if len(ids) != 2 {
		t.Fatalf("tapes = %q, want 2", ids)
	}
	older, titled := ids[0], ids[1]
	if older > titled {
		older, titled = titled, older
	}
	// A tape named in the old way (in the local zone) can have an ID that sorts after a tape that started later: the order
	// is by the time a tape started, and only a tie falls back to the ID. This one is the older tape under another name.
	dir := filepath.Join(root, ".srwr", "tapes")
	oldPath, ok := tape.Find(dir, older)
	if !ok {
		t.Fatal("the older tape is not found")
	}
	b, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	late := "29991231-2359-zzzz"
	if err := os.WriteFile(filepath.Join(dir, late+strings.TrimPrefix(filepath.Base(oldPath), older)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	newestFirst := []string{titled, late, older}

	for _, tt := range []struct {
		name  string
		words []string
		cur   string
		want  []string
	}{
		{"view", []string{"view", "--root", root}, "", newestFirst},
		{"tapes path", []string{"tapes", "--root", root, "path"}, "", newestFirst},
		{"tapes check, the flag after the action", []string{"tapes", "check", "--root", root}, "", newestFirst},
		{"trace --tape (the next word)", []string{"trace", "--root", root, "--tape"}, "", newestFirst},
		{"trace --tape= (bash: the value is the word)", []string{"trace", "--root", root, "--tape="}, "", newestFirst},
		{"trace --tape= (the other shells: one word)", []string{"trace", "--root", root}, "--tape=", []string{"--tape=" + titled, "--tape=" + late, "--tape=" + older}},
		{"--root=<dir>", []string{"view", "--root=" + root}, "", newestFirst},
		{"the start of an ID", []string{"view", "--root", root}, titled[:len(titled)-1], []string{titled}},
		{"only one tape for path", []string{"tapes", "--root", root, "path", titled}, "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := values(complete(tt.words, tt.cur)); !slices.Equal(got, tt.want) {
				t.Errorf("complete(%q, %q) = %q, want %q", tt.words, tt.cur, got, tt.want)
			}
		})
	}
	got := complete([]string{"view", "--root", root}, "")
	if !strings.Contains(got[0].desc, "ドキュメントを先に直す") || strings.Contains(got[0].desc, "言い回しを決める") {
		t.Errorf("the description of the titled tape is %q: want its title, without its why", got[0].desc)
	}
	if !strings.Contains(got[2].desc, shortTime(time.Now().Add(-time.Hour))) {
		t.Errorf("the description of the older tape is %q: want when it started", got[2].desc)
	}
}

func TestCompleteMakesNoSrwrDirectory(t *testing.T) {
	root := t.TempDir()
	if got := complete([]string{"view", "--root", root}, ""); got != nil {
		t.Errorf("a workspace with no tapes gave %q", values(got))
	}
	if _, err := os.Stat(filepath.Join(root, ".srwr")); !os.IsNotExist(err) {
		t.Errorf(".srwr/ was made by a TAB: %v", err)
	}
}

func TestCompleteOutput(t *testing.T) {
	// One candidate per line, "value<TAB>description"; the word comes as --word=, and an empty one is fine.
	code, out, errOut := run([]string{"__complete", "--word=com", "--"}, "")
	if code != 0 || out != "completion\tシェルの補完スクリプトを出す\n" || errOut != "" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
	code, out, _ = run([]string{"__complete", "--word=", "--", "completion", "bash"}, "")
	if code != 0 || out != "" {
		t.Errorf("nothing to offer: code %d, stdout %q", code, out)
	}
	// bash hands --root=<dir> over split at "=": put back together, the next word is the action of tapes.
	code, out, _ = run([]string{"__complete", "--word=pa", "--", "tapes", "--root", "=", t.TempDir()}, "")
	if code != 0 || out != "path\tテープのパスを出す\n" {
		t.Errorf("--root = <dir> from bash: code %d, stdout %q", code, out)
	}
	// It never fails, whatever it is given.
	if code, _, errOut := run([]string{"__complete"}, ""); code != 0 || errOut != "" {
		t.Errorf("no arguments: code %d, stderr %q", code, errOut)
	}
}

func TestCompleteInEnglish(t *testing.T) {
	t.Setenv("SRWR_LANG", "")
	for _, c := range complete(nil, "view") {
		if c.value == "view" && c.desc != "replay a tape in Vim" {
			t.Errorf("the description of view is %q", c.desc)
		}
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, tt := range []struct{ shell, want string }{
		{"bash", "complete -o default -F _srwr srwr"},
		{"zsh", "#compdef srwr"},
		{"fish", "complete -c srwr"},
		{"powershell", "Register-ArgumentCompleter -Native -CommandName srwr"},
	} {
		code, out, errOut := run([]string{"completion", tt.shell}, "")
		if code != 0 || errOut != "" || !strings.Contains(out, tt.want) || !strings.Contains(out, "srwr __complete") {
			t.Errorf("%s: code %d, stderr %q, the script has %q: %v", tt.shell, code, errOut, tt.want, strings.Contains(out, tt.want))
		}
	}
	for _, args := range [][]string{{"completion"}, {"completion", "tcsh"}, {"completion", "bash", "zsh"}} {
		if code, out, errOut := run(args, ""); code != 2 || out != "" || !strings.Contains(errOut, "srwr completion <bash|zsh|fish|powershell>") {
			t.Errorf("%q: code %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
}
