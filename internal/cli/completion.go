package cli

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// The scripts srwr completion prints. None of them holds a list of commands: at every TAB they ask srwr __complete.
//
//go:embed completion/srwr.bash completion/srwr.zsh completion/srwr.fish completion/srwr.ps1
var completionScripts embed.FS

// shells are the shells srwr completion knows, with the file of each script.
var shells = []struct{ name, file string }{
	{"bash", "srwr.bash"}, {"zsh", "srwr.zsh"}, {"fish", "srwr.fish"}, {"powershell", "srwr.ps1"},
}

// runCompletion is srwr completion <shell>: it prints the completion script of the shell.
func runCompletion(args []string, stdout, stderr io.Writer) int {
	use := lang.Pick("Usage: srwr completion <bash|zsh|fish|powershell>", "使い方: srwr completion <bash|zsh|fish|powershell>")
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, use)
		return 2
	}
	for _, s := range shells {
		if s.name != args[0] {
			continue
		}
		b, err := completionScripts.ReadFile("completion/" + s.file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "srwr completion: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(b)
		return 0
	}
	_, _ = fmt.Fprintf(stderr, lang.Pick("srwr completion: unknown shell %q\n%s\n", "srwr completion: 知らないシェル %q\n%s\n"), args[0], use)
	return 2
}

// candidate is one thing a shell may put in at a TAB: the text, and what it is.
type candidate struct{ value, desc string }

// runComplete is srwr __complete --word=<the word being typed> -- <the words before it, without "srwr">, which the
// completion scripts call. It prints one candidate per line, "value<TAB>description", and nothing where the shell should
// offer files. It never fails: a TAB must not show an error.
func runComplete(args []string, stdout io.Writer) int {
	word := ""
	var words []string
	for i, a := range args {
		if a == "--" {
			words = args[i+1:]
			break
		}
		if v, ok := strings.CutPrefix(a, "--word="); ok {
			word = v
		}
	}
	for _, c := range complete(joinEquals(words), word) {
		if c.desc == "" {
			_, _ = fmt.Fprintln(stdout, c.value)
		} else {
			_, _ = fmt.Fprintf(stdout, "%s\t%s\n", c.value, c.desc)
		}
	}
	return 0
}

// joinEquals puts back together what bash splits at "=": "--root", "=", "x" is "--root=x", and a line that ends in
// "--tape", "=" (the value is the word being typed) ends in "--tape=".
func joinEquals(words []string) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		if words[i] == "=" && len(out) > 0 && strings.HasPrefix(out[len(out)-1], "-") {
			out[len(out)-1] += "="
			if i+1 < len(words) {
				i++
				out[len(out)-1] += words[i]
			}
			continue
		}
		out = append(out, words[i])
	}
	return out
}

// valueFlags are the flags that take a value (the next word, or after "=").
var valueFlags = map[string]bool{"--root": true, "--tape": true, "--keep": true, "--older-than": true}

// complete returns the candidates for cur, after words (words[0] is the command), that start with cur.
func complete(words []string, cur string) []candidate {
	if len(words) == 0 {
		if strings.HasPrefix(cur, "-") {
			return withPrefix(cur, globalFlags())
		}
		return withPrefix(cur, commands())
	}
	cmd := words[0]
	root := "."
	given := map[string]bool{}
	var pos []string // the words that are not flags or their values
	pending := ""    // a flag whose value is the word being typed
	for i := 1; i < len(words); i++ {
		w := words[i]
		if !strings.HasPrefix(w, "-") {
			pos = append(pos, w)
			continue
		}
		name, val, hasEq := strings.Cut(w, "=")
		given[name] = true
		if !valueFlags[name] {
			continue
		}
		switch {
		case hasEq && val == "" && i == len(words)-1:
			pending = name
			continue
		case !hasEq && i+1 < len(words):
			i++
			val = words[i]
		case !hasEq:
			pending = name
			continue
		}
		if name == "--root" {
			root = val
		}
	}
	if pending != "" {
		return withPrefix(cur, flagValues(pending, root))
	}
	// zsh, fish and PowerShell keep --tape=x as one word.
	if name, val, ok := strings.Cut(cur, "="); ok && strings.HasPrefix(cur, "-") {
		var out []candidate
		for _, c := range withPrefix(val, flagValues(name, root)) {
			out = append(out, candidate{name + "=" + c.value, c.desc})
		}
		return out
	}
	if strings.HasPrefix(cur, "-") {
		var out []candidate
		for _, c := range flagsOf(cmd, pos) {
			if !given[c.value] {
				out = append(out, c)
			}
		}
		return withPrefix(cur, out)
	}
	switch {
	case cmd == "view" && len(pos) == 0:
		return withPrefix(cur, tapeCandidates(root))
	case cmd == "tapes" && len(pos) == 0:
		return withPrefix(cur, []candidate{
			{"new", lang.Pick("end the current session", "今のセッションを閉じる")},
			{"prune", lang.Pick("delete old tapes (--keep N or --older-than 30d)", "古いテープを消す（--keep N か --older-than 30d）")},
			{"path", lang.Pick("print the path of a tape", "テープのパスを出す")},
			{"check", lang.Pick("list changed files that are not on the tape", "変わったがテープにないファイルを出す")},
		})
	case cmd == "tapes" && len(pos) == 1 && (pos[0] == "path" || pos[0] == "check"):
		return withPrefix(cur, tapeCandidates(root))
	case cmd == "completion" && len(pos) == 0:
		var out []candidate
		for _, s := range shells {
			out = append(out, candidate{s.name, lang.Sprintf("the script for %s", "%s のスクリプト", s.name)})
		}
		return withPrefix(cur, out)
	}
	return nil // the file of trace, and what takes no word: the shell offers files
}

