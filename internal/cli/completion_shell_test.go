package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These run the completion scripts in the real shells, with the srwr built for the tests on the PATH. They check only that
// a script hooks into its shell and asks srwr: what the candidates are is checked by completion_test.go. A shell that is
// not installed is skipped (the devcontainer has all four; the CI runners have bash and pwsh).
func TestCompletionInRealShells(t *testing.T) {
	bin := binary(t)
	home := t.TempDir() // fish writes its history and data under HOME
	env := append(os.Environ(),
		"PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"SRWR_LANG=")
	tests := []struct {
		shell string
		args  []string
		want  string // a line of the output
	}{
		{"bash", []string{"-c", `eval "$(srwr completion bash)"; COMP_WORDS=(srwr ta); COMP_CWORD=1; _srwr; printf '%s\n' "${COMPREPLY[@]}"`}, "tapes"},
		{"zsh", []string{"-fc", `autoload -Uz compinit; compinit -u -d "$HOME/zcompdump"; source <(srwr completion zsh); print -r -- $_comps[srwr]`}, "_srwr"},
		{"fish", []string{"-c", `srwr completion fish | source; complete -C "srwr ta"`}, "tapes\tlist, end, tidy and check tapes"},
		{"pwsh", []string{"-NoLogo", "-NoProfile", "-Command", `srwr completion powershell | Out-String | Invoke-Expression; (TabExpansion2 "srwr ta" 7).CompletionMatches | ForEach-Object CompletionText`}, "tapes"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			if tt.shell != "pwsh" && runtime.GOOS == "windows" {
				t.Skip("only pwsh is checked on Windows")
			}
			if _, err := exec.LookPath(tt.shell); err != nil {
				t.Skipf("%s is not installed (in the devcontainer, rebuild it: .claude/rules/testing.md)", tt.shell)
			}
			cmd := exec.Command(tt.shell, tt.args...)
			cmd.Dir = t.TempDir()
			cmd.Env = env
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(out)), "\r\n", "\n"), "\n")
			if len(lines) != 1 || lines[0] != tt.want {
				t.Errorf("output %q, want the one line %q", out, tt.want)
			}
		})
	}
}
