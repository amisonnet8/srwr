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
		toolCall(2, "select", `{"file":"f.txt","startLine":1,"endLine":1,"why":"w"}`))
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
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edit","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":"x"}`,
	)
	want := []string{
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found: resources/list"}}`,
		`{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"unknown tool: edit"}}`,
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
	if len(r.Result.Tools) != 2 || r.Result.Tools[0].Name != "select" || r.Result.Tools[1].Name != "replace" {
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
	if !contains(r.Result.Tools[0].InputSchema.Required, "file") || !contains(r.Result.Tools[1].InputSchema.Required, "selection") {
		t.Error("required lists are wrong")
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

	sel, isErr := call(1, "select", `{"file":"a.go","startLine":2,"endLine":3,"why":"見る"}`)
	if isErr || sel["ok"] != true || len(sel["lines"].([]any)) != 2 {
		t.Fatalf("select = %v %v", sel, isErr)
	}
	rep, isErr := call(2, "replace", `{"selection":"`+sel["selection"].(string)+`","newText":"a < b && c > d","why":"変える"}`)
	if isErr || rep["ok"] != true || rep["startLine"] != float64(2) || rep["endLine"] != float64(2) {
		t.Fatalf("replace = %v %v", rep, isErr)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.go")); string(b) != "1\na < b && c > d\n" { //nolint:gosec // a path in a temporary directory
		t.Errorf("file = %q", b)
	}

	// A select with only expect finds its own range, and says where.
	found, isErr := call(4, "select", `{"file":"a.go","expect":"a < b && c > d","why":"探す"}`)
	if isErr || found["startLine"] != float64(2) || found["endLine"] != float64(2) {
		t.Errorf("select by expect = %v %v", found, isErr)
	}

	// The result holds the range now and the lines around it; what is empty is [] and not null.
	for key, want := range map[string]int{"lines": 1, "before": 1, "after": 0} {
		if l, ok := rep[key].([]any); !ok || len(l) != want {
			t.Errorf("%s of the replace = %#v, want %d lines", key, rep[key], want)
		}
	}
	if l := rep["lines"].([]any); l[0] != "a < b && c > d" {
		t.Errorf("lines of the replace = %#v", l)
	}

	// An empty range gives [] and not null.
	empty, _ := call(3, "select", `{"file":"a.go","startLine":3,"endLine":2,"why":"w"}`)
	if l, ok := empty["lines"].([]any); !ok || len(l) != 0 {
		t.Errorf("lines of an empty range = %#v, want []", empty["lines"])
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
		{"range", "select", `{"file":"a.go","startLine":9,"endLine":9,"why":"w"}`, "invalid_range", map[string]any{"lineCount": float64(2)}},
		{"missing file", "select", `{"file":"no.go","startLine":1,"endLine":1,"why":"w"}`, "file_not_found", nil},
		{"missing why", "select", `{"file":"a.go","startLine":1,"endLine":1}`, "invalid_input", nil},
		{"blank why", "select", `{"file":"a.go","startLine":1,"endLine":1,"why":"  "}`, "invalid_input", nil},
		{"missing file argument", "select", `{"startLine":1,"endLine":1,"why":"w"}`, "invalid_input", nil},
		{"line as a string", "select", `{"file":"a.go","startLine":"1","endLine":1,"why":"w"}`, "invalid_input", nil},
		{"line as a fraction", "select", `{"file":"a.go","startLine":1.5,"endLine":2,"why":"w"}`, "invalid_input", nil},
		{"file as a number", "select", `{"file":3,"startLine":1,"endLine":1,"why":"w"}`, "invalid_input", nil},
		{"only startLine", "select", `{"file":"a.go","startLine":1,"why":"w"}`, "invalid_input", nil},
		{"only endLine", "select", `{"file":"a.go","endLine":1,"why":"w"}`, "invalid_input", nil},
		{"no lines and no expect", "select", `{"file":"a.go","why":"w"}`, "invalid_input", nil},
		{"expect as a number", "select", `{"file":"a.go","expect":1,"why":"w"}`, "invalid_input", nil},
		{"expect that is not in the file", "select", `{"file":"a.go","expect":"zzz","why":"w"}`, "content_not_found", nil},
		{"expect that is in the file, in other lines", "select", `{"file":"a.go","startLine":1,"endLine":1,"expect":"2","why":"w"}`, "content_mismatch", []string{"1"}},
		{"no arguments", "select", `null`, "invalid_input", nil},
		{"arguments of the wrong shape", "select", `[1]`, "invalid_input", nil},
		{"replace without newText", "replace", `{"selection":"sel_x","why":"w"}`, "invalid_input", nil},
		{"replace with a bad token", "replace", `{"selection":"sel_x","newText":"","why":"w"}`, "invalid_selection", nil},
		{"replace with newText as a number", "replace", `{"selection":"sel_x","newText":1,"why":"w"}`, "invalid_input", nil},
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
		toolCall(1, "select", `{"file":"a.go","startLine":1,"endLine":1}`),                      // no why
		toolCall(2, "select", `{"file":`+mustJSON(abs)+`,"startLine":1,"endLine":1,"why":"w"}`), // absolute
		toolCall(3, "replace", `{"selection":"sel_x","newText":"SECRET NEW TEXT","why":"w"}`),   // bad token
		toolCall(4, "select", `{"file":"a.go","startLine":"1","endLine":1,"why":"w"}`),          // a line as a string
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
	if want := "select select replace select"; strings.Join(tools, " ") != want {
		t.Errorf("tools = %v, want %s", tools, want)
	}
	if strings.Contains(string(b), root) || strings.Contains(string(b), "SECRET NEW TEXT") {
		t.Errorf("the tape holds a real path or the new text:\n%s", b)
	}
}
