package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// serve runs the requests (one JSON value per line) through a Server on a fresh workspace and returns the response lines.
func serve(t *testing.T, root string, lines ...string) []string {
	t.Helper()
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Core: &core.Core{WS: ws}, Version: "test"}
	var out bytes.Buffer
	if err := s.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var res []string
	for l := range strings.SplitSeq(out.String(), "\n") {
		if l != "" {
			res = append(res, l)
		}
	}
	return res
}

func toolCall(id int, name, args string) string {
	return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

// body decodes a tools/call response: the JSON inside the text, and isError.
func body(t *testing.T, line string) (map[string]any, bool) {
	t.Helper()
	var r struct {
		Result struct {
			Content []struct{ Text, Type string }
			IsError bool
		}
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	if len(r.Result.Content) != 1 || r.Result.Content[0].Type != "text" {
		t.Fatalf("content = %+v in %s", r.Result.Content, line)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Result.Content[0].Text), &m); err != nil {
		t.Fatalf("text of %s: %v", line, err)
	}
	return m, r.Result.IsError
}

func TestInitialize(t *testing.T) {
	tests := []struct {
		name, params, wantVersion string
	}{
		{"newest", `{"protocolVersion":"2025-11-25","clientInfo":{"name":"c"}}`, "2025-11-25"},
		{"2025-06-18", `{"protocolVersion":"2025-06-18"}`, "2025-06-18"},
		{"2025-03-26", `{"protocolVersion":"2025-03-26"}`, "2025-03-26"},
		{"oldest", `{"protocolVersion":"2024-11-05"}`, "2024-11-05"},
		{"a newer one than srwr knows", `{"protocolVersion":"2030-01-01"}`, "2025-11-25"},
		{"an older one than srwr knows", `{"protocolVersion":"2024-01-01"}`, "2025-11-25"},
		{"no version", `{}`, "2025-11-25"},
		{"no params at all", ``, "2025-11-25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := `{"jsonrpc":"2.0","id":1,"method":"initialize"`
			if tt.params != "" {
				req += `,"params":` + tt.params
			}
			got := serve(t, t.TempDir(), req+`}`)
			want := `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"` + tt.wantVersion + `","serverInfo":{"name":"srwr","version":"test"}}}`
			if len(got) != 1 || got[0] != want {
				t.Errorf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestClientNameIsTheAuthorOfTheTape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	serve(t, root,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"claude-code"}}}`,
		toolCall(2, "look", `{"file":"f.txt","startLine":1,"endLine":1,"why":"w"}`))
	matches, _ := filepath.Glob(filepath.Join(root, ".srwr", "tapes", "*.tape.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("tapes = %v", matches)
	}
	b, _ := os.ReadFile(matches[0])
	if first, _, _ := strings.Cut(string(b), "\n"); !strings.Contains(first, `"author":{"kind":"ai","name":"claude-code"}`) ||
		!strings.Contains(first, `"tool":{"name":"srwr","version":"test"}`) {
		t.Errorf("header = %s", first)
	}
}

func TestPingNotificationsAndUnknowns(t *testing.T) {
	got := serve(t, t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"delete","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":"x"}`,
	)
	want := []string{
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found: resources/list"}}`,
		`{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"unknown tool: delete"}}`,
	}
	if len(got) != 4 {
		t.Fatalf("got %d lines: %q", len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d:\n got %s\nwant %s", i, got[i], w)
		}
	}
	if !strings.Contains(got[3], `"code":-32602`) {
		t.Errorf("params that are not an object: %s", got[3])
	}
}

func TestToolsList(t *testing.T) {
	got := serve(t, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var r struct {
		Result struct {
			Tools []struct {
				Name        string
				Description string
				InputSchema struct {
					Type       string
					Properties map[string]map[string]any
					Required   []string
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(got[0]), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Result.Tools) != 4 || r.Result.Tools[0].Name != "look" || r.Result.Tools[1].Name != "edit" || r.Result.Tools[2].Name != "replace" || r.Result.Tools[3].Name != "new" {
		t.Fatalf("tools = %+v", r.Result.Tools)
	}
	for _, tool := range r.Result.Tools {
		if tool.Description == "" || tool.InputSchema.Type != "object" {
			t.Errorf("%s: description %q, type %q", tool.Name, tool.Description, tool.InputSchema.Type)
		}
		why, ok := tool.InputSchema.Properties["why"]
		if !ok || why["pattern"] != `\S` || why["minLength"] != float64(1) {
			t.Errorf("%s: why = %v: it must refuse an empty or blank text in the schema", tool.Name, why)
		}
		if !contains(tool.InputSchema.Required, "why") {
			t.Errorf("%s: why is not required: %v", tool.Name, tool.InputSchema.Required)
		}
		for _, name := range tool.InputSchema.Required {
			if _, ok := tool.InputSchema.Properties[name]; !ok {
				t.Errorf("%s: required %q has no property", tool.Name, name)
			}
		}
	}
	// The line numbers are optional: expect can find the range.
	sel := r.Result.Tools[0].InputSchema
	if contains(sel.Required, "startLine") || contains(sel.Required, "endLine") || contains(sel.Required, "expect") {
		t.Errorf("select requires %v: only file and why are required", sel.Required)
	}
	if _, ok := sel.Properties["expect"]; !ok {
		t.Error("select has no expect")
	}
	if !contains(r.Result.Tools[0].InputSchema.Required, "file") || !contains(r.Result.Tools[1].InputSchema.Required, "newText") {
		t.Error("required lists are wrong")
	}
	// edit takes a token or a file: neither alone is required, and the by-file inputs exist.
	ed := r.Result.Tools[1].InputSchema
	for _, name := range []string{"selection", "file", "startLine", "endLine", "expect"} {
		if contains(ed.Required, name) {
			t.Errorf("edit requires %s", name)
		}
		if _, ok := ed.Properties[name]; !ok {
			t.Errorf("edit has no %s", name)
		}
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestSelectAndReplace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("1\n2\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, _ := session.Open(root, session.Options{})
	s := &Server{Core: &core.Core{WS: ws}}
	call := func(id int, name, args string) (map[string]any, bool) {
		var out bytes.Buffer
		if err := s.Serve(strings.NewReader(toolCall(id, name, args)+"\n"), &out); err != nil {
			t.Fatal(err)
		}
		return body(t, strings.TrimSpace(out.String()))
	}

	sel, isErr := call(1, "look", `{"file":"a.go","startLine":2,"endLine":3,"why":"見る"}`)
	if isErr || sel["ok"] != true || len(sel["lines"].([]any)) != 2 {
		t.Fatalf("select = %v %v", sel, isErr)
	}
	rep, isErr := call(2, "edit", `{"selection":"`+sel["selection"].(string)+`","newText":"a < b && c > d","why":"変える"}`)
	if isErr || rep["ok"] != true || rep["startLine"] != float64(2) || rep["endLine"] != float64(2) {
		t.Fatalf("replace = %v %v", rep, isErr)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.go")); string(b) != "1\na < b && c > d\n" { //nolint:gosec // a path in a temporary directory
		t.Errorf("file = %q", b)
	}

	// An edit by file and expect needs no token, and takes the line numbers if it has them.
	byFile, isErr := call(5, "edit", `{"file":"a.go","startLine":2,"endLine":2,"expect":"a < b && c > d","newText":"two","why":"変える"}`)
	if isErr || byFile["ok"] != true || byFile["selection"] == "" || byFile["startLine"] != float64(2) {
		t.Fatalf("edit by file = %v %v", byFile, isErr)
	}
	byExpect, isErr := call(6, "edit", `{"file":"a.go","expect":"two","newText":"a < b && c > d","why":"戻す"}`)
	if isErr || byExpect["ok"] != true {
		t.Fatalf("edit by expect = %v %v", byExpect, isErr)
	}

	// A select with only expect finds its own range, and says where.
	found, isErr := call(4, "look", `{"file":"a.go","expect":"a < b && c > d","why":"探す"}`)
	if isErr || found["startLine"] != float64(2) || found["endLine"] != float64(2) {
		t.Errorf("select by expect = %v %v", found, isErr)
	}

	// The result holds the range now and the lines around it; what is empty is [] and not null.
	for key, want := range map[string]int{"lines": 1, "above": 1, "below": 0} {
		if l, ok := rep[key].([]any); !ok || len(l) != want {
			t.Errorf("%s of the replace = %#v, want %d lines", key, rep[key], want)
		}
	}
	if l := rep["lines"].([]any); l[0] != "a < b && c > d" {
		t.Errorf("lines of the replace = %#v", l)
	}

	// An empty range gives [] and not null.
	empty, _ := call(3, "look", `{"file":"a.go","startLine":3,"endLine":2,"why":"w"}`)
	if l, ok := empty["lines"].([]any); !ok || len(l) != 0 {
		t.Errorf("lines of an empty range = %#v, want []", empty["lines"])
	}
}

func TestSub(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{"a.go": "foo\nx\nfoo\n", "b.go": "foo\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := serve(t, root, toolCall(1, "replace", `{"files":["a.go","b.go"],"old":"foo","new":"bar & <baz>","count":3,"why":"名前を変える"}`))
	m, isErr := body(t, got[0])
	files, _ := m["files"].([]any)
	if isErr || m["ok"] != true || m["count"] != float64(3) || len(files) != 2 {
		t.Fatalf("sub = %v %v", m, isErr)
	}
	a := files[0].(map[string]any)
	hits, _ := a["hits"].([]any)
	if a["file"] != "a.go" || a["count"] != float64(2) || len(hits) != 2 {
		t.Fatalf("a.go = %v", a)
	}
	if _, has := a["selection"]; has {
		t.Error("replace returns no selection token")
	}
	h := hits[0].(map[string]any)
	if h["startLine"] != float64(1) || h["endLine"] != float64(1) || h["lines"].([]any)[0] != "bar & <baz>" ||
		len(h["above"].([]any)) != 0 || h["below"].([]any)[0] != "x" {
		t.Errorf("first hit = %v: before is [] and not null", h)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "b.go")); string(b) != "bar & <baz>\n" { //nolint:gosec // a path in a temporary directory
		t.Errorf("b.go = %q", b)
	}
}

func TestNew(t *testing.T) {
	root := t.TempDir()
	got := serve(t, root, toolCall(1, "new", `{"file":"pkg/a.go","content":"package pkg\n\nvar X = 1","why":"パッケージを作る"}`))
	m, isErr := body(t, got[0])
	if isErr || m["ok"] != true || m["startLine"] != float64(1) || m["endLine"] != float64(3) || m["selection"] == "" {
		t.Fatalf("new = %v %v", m, isErr)
	}
	if _, has := m["lines"]; has {
		t.Errorf("the answer has lines: %v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "pkg", "a.go")); string(b) != "package pkg\n\nvar X = 1\n" { //nolint:gosec // a path in a temporary directory
		t.Errorf("file = %q", b)
	}
	// A file that exists is refused.
	got = serve(t, root, toolCall(1, "new", `{"file":"pkg/a.go","content":"x","why":"w"}`))
	if m, isErr = body(t, got[0]); !isErr || m["error"].(map[string]any)["code"] != "file_exists" {
		t.Errorf("second new = %v %v", m, isErr)
	}
}

func TestToolErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("1\n2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, tool, args, code string
		actual                 any
	}{
		{"range", "look", `{"file":"a.go","startLine":9,"endLine":9,"why":"w"}`, "invalid_range", map[string]any{"lineCount": float64(2)}},
		{"missing file", "look", `{"file":"no.go","startLine":1,"endLine":1,"why":"w"}`, "file_not_found", nil},
		{"missing why", "look", `{"file":"a.go","startLine":1,"endLine":1}`, "invalid_input", nil},
		{"blank why", "look", `{"file":"a.go","startLine":1,"endLine":1,"why":"  "}`, "invalid_input", nil},
		{"missing file argument", "look", `{"startLine":1,"endLine":1,"why":"w"}`, "invalid_input", nil},
		{"line as a string", "look", `{"file":"a.go","startLine":"1","endLine":1,"why":"w"}`, "invalid_input", nil},
		{"line as a fraction", "look", `{"file":"a.go","startLine":1.5,"endLine":2,"why":"w"}`, "invalid_input", nil},
		{"file as a number", "look", `{"file":3,"startLine":1,"endLine":1,"why":"w"}`, "invalid_input", nil},
		{"only startLine", "look", `{"file":"a.go","startLine":1,"why":"w"}`, "invalid_input", nil},
		{"only endLine", "look", `{"file":"a.go","endLine":1,"why":"w"}`, "invalid_input", nil},
		{"no lines and no expect", "look", `{"file":"a.go","why":"w"}`, "invalid_input", nil},
		{"expect as a number", "look", `{"file":"a.go","expect":1,"why":"w"}`, "invalid_input", nil},
		{"expect that is not in the file", "look", `{"file":"a.go","expect":"zzz","why":"w"}`, "content_not_found", nil},
		{"expect that is in the file, in other lines", "look", `{"file":"a.go","startLine":1,"endLine":1,"expect":"2","why":"w"}`, "content_mismatch", []string{"1"}},
		{"no arguments", "look", `null`, "invalid_input", nil},
		{"arguments of the wrong shape", "look", `[1]`, "invalid_input", nil},
		{"replace without newText", "edit", `{"selection":"sel_x","why":"w"}`, "invalid_input", nil},
		{"replace with a bad token", "edit", `{"selection":"sel_x","newText":"","why":"w"}`, "invalid_selection", nil},
		{"replace with newText as a number", "edit", `{"selection":"sel_x","newText":1,"why":"w"}`, "invalid_input", nil},
		{"edit with neither selection nor file", "edit", `{"newText":"x","why":"w"}`, "invalid_input", nil},
		{"edit with selection and file", "edit", `{"selection":"sel_x","file":"a.go","expect":"1","newText":"x","why":"w"}`, "invalid_input", nil},
		{"edit with selection and expect", "edit", `{"selection":"sel_x","expect":"1","newText":"x","why":"w"}`, "invalid_input", nil},
		{"edit by file without expect", "edit", `{"file":"a.go","startLine":1,"endLine":1,"newText":"x","why":"w"}`, "invalid_input", nil},
		{"edit by file with only startLine", "edit", `{"file":"a.go","startLine":1,"expect":"1","newText":"x","why":"w"}`, "invalid_input", nil},
		{"edit by file with expect that is not there", "edit", `{"file":"a.go","expect":"zzz","newText":"x","why":"w"}`, "content_not_found", nil},
		{"sub without count", "replace", `{"files":["a.go"],"old":"1","new":"x","why":"w"}`, "invalid_input", nil},
		{"sub with files as a string", "replace", `{"files":"a.go","old":"1","new":"x","count":1,"why":"w"}`, "invalid_input", nil},
		{"sub with count as a string", "replace", `{"files":["a.go"],"old":"1","new":"x","count":"1","why":"w"}`, "invalid_input", nil},
		{"sub with no files", "replace", `{"files":[],"old":"1","new":"x","count":1,"why":"w"}`, "invalid_input", nil},
		{"new without content", "new", `{"file":"n.go","why":"w"}`, "invalid_input", nil},
		{"new with content as a number", "new", `{"file":"n.go","content":1,"why":"w"}`, "invalid_input", nil},
		{"new on an existing file", "new", `{"file":"a.go","content":"x","why":"w"}`, "file_exists", nil},
		{"sub of one place", "replace", `{"files":["a.go"],"old":"1","new":"x","count":1,"why":"w"}`, "use_edit", map[string]any{"hits": []any{map[string]any{"file": "a.go", "startLine": float64(1), "endLine": float64(1), "lines": []any{"1"}}}, "edit": map[string]any{"file": "a.go", "startLine": float64(1), "endLine": float64(1), "expect": "1", "newText": "x"}}},
		{"sub with the wrong count", "replace", `{"files":["a.go"],"old":"1","new":"x","count":2,"why":"w"}`, "count_mismatch", map[string]any{"a.go": float64(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serve(t, root, toolCall(1, tt.tool, tt.args))
			m, isErr := body(t, got[0])
			errObj, _ := m["error"].(map[string]any)
			if !isErr || m["ok"] != false || errObj["code"] != tt.code || errObj["message"] == "" {
				t.Fatalf("got %v (isError %v), want code %s", m, isErr, tt.code)
			}
			if tt.actual != nil {
				if a, _ := json.Marshal(errObj["actual"]); string(a) != mustJSON(tt.actual) {
					t.Errorf("actual = %s, want %s", a, mustJSON(tt.actual))
				}
			} else if _, has := errObj["actual"]; has {
				t.Errorf("actual = %v, want none", errObj["actual"])
			}
		})
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// A call that fails is on the tape as a failure, including one turned away before it reaches select or replace.
func TestFailedCallsAreOnTheTape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("1\n2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, "a.go")
	serve(t, root,
		toolCall(1, "look", `{"file":"a.go","startLine":1,"endLine":1}`),                      // no why
		toolCall(2, "look", `{"file":`+mustJSON(abs)+`,"startLine":1,"endLine":1,"why":"w"}`), // absolute
		toolCall(3, "edit", `{"selection":"sel_x","newText":"SECRET NEW TEXT","why":"w"}`),    // bad token
		toolCall(4, "look", `{"file":"a.go","startLine":"1","endLine":1,"why":"w"}`),          // a line as a string
	)
	files, _ := filepath.Glob(filepath.Join(root, ".srwr", "tapes", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("tapes = %v", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var codes, tools []string
	for _, e := range tape.Parse(b).Events {
		if e.Type == tape.TypeFailure {
			codes, tools = append(codes, e.Failure.Code), append(tools, e.Failure.Tool)
			if e.Failure.File != nil {
				t.Errorf("file = %q, want null for %s", *e.Failure.File, e.Failure.Message)
			}
		}
	}
	if want := "invalid_input invalid_range invalid_selection invalid_input"; strings.Join(codes, " ") != want {
		t.Errorf("codes = %v, want %s", codes, want)
	}
	if want := "look look edit look"; strings.Join(tools, " ") != want {
		t.Errorf("tools = %v, want %s", tools, want)
	}
	if strings.Contains(string(b), root) || strings.Contains(string(b), "SECRET NEW TEXT") {
		t.Errorf("the tape holds a real path or the new text:\n%s", b)
	}
}

// A failure to match that is only about spaces and tabs carries the near places next to actual; other errors do not have the key.
// insert keeps the lines pointed at and puts newText next to them; the schema offers it, and a wrong value is refused.
func TestEditInsert(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, isErr := body(t, serve(t, root, toolCall(1, "edit", `{"file":"a.go","expect":"b","insert":"after","newText":"x","why":"w"}`))[0])
	if isErr || m["startLine"] != float64(3) || m["endLine"] != float64(3) {
		t.Fatalf("got %v", m)
	}
	b, _ := os.ReadFile(filepath.Join(root, "a.go")) //nolint:gosec // a path in a temporary directory
	if string(b) != "a\nb\nx\nc\n" {
		t.Errorf("file = %q", b)
	}
	m, isErr = body(t, serve(t, root, toolCall(1, "edit", `{"file":"a.go","expect":"b","insert":"next","newText":"x","why":"w"}`))[0])
	if e, _ := m["error"].(map[string]any); !isErr || e["code"] != "invalid_input" {
		t.Errorf("got %v", m)
	}
}

// look with search returns every line that holds the text, each with a token; it is refused with a range or expect.
func TestLookSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("a\nfoo(1)\nb\nfoo(2)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, isErr := body(t, serve(t, root, toolCall(1, "look", `{"file":"a.go","search":"foo(","why":"w"}`))[0])
	ms, _ := m["matches"].([]any)
	if isErr || m["count"] != float64(2) || len(ms) != 2 {
		t.Fatalf("got %v", m)
	}
	first, _ := ms[0].(map[string]any)
	if first["startLine"] != float64(2) || first["selection"] == "" || len(first["above"].([]any)) != 1 || len(first["below"].([]any)) != 2 {
		t.Errorf("first = %v", first)
	}
	if _, has := m["more"]; has {
		t.Errorf("more is left out when nothing was left out: %v", m)
	}
	m, isErr = body(t, serve(t, root, toolCall(1, "look", `{"file":"a.go","search":"zzz","why":"w"}`))[0])
	if ms, ok := m["matches"].([]any); isErr || !ok || len(ms) != 0 || m["count"] != float64(0) {
		t.Errorf("no match: got %v", m)
	}
	for _, args := range []string{
		`{"file":"a.go","search":"foo","startLine":1,"endLine":1,"why":"w"}`,
		`{"file":"a.go","search":"foo","expect":"a","why":"w"}`,
		`{"file":"a.go","search":"","why":"w"}`,
	} {
		m, isErr = body(t, serve(t, root, toolCall(1, "look", args))[0])
		if e, _ := m["error"].(map[string]any); !isErr || e["code"] != "invalid_input" {
			t.Errorf("%s: got %v", args, m)
		}
	}
}

// edit with edits makes several edits with one why, all or none.
func TestEditWithEdits(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, isErr := body(t, serve(t, root, toolCall(1, "edit",
		`{"edits":[{"file":"a.go","expect":"d","newText":"D"},{"file":"a.go","expect":"a","newText":"x\ny"},{"file":"a.go","expect":"b","insert":"after","newText":"z"}],"why":"w"}`))[0])
	es, _ := m["edits"].([]any)
	if isErr || len(es) != 3 {
		t.Fatalf("got %v", m)
	}
	if first, _ := es[0].(map[string]any); first["startLine"] != float64(6) || first["selection"] == "" {
		t.Errorf("first = %v", es[0])
	}
	b, _ := os.ReadFile(path) //nolint:gosec // a path in a temporary directory
	if string(b) != "x\ny\nb\nz\nc\nD\n" {
		t.Errorf("file = %q", b)
	}
	for _, args := range []string{
		`{"edits":[{"file":"a.go","expect":"b","newText":"B"}],"newText":"x","why":"w"}`,
		`{"edits":[{"file":"a.go","expect":"b"}],"why":"w"}`,
		`{"edits":[{"file":"a.go","expect":"b","newText":"B"}]}`,
		`{"edits":[],"why":"w"}`,
		`{"edits":[{"file":"a.go","expect":"b","newText":"B"},{"file":"a.go","expect":"b","newText":"C"}],"why":"w"}`,
	} {
		m, isErr = body(t, serve(t, root, toolCall(1, "edit", args))[0])
		if e, _ := m["error"].(map[string]any); !isErr || e["code"] != "invalid_input" {
			t.Errorf("%s: got %v", args, m)
		}
	}
	b, _ = os.ReadFile(path) //nolint:gosec // a path in a temporary directory
	if string(b) != "x\ny\nb\nz\nc\nD\n" {
		t.Errorf("a refused call changed the file: %q", b)
	}
}

func TestNearMatchesInTheError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("func f() {\n\treturn 1\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, tool, args string
		near             bool
	}{
		{"look", "look", `{"file":"a.go","expect":"    return 1","why":"w"}`, true},
		{"edit", "edit", `{"file":"a.go","expect":"    return 1","newText":"x","why":"w"}`, true},
		{"replace", "replace", `{"files":["a.go"],"old":"  return 1","new":"x","count":2,"why":"w"}`, true},
		{"part of a line", "edit", `{"file":"a.go","expect":"return 1","newText":"x","why":"w"}`, true},
		{"nothing near", "look", `{"file":"a.go","expect":"zzz","why":"w"}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, isErr := body(t, serve(t, root, toolCall(1, tt.tool, tt.args))[0])
			errObj, _ := m["error"].(map[string]any)
			near, has := errObj["nearMatches"].([]any)
			if !isErr || has != tt.near {
				t.Fatalf("got %v, want nearMatches: %v", m, tt.near)
			}
			if !tt.near {
				return
			}
			first, _ := near[0].(map[string]any)
			if lines, _ := json.Marshal(first["lines"]); !strings.Contains(string(lines), `\treturn 1`) || first["startLine"] != float64(2) {
				t.Errorf("first = %v", first)
			}
			if strings.Contains(errObj["message"].(string), "return 1") {
				t.Errorf("the message holds the file: %v", errObj["message"])
			}
		})
	}
}
