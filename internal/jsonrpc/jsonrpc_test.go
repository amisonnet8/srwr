package jsonrpc

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
)

// run feeds input to Serve with a handler that echoes the method, and returns the lines written.
func run(t *testing.T, input string, h Handler) []string {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), NewWriter(&out), h); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var lines []string
	for l := range strings.SplitSeq(out.String(), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func echo(req Request) (any, *Error) {
	switch req.Method {
	case "fail":
		return nil, &Error{Code: InvalidParams, Message: "bad"}
	case "empty":
		return nil, nil
	}
	return map[string]string{"method": req.Method}, nil
}

func TestServe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"result", `{"jsonrpc":"2.0","id":1,"method":"a"}` + "\n", []string{`{"jsonrpc":"2.0","id":1,"result":{"method":"a"}}`}},
		{"string id is kept as is", `{"jsonrpc":"2.0","id":"x-1","method":"a"}` + "\n", []string{`{"jsonrpc":"2.0","id":"x-1","result":{"method":"a"}}`}},
		{"null id is a request", `{"jsonrpc":"2.0","id":null,"method":"a"}` + "\n", []string{`{"jsonrpc":"2.0","id":null,"result":{"method":"a"}}`}},
		{"notification gets no response", `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n", nil},
		{"error", `{"jsonrpc":"2.0","id":2,"method":"fail"}` + "\n", []string{`{"jsonrpc":"2.0","id":2,"error":{"code":-32602,"message":"bad"}}`}},
		{"nil result is an empty object", `{"jsonrpc":"2.0","id":3,"method":"empty"}` + "\n", []string{`{"jsonrpc":"2.0","id":3,"result":{}}`}},
		{"in order", `{"jsonrpc":"2.0","id":1,"method":"a"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"b"}` + "\n",
			[]string{`{"jsonrpc":"2.0","id":1,"result":{"method":"a"}}`, `{"jsonrpc":"2.0","id":2,"result":{"method":"b"}}`}},
		{"last line without newline", `{"jsonrpc":"2.0","id":1,"method":"a"}`, []string{`{"jsonrpc":"2.0","id":1,"result":{"method":"a"}}`}},
		{"blank lines and CRLF", "\n\r\n" + `{"jsonrpc":"2.0","id":1,"method":"a"}` + "\r\n", []string{`{"jsonrpc":"2.0","id":1,"result":{"method":"a"}}`}},
		{"not json", "nope\n", []string{`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error: invalid character 'o' in literal null (expecting 'u')"}}`}},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"method":"a"}` + "\n", []string{`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"not a JSON-RPC 2.0 request"}}`}},
		{"no method", `{"jsonrpc":"2.0","id":1}` + "\n", []string{`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"not a JSON-RPC 2.0 request"}}`}},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"a"}]` + "\n", []string{`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch requests are not supported"}}`}},
		{"keeps going after a bad line", "nope\n" + `{"jsonrpc":"2.0","id":1,"method":"a"}` + "\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.input, echo)
			if tt.name == "keeps going after a bad line" {
				if len(got) != 2 || !strings.Contains(got[1], `"result"`) {
					t.Errorf("got %q", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d: %q", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d:\n got %s\nwant %s", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLongLine(t *testing.T) {
	// A replace with a large file in newText can be well over bufio.Scanner's 64 KiB default.
	big := strings.Repeat("x", 5<<20)
	var got string
	h := func(req Request) (any, *Error) {
		var p struct{ Text string }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &Error{Code: InvalidParams, Message: err.Error()}
		}
		got = p.Text
		return nil, nil
	}
	lines := run(t, `{"jsonrpc":"2.0","id":1,"method":"m","params":{"text":"`+big+`"}}`+"\n", h)
	if got != big || len(lines) != 1 {
		t.Errorf("long line: got %d bytes, %d responses", len(got), len(lines))
	}
}

func TestWriterDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := NewWriter(&buf).Write(map[string]string{"t": "a < b && c > d"}); err != nil {
		t.Fatal(err)
	}
	if want := `{"t":"a < b && c > d"}` + "\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

type countingWriter struct {
	mu     sync.Mutex
	writes [][]byte
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

func TestWriterWritesEachMessageInOneCall(t *testing.T) {
	cw := &countingWriter{}
	w := NewWriter(cw)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.Write(map[string]int{"n": i})
		}()
	}
	wg.Wait()
	if len(cw.writes) != 50 {
		t.Fatalf("%d writes, want 50", len(cw.writes))
	}
	for _, p := range cw.writes {
		if !json.Valid(bytes.TrimSpace(p)) || bytes.Count(p, []byte("\n")) != 1 || p[len(p)-1] != '\n' {
			t.Errorf("a write is not exactly one line: %q", p)
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestServeReturnsReadError(t *testing.T) {
	if err := Serve(failingReader{}, NewWriter(io.Discard), echo); err == nil {
		t.Error("Serve should return a read error")
	}
}

type afterResult struct{ after func() bool }

func (a afterResult) After() bool { return a.after() }

func TestAfterwardRunsAfterTheResponseIsWritten(t *testing.T) {
	var out bytes.Buffer
	var seenAtAfter string
	h := func(req Request) (any, *Error) {
		switch req.Method {
		case "start":
			return afterResult{after: func() bool { seenAtAfter = out.String(); return false }}, nil
		case "fail":
			return nil, &Error{Code: InvalidParams, Message: "bad"}
		}
		return struct{}{}, nil
	}
	in := `{"jsonrpc":"2.0","id":1,"method":"start"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"other"}` + "\n"
	if err := Serve(strings.NewReader(in), NewWriter(&out), h); err != nil {
		t.Fatal(err)
	}
	if want := `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"; seenAtAfter != want {
		t.Errorf("when After ran, the output was %q, want %q (the response is written first)", seenAtAfter, want)
	}
	if n := strings.Count(out.String(), "\n"); n != 2 {
		t.Errorf("%d lines written, want 2", n)
	}
}

func TestAfterwardCanEndServe(t *testing.T) {
	var out bytes.Buffer
	h := func(req Request) (any, *Error) {
		if req.Method == "shutdown" {
			return afterResult{after: func() bool { return true }}, nil
		}
		return struct{}{}, nil
	}
	in := `{"jsonrpc":"2.0","id":1,"method":"shutdown"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"late"}` + "\n"
	if err := Serve(strings.NewReader(in), NewWriter(&out), h); err != nil {
		t.Fatal(err)
	}
	if want := `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"; out.String() != want {
		t.Errorf("output = %q, want only the response to shutdown", out.String())
	}
}

func TestAfterwardIsNotRunForAnError(t *testing.T) {
	ran := false
	h := func(Request) (any, *Error) {
		return afterResult{after: func() bool { ran = true; return true }}, &Error{Code: InvalidParams, Message: "bad"}
	}
	if err := Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"x"}`+"\n"), NewWriter(io.Discard), h); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("After ran although the request failed")
	}
}

func TestNotify(t *testing.T) {
	var buf bytes.Buffer
	if err := NewWriter(&buf).Notify("live/frame", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if want := `{"jsonrpc":"2.0","method":"live/frame","params":{"n":1}}` + "\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
