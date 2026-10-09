package viewserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// exchange sends the requests (one JSON object per line) to a Server in one connection and returns
// the lines it wrote, in order. The input ends after the last request, so a live view is stopped then.
func exchange(t *testing.T, srv *Server, requests ...string) []string {
	t.Helper()
	var out bytes.Buffer
	if err := srv.Serve(strings.NewReader(strings.Join(requests, "\n")+"\n"), &out); err != nil {
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

// req makes a request line.
func req(id int, method string, params any) string {
	p, _ := json.Marshal(params)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, p)
}

func initReq(id int, options map[string]any) string {
	return req(id, "initialize", map[string]any{"client": "vim", "protocolVersion": 3, "options": options})
}

// resultOf decodes the result of a response line, or fails if it is an error.
func resultOf(t *testing.T, line string, into any) {
	t.Helper()
	var r struct {
		Result json.RawMessage
		Error  *struct {
			Code    int
			Message string
			Data    map[string]string
		}
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	if r.Error != nil {
		t.Fatalf("error response: %s", line)
	}
	if err := json.Unmarshal(r.Result, into); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
}

// errorOf decodes an error response and returns its JSON-RPC code and error.data.code.
func errorOf(t *testing.T, line string) (rpcCode int, dataCode string) {
	t.Helper()
	var r struct {
		Result json.RawMessage
		Error  *struct {
			Code    int
			Message string
			Data    map[string]string
		}
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	if r.Error == nil || r.Error.Message == "" {
		t.Fatalf("not an error response: %s", line)
	}
	return r.Error.Code, r.Error.Data["code"]
}
