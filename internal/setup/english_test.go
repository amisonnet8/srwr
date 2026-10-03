package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestEnglishDetails(t *testing.T) {
	t.Setenv("SRWR_LANG", "")
	root := ws(t, true, map[string]string{".mcp.json": `{"mcpServers":{"a":{"command":"x"},"b":{"command":"y"}}}`})
	res := run(t, root, false)
	want := map[string]string{
		".mcp.json":             "registered srwr mcp (2 other servers left as they were)",
		".claude/settings.json": "registered the hook; forbade Edit, Write, etc. (strict mode)",
	}
	for _, c := range res.Changes {
		if w, ok := want[c.Path]; ok && c.Detail != w {
			t.Errorf("%s: %q, want %q", c.Path, c.Detail, w)
		}
	}
	if d := run(t, root, true).Changes[2].Detail; d != "lifted the ban on Edit, Write, etc. (lenient mode)" {
		t.Errorf("lenient: %q", d)
	}
	if d := run(t, root, false).Changes[2].Detail; d != "forbade Edit, Write, etc. (strict mode)" {
		t.Errorf("strict: %q", d)
	}
}

func TestEnglishErrors(t *testing.T) {
	t.Setenv("SRWR_LANG", "")
	for name, tc := range map[string]struct{ file, text, want string }{
		"broken":       {".claude/settings.json", "{\n  \"a\": 1\n  \"b\": 2\n}\n", ".claude/settings.json is not valid JSON (near line 3)"},
		"array on top": {".mcp.json", "[]", "the top level of .mcp.json is not a JSON object"},
		"servers list": {".mcp.json", `{"mcpServers":[]}`, "mcpServers is not an object"},
		"deny string":  {".claude/settings.json", `{"permissions":{"deny":"Edit"}}`, "deny is not an array"},
	} {
		_, err := Init(Options{Root: ws(t, true, map[string]string{tc.file: tc.text}), Now: fixedNow})
		var ue *UserError
		if !errors.As(err, &ue) || !strings.Contains(ue.Msg, tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
