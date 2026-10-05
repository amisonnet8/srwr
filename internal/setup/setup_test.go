package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// The texts these tests expect are the Japanese ones; english_test.go sets SRWR_LANG back for the English ones.
	_ = os.Setenv("SRWR_LANG", "ja")
	os.Exit(m.Run())
}

var fixedNow = func() time.Time { return time.Date(2026, 10, 3, 17, 12, 4, 0, time.Local) }

func ws(t *testing.T, git bool, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if git {
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // a file in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func run(t *testing.T, root string, lenient bool) *Result {
	t.Helper()
	res, err := Init(Options{Root: root, Lenient: lenient, Now: fixedNow, LookPath: func(string) (string, error) { return "/usr/bin/srwr", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func kinds(r *Result) map[string]Kind {
	m := map[string]Kind{}
	for _, c := range r.Changes {
		m[c.Path] = c.Kind
	}
	return m
}

const wantMCP = `{
  "mcpServers": {
    "srwr": {
      "command": "srwr",
      "args": [
        "mcp"
      ]
    }
  }
}
`

const wantStrictSettings = `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Read|Bash|Grep|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "srwr hook"
          }
        ]
      }
    ]
  },
  "enabledMcpjsonServers": [
    "srwr"
  ],
  "permissions": {
    "allow": [
      "mcp__srwr__select",
      "mcp__srwr__replace",
      "mcp__srwr__sub",
      "mcp__srwr__new"
    ],
    "deny": [
      "Edit",
      "Write",
      "MultiEdit",
      "NotebookEdit"
    ]
  }
}
`

func TestEmptyWorkspace(t *testing.T) {
	root := ws(t, true, nil)
	res := run(t, root, false)
	if got := read(t, root, ".mcp.json"); got != wantMCP {
		t.Errorf(".mcp.json =\n%s", got)
	}
	if got := read(t, root, ".claude/settings.json"); got != wantStrictSettings {
		t.Errorf("settings.json =\n%s", got)
	}
	if got := read(t, root, ".gitignore"); got != ".srwr/key\n.srwr/lock\n.srwr/active\n.srwr/init-backup/\n" {
		t.Errorf(".gitignore = %q", got)
	}
	if !exists(root, ".srwr/key") {
		t.Error("no key")
	}
	k := kinds(res)
	for _, p := range []string{".srwr/", ".mcp.json", ".claude/settings.json", ".gitignore"} {
		if k[p] != Created {
			t.Errorf("%s = %v, want Created", p, k[p])
		}
	}
	if res.Backup != "" || !res.RegistrationChanged {
		t.Errorf("backup %q, registration changed %v", res.Backup, res.RegistrationChanged)
	}
}

func TestNoGitMeansNoGitignore(t *testing.T) {
	// A temporary directory may sit inside a git work tree; make sure of the case by checking the result against inGit.
	root := ws(t, false, nil)
	res := run(t, root, false)
	_, has := kinds(res)[".gitignore"]
	if has != inGit(root) || (!inGit(root) && exists(root, ".gitignore")) {
		t.Errorf(".gitignore listed %v, written %v, inGit %v", has, exists(root, ".gitignore"), inGit(root))
	}
}

func TestSecondRunChangesNothing(t *testing.T) {
	root := ws(t, true, nil)
	run(t, root, false)
	before := map[string]string{}
	for _, f := range []string{".mcp.json", ".claude/settings.json", ".gitignore", ".srwr/key"} {
		before[f] = read(t, root, f)
	}
	res := run(t, root, false)
	if res.Changed() || res.Backup != "" || res.RegistrationChanged {
		t.Errorf("second run changed something: %+v", res)
	}
	for f, want := range before {
		if got := read(t, root, f); got != want {
			t.Errorf("%s changed", f)
		}
	}
	if exists(root, ".srwr/init-backup") {
		t.Error("a backup was made for nothing")
	}
}

func TestKeepsWhatIsThere(t *testing.T) {
	root := ws(t, true, map[string]string{
		".mcp.json": `{"mcpServers":{"other":{"command":"x"},"more":{"command":"y"}},"zeta":1,"alpha":2.50}`,
		".claude/settings.json": `{
  "model": "opus",
  "permissions": {"deny": ["Bash(rm:*)", "Edit"], "allow": ["Read"], "ask": ["Bash"]},
  "hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "other hook"}]}], "Stop": []},
  "env": {"A": "<b>&"}
}`,
		".gitignore": "node_modules\n.srwr/key",
	})
	res := run(t, root, false)
	mcp := read(t, root, ".mcp.json")
	for _, want := range []string{`"other"`, `"more"`, `"zeta": 1`, `"alpha": 2.50`, `"srwr"`} {
		if !strings.Contains(mcp, want) {
			t.Errorf(".mcp.json lacks %s:\n%s", want, mcp)
		}
	}
	if strings.Index(mcp, `"other"`) > strings.Index(mcp, `"srwr"`) || strings.Index(mcp, `"zeta"`) > strings.Index(mcp, `"alpha"`) {
		t.Errorf("the order of the keys changed:\n%s", mcp)
	}
	s := read(t, root, ".claude/settings.json")
	for _, want := range []string{`"model": "opus"`, `"Bash(rm:*)"`, `"ask"`, `"other hook"`, `"Stop": []`, `"A": "<b>&"`, `"srwr hook"`, `"NotebookEdit"`, `"mcp__srwr__replace"`} {
		if !strings.Contains(s, want) {
			t.Errorf("settings.json lacks %s:\n%s", want, s)
		}
	}
	if strings.Count(s, `"Edit"`) != 1 {
		t.Errorf("Edit forbidden twice:\n%s", s)
	}
	if got := read(t, root, ".gitignore"); got != "node_modules\n.srwr/key\n.srwr/lock\n.srwr/active\n.srwr/init-backup/\n" {
		t.Errorf(".gitignore = %q", got)
	}
	if res.Backup != ".srwr/init-backup/20261003-171204" {
		t.Fatalf("backup = %q", res.Backup)
	}
	for _, f := range []string{".mcp.json", ".claude/settings.json", ".gitignore"} {
		if got := read(t, root, res.Backup+"/"+f); got == read(t, root, f) || got == "" {
			t.Errorf("backup of %s is wrong: %q", f, got)
		}
	}
	if got := read(t, root, res.Backup+"/.mcp.json"); !strings.HasPrefix(got, `{"mcpServers"`) {
		t.Errorf("backup is not the original: %q", got)
	}
	k := kinds(res)
	if k[".mcp.json"] != Appended || k[".claude/settings.json"] != Appended || k[".gitignore"] != Appended || k[".srwr/"] != Created {
		t.Errorf("kinds = %v", k)
	}
	if !strings.Contains(res.Changes[1].Detail, "ほかのサーバー 2 件") {
		t.Errorf("detail = %q", res.Changes[1].Detail)
	}
}

func TestServerThatIsThereIsLeftAlone(t *testing.T) {
	root := ws(t, false, map[string]string{".mcp.json": `{"mcpServers":{"srwr":{"command":"/opt/srwr","args":["mcp","--root","x"]}}}`})
	res := run(t, root, false)
	if kinds(res)[".mcp.json"] != Unchanged || !strings.Contains(read(t, root, ".mcp.json"), "/opt/srwr") {
		t.Error("an srwr entry that is there was changed")
	}
}

func TestStrictAndLenientSwitch(t *testing.T) {
	root := ws(t, false, map[string]string{".claude/settings.json": `{"permissions":{"deny":["Bash(rm:*)"]}}`})
	run(t, root, true)
	s := read(t, root, ".claude/settings.json")
	if strings.Contains(s, `"Edit"`) || !strings.Contains(s, `"Bash(rm:*)"`) || !strings.Contains(s, "srwr hook") {
		t.Errorf("lenient:\n%s", s)
	}
	res := run(t, root, false)
	s = read(t, root, ".claude/settings.json")
	for _, f := range forbidden {
		if !strings.Contains(s, `"`+f+`"`) {
			t.Errorf("strict lacks %s:\n%s", f, s)
		}
	}
	if d := res.Changes[2].Detail; d != "Edit・Write などを禁止しました（厳格モード）" {
		t.Errorf("detail = %q", d)
	}
	res = run(t, root, true)
	if d := res.Changes[2].Detail; d != "Edit・Write などの禁止を外しました（緩いモード）" {
		t.Errorf("detail = %q", d)
	}
	if res.RegistrationChanged {
		t.Error("switching to lenient is not a change of the registration")
	}
	// All the forbidden entries gone: the empty deny list goes too.
	root2 := ws(t, false, nil)
	run(t, root2, false)
	run(t, root2, true)
	if s := read(t, root2, ".claude/settings.json"); strings.Contains(s, "deny") {
		t.Errorf("empty deny stayed:\n%s", s)
	}
}

func TestBrokenFilesStopEverything(t *testing.T) {
	for name, tc := range map[string]struct{ file, text, want string }{
		"broken settings": {".claude/settings.json", "{\n  \"a\": 1\n  \"b\": 2\n}\n", "（3 行目付近）"},
		"broken mcp":      {".mcp.json", "{", ".mcp.json を JSON として読めません"},
		"array on top":    {".mcp.json", "[]", "オブジェクトではありません"},
		"servers a list":  {".mcp.json", `{"mcpServers":[]}`, "mcpServers がオブジェクトではありません"},
		"deny a string":   {".claude/settings.json", `{"permissions":{"deny":"Edit"}}`, "deny が配列ではありません"},
		"trailing data":   {".mcp.json", "{} {}", "JSON として読めません"},
	} {
		root := ws(t, true, map[string]string{tc.file: tc.text})
		_, err := Init(Options{Root: root, Now: fixedNow})
		var ue *UserError
		if !errors.As(err, &ue) || !strings.Contains(ue.Msg, tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if exists(root, ".srwr") || exists(root, ".gitignore") || read(t, root, tc.file) != tc.text {
			t.Errorf("%s: something was written", name)
		}
	}
}

func TestSrwrNotOnPath(t *testing.T) {
	root := ws(t, false, nil)
	res, err := Init(Options{Root: root, Now: fixedNow, LookPath: func(string) (string, error) { return "", errors.New("no") }})
	if err != nil || res.SrwrOnPath {
		t.Errorf("SrwrOnPath = %v, err %v", res.SrwrOnPath, err)
	}
}