func commands() []candidate {
	return []candidate{
		{"mcp", lang.Pick("MCP server (started by the AI agent)", "MCP サーバー（AI のエージェントが起動する）")},
		{"hook", lang.Pick("record Claude Code hook events", "Claude Code の hook の記録")},
		{"view-server", lang.Pick("view server (started by the editor)", "表示サーバー（エディタが起動する）")},
		{"view", lang.Pick("replay a tape in Vim", "Vim でテープを再生する")},
		{"init", lang.Pick("set up a workspace for srwr", "作業場を srwr 用に準備する")},
		{"tapes", lang.Pick("list, end, tidy and check tapes", "テープの一覧・終了・整理・検査")},
		{"trace", lang.Pick("find the tape operations behind the lines a diff adds", "差分の追加行を作ったテープの操作を引く")},
		{"completion", lang.Pick("print a shell completion script", "シェルの補完スクリプトを出す")},
	}
}

func globalFlags() []candidate {
	return []candidate{
		{"--help", lang.Pick("show the usage", "使い方を出す")},
		{"--version", lang.Pick("show the version", "バージョンを出す")},
	}
}

// flagsOf returns the flags of cmd, given the words that are not flags (pos).
func flagsOf(cmd string, pos []string) []candidate {
	rootFlag := candidate{"--root", lang.Pick("the workspace directory", "作業場のディレクトリ")}
	switch cmd {
	case "mcp", "hook", "view-server":
		return []candidate{rootFlag}
	case "view":
		if len(pos) > 0 {
			return []candidate{rootFlag} // --live does not go with a tape
		}
		return []candidate{{"--live", lang.Pick("follow the current session live", "今のセッションをライブで見る")}, rootFlag}
	case "init":
		return []candidate{{"--lenient", lang.Pick("do not forbid Edit and Write", "Edit・Write を禁止しない")}, rootFlag}
	case "tapes":
		if len(pos) > 0 && pos[0] == "prune" {
			return []candidate{
				{"--keep", lang.Pick("keep the newest N tapes", "新しい方から N 本を残す")},
				{"--older-than", lang.Pick("delete tapes older than this (30d, 12h)", "これより古いテープを消す（30d、12h）")},
				rootFlag,
			}
		}
		return []candidate{rootFlag}
	case "trace":
		return []candidate{
			{"--as-tape", lang.Pick("write what was found as a tape to replay", "見つけたものを再生できるテープに書く")},
			{"--json", lang.Pick("print JSON", "JSON で出す")},
			{"--mark", lang.Pick("print the diff with a why after each hunk", "差分の各塊の後ろに理由を添えて出す")},
			rootFlag,
			{"--tape", lang.Pick("only this tape", "このテープだけ")},
		}
	}
	return nil
}

// flagValues returns what can be the value of a flag: the tapes for --tape, and nothing for the others (the shell offers
// files for --root).
func flagValues(flag, root string) []candidate {
	if flag == "--tape" {
		return tapeCandidates(root)
	}
	return nil
}

// tapeCandidates are the tapes of the workspace, newest first, each described by when it started and its title. Only the
// header of each tape is used; .srwr/ is not made.
func tapeCandidates(root string) []candidate {
	ws, err := session.Open(root, session.Options{Version: Version(), Now: Now})
	if err != nil {
		return nil
	}
	dir := filepath.Dir(ws.TapePath("x"))
	type head struct {
		id, title string
		started   time.Time
	}
	var heads []head
	for _, id := range tape.IDs(dir) {
		h := head{id: id}
		if path, ok := tape.Find(dir, id); ok {
			if b, err := tape.ReadFile(path); err == nil {
				if i := bytes.IndexByte(b, '\n'); i >= 0 {
					b = b[:i+1]
				}
				for _, ev := range tape.Parse(b).Events {
					if ev.Type == tape.TypeHeader {
						h.title = ev.Title
						h.started, _ = time.Parse(time.RFC3339, ev.StartedAt)
					}
				}
			}
		}
		heads = append(heads, h)
	}
	sort.Slice(heads, func(i, j int) bool {
		if !heads[i].started.Equal(heads[j].started) {
			return heads[i].started.After(heads[j].started)
		}
		return heads[i].id > heads[j].id
	})
	var out []candidate
	for _, h := range heads {
		var desc []string
		if !h.started.IsZero() {
			desc = append(desc, shortTime(h.started))
		}
		if h.title != "" {
			desc = append(desc, h.title)
		}
		out = append(out, candidate{h.id, strings.Join(desc, "  ")})
	}
	return out
}

// withPrefix keeps the candidates that start with prefix.
func withPrefix(prefix string, cands []candidate) []candidate {
	var out []candidate
	for _, c := range cands {
		if strings.HasPrefix(c.value, prefix) {
			out = append(out, c)
		}
	}
	return out
}
