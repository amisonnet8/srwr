# An example of select → replace

*[日本語](select-replace_ja.md) | **English***

**Readers**: people who use srwr. See, in a real exchange, what the AI sends to `srwr mcp` and what comes back. The rules of the tools are in [mcp.md](../reference/mcp.md).

This example was taken by actually running `srwr mcp`. The values of the selection tokens and the way the version is written change on every run.

## The subject

`cmd/main.go` of the workspace (5 lines).

```go
package main

func main() {
	run()
}
```

The AI calls `select` and `replace` with the MCP `tools/call`. In what follows `→` is a line from the AI (the MCP client) and `←` is a line from `srwr mcp`.

## Look, then change

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2025-11-25","serverInfo":{"name":"srwr","version":"(devel)"}}}
→ {"jsonrpc":"2.0","method":"notifications/initialized"}
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"select","arguments":{"file":"cmd/main.go","startLine":3,"endLine":5,"why":"Check whether a call to initialization can be added to main"}}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041061E48KVH3K24RN324MN2\",\"lines\":[\"func main() {\",\"\\trun()\",\"}\"]}","type":"text"}],"isError":false}}
→ {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"replace","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"func main() {\n\tsetup()\n\trun()\n}","why":"run needs the settings loaded first, so call setup"}}}
← {"jsonrpc":"2.0","id":3,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":3,\"endLine\":6,\"lines\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\",\"}\"],\"before\":[\"package main\",\"\"],\"after\":[]}","type":"text"}],"isError":false}}
```

- `select` (id 2) declares lines 3 to 5, and returns the selection token `sel_…` and the current content of the range, `lines`
- `replace` (id 3) passes that token **as it is**. It passes neither a file nor line numbers
- The response of `replace` is a new token for **the range after the replacement** (lines 3 to 6; it grew by one line). To go on fixing the same place, use this one. It also holds `lines` (what the range is now) and `before` and `after` (the lines around it), so the edit can be checked without reading the file again

As a result, `cmd/main.go` becomes the following 6 lines, and a tape is made in `.srwr/tapes/` ([tape.md](../reference/tape.md); how it replays is in [protocol-session.md](protocol-session.md)).

```go
package main

func main() {
	setup()
	run()
}
```

## Examples of errors

Use the same token (the one the `select` of id 2 returned) again. After the `replace`, that range has changed, so it gives `selection_stale`. The body of the response is `ok: false`, and `isError` is true. `actual` carries the current content.

```jsonrpc
→ {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"replace","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"x","why":"Use the same token again"}}}
← {"jsonrpc":"2.0","id":4,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"selection_stale\",\"message\":\"an edit overlapped the range after the select. Call select again\",\"actual\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\"]}}","type":"text"}],"isError":true}}
```

The AI looks at `actual` and calls `select` again.

Selecting a range beyond the number of lines of the file gives `invalid_range`, and `actual` carries the number of lines.

```jsonrpc
→ {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"select","arguments":{"file":"cmd/main.go","startLine":9,"endLine":9,"why":"Select outside the range"}}}
← {"jsonrpc":"2.0","id":5,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"invalid_range\",\"message\":\"cmd/main.go has 6 lines; startLine=9 endLine=9 is out of range\",\"actual\":{\"lineCount\":6}}}","type":"text"}],"isError":true}}
```

The list of errors is in [mcp.md](../reference/mcp.md).
