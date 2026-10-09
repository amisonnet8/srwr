# An example of look → edit

*[日本語](look-edit_ja.md) | **English***

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

The AI calls `look` and `edit` with the MCP `tools/call`. In what follows `→` is a line from the AI (the MCP client) and `←` is a line from `srwr mcp`.

## Look, then change

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2025-11-25","serverInfo":{"name":"srwr","version":"(devel)"}}}
→ {"jsonrpc":"2.0","method":"notifications/initialized"}
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"look","arguments":{"file":"cmd/main.go","startLine":3,"endLine":5,"why":"Check whether a call to initialization can be added to main"}}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041061E48KVH3K24RN324MN2\",\"startLine\":3,\"endLine\":5,\"lines\":[\"func main() {\",\"\\trun()\",\"}\"]}","type":"text"}],"isError":false}}
→ {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edit","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"func main() {\n\tsetup()\n\trun()\n}","why":"run needs the settings loaded first, so call setup"}}}
← {"jsonrpc":"2.0","id":3,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":3,\"endLine\":6,\"lines\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\",\"}\"],\"above\":[\"package main\",\"\"],\"below\":[]}","type":"text"}],"isError":false}}
```

- `look` (id 2) declares lines 3 to 5, and returns the selection token `sel_…` and the current content of the range, `lines`
- `edit` (id 3) passes that token **as it is**. It passes neither a file nor line numbers
- The response of `edit` is a new token for **the range after the replacement** (lines 3 to 6; it grew by one line). To go on fixing the same place, use this one. It also holds `lines` (what the range is now) and `above` and `below` (the lines just above and below it, as the file is now), so the edit can be checked without reading the file again

As a result, `cmd/main.go` becomes the following 6 lines, and a tape is made in `.srwr/tapes/` ([tape.md](../reference/tape.md); how it replays is in [protocol-session.md](protocol-session.md)).

```go
package main

func main() {
	setup()
	run()
}
```

## Examples of errors

Use the same token (the one the `look` of id 2 returned) again. After the `edit`, that range has changed, so it gives `selection_stale`. The body of the response is `ok: false`, and `isError` is true. `actual` carries the current content.

```jsonrpc
→ {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"edit","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"x","why":"Use the same token again"}}}
← {"jsonrpc":"2.0","id":4,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"selection_stale\",\"message\":\"an edit overlapped the range after the look. Call look again\",\"actual\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\"]}}","type":"text"}],"isError":true}}
```

The AI looks at `actual` and calls `look` again.

Looking at a range beyond the number of lines of the file gives `invalid_range`, and `actual` carries the number of lines.

```jsonrpc
→ {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"look","arguments":{"file":"cmd/main.go","startLine":9,"endLine":9,"why":"Select outside the range"}}}
← {"jsonrpc":"2.0","id":5,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"invalid_range\",\"message\":\"cmd/main.go has 6 lines; startLine=9 endLine=9 is out of range\",\"actual\":{\"lineCount\":6}}}","type":"text"}],"isError":true}}
```

## Edit without a token

`edit` can also point at the range by itself: pass `file` and `expect`, the lines the range holds now. The line numbers (`startLine`, `endLine`) are optional, and only a hint: if they are wrong, srwr looks for the lines of `expect`, and takes the place if there is exactly one. Several edits to one file can be sent together this way, in any order, without a `look` for each.

Here the line numbers are wrong (`\trun()` is on line 5, not 4), and srwr finds it:

```jsonrpc
→ {"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","startLine":4,"endLine":4,"expect":"\trun()","newText":"\trun()\n\tcleanup()","why":"Release what setup made, after run"}}}
← {"jsonrpc":"2.0","id":6,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":5,\"endLine\":6,\"lines\":[\"\\trun()\",\"\\tcleanup()\"],\"above\":[\"func main() {\",\"\\tsetup()\"],\"below\":[\"}\"]}","type":"text"}],"isError":false}}
```

The file is now 7 lines.

```go
package main

func main() {
	setup()
	run()
	cleanup()
}
```

## When the lines do not match

If `expect` differs from the file only in spaces or tabs (here four spaces were written for a tab), nothing is changed, and the error lists the places that are the same but for that in `nearMatches`, with their lines as they are. `retry` is the call to make again. The message says only where, not what. Copy `expect` from `lines` and call again.

```jsonrpc
→ {"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","expect":"    run()","newText":"    run()\n    wait()","why":"Wait for run to finish"}}}
← {"jsonrpc":"2.0","id":7,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"content_not_found\",\"message\":\"the lines of expect are not in cmd/main.go (7 lines). Check the content, or call look again. Line 5 differs from expect only in spaces or tabs: see nearMatches\",\"nearMatches\":[{\"startLine\":5,\"endLine\":5,\"lines\":[\"\\trun()\"]}],\"retry\":{\"endLine\":5,\"expect\":\"\\trun()\",\"file\":\"cmd/main.go\",\"newText\":\"    run()\\n    wait()\",\"startLine\":5}}}","type":"text"}],"isError":true}}
```

## Insert next to lines

To put new lines after (or before) lines without changing them, point at those lines as usual and add `insert: "after"` (or `"before"`). `expect` is checked, so this works however the file has changed. The result is about the new lines.

```jsonrpc
→ {"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","expect":"\tsetup()","insert":"after","newText":"\tcheck()","why":"Check before run"}}}
← {"jsonrpc":"2.0","id":8,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_044GA1E48KVH269F6JJ94T8Z\",\"startLine\":5,\"endLine\":5,\"lines\":[\"\\tcheck()\"],\"above\":[\"func main() {\",\"\\tsetup()\"],\"below\":[\"\\trun()\",\"\\tcleanup()\"]}","type":"text"}],"isError":false}}
```

The file is now 8 lines; `setup()` is kept and `check()` follows it.

```go
package main

func main() {
	setup()
	check()
	run()
	cleanup()
}
```

## Search for a place

To find a place, `look` with `search` instead of line numbers returns every line that holds the text (up to 20), each with its own selection token that `edit` takes as it is. The lines are looked at, so each one is on the tape with the `why`; nothing is changed.

```jsonrpc
→ {"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"look","arguments":{"file":"cmd/main.go","search":"up()","why":"Find where setup and cleanup are called"}}}
← {"jsonrpc":"2.0","id":9,"result":{"content":[{"text":"{\"ok\":true,\"count\":2,\"matches\":[{\"selection\":\"sel_045081648KVH3Z5DC93GTFB3\",\"startLine\":4,\"endLine\":4,\"lines\":[\"\\tsetup()\"],\"above\":[\"\",\"func main() {\"],\"below\":[\"\\tcheck()\",\"\\trun()\"]},{\"selection\":\"sel_045GE1Y48KVH3MCC2C6EXB6B\",\"startLine\":7,\"endLine\":7,\"lines\":[\"\\tcleanup()\"],\"above\":[\"\\tcheck()\",\"\\trun()\"],\"below\":[\"}\"]}]}","type":"text"}],"isError":false}}
```

## Several edits in one call

Edits that share a reason go in one call, with `edits` and one `why`. Each item points at its lines as `edit` does. All the ranges are found as the file is before the call, so the items need no order and none depends on another; they must not overlap. All are made, or none. The result has one entry for each item, in your order, with the line numbers after the call.

```jsonrpc
→ {"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"edit","arguments":{"edits":[{"file":"cmd/main.go","expect":"\tsetup()","newText":"\tstart()"},{"file":"cmd/main.go","expect":"\tcleanup()","newText":"\tstop()"},{"file":"cmd/main.go","expect":"\trun()","insert":"after","newText":"\tlog()"}],"why":"Rename the calls and log the run"}}}
← {"jsonrpc":"2.0","id":10,"result":{"content":[{"text":"{\"ok\":true,\"edits\":[{\"selection\":\"sel_046081648KVH3M6NE7HJW31K\",\"startLine\":4,\"endLine\":4,\"lines\":[\"\\tstart()\"],\"above\":[\"\",\"func main() {\"],\"below\":[\"\\tcheck()\",\"\\trun()\"]},{\"selection\":\"sel_0470G2648KVH257ZXQFSTS3K\",\"startLine\":8,\"endLine\":8,\"lines\":[\"\\tstop()\"],\"above\":[\"\\trun()\",\"\\tlog()\"],\"below\":[\"}\"]},{\"selection\":\"sel_046GE1Y48KVH28BMME71JYMC\",\"startLine\":7,\"endLine\":7,\"lines\":[\"\\tlog()\"],\"above\":[\"\\tcheck()\",\"\\trun()\"],\"below\":[\"\\tstop()\",\"}\"]}]}","type":"text"}],"isError":false}}
```

## A part of a line

To change a few words, give `file`, `old` and `new` instead of `expect` and `newText`. `old` is a text that is in the file in one place; it need not be whole lines. The range is the lines it touches, and the result is the same as for any edit.

```jsonrpc
→ {"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","old":"stop()","new":"stop(true)","why":"Stop with a flag"}}}
← {"jsonrpc":"2.0","id":11,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_047GG2648KVH3FFGG4WYYXPE\",\"startLine\":8,\"endLine\":8,\"lines\":[\"\\tstop(true)\"],\"above\":[\"\\trun()\",\"\\tlog()\"],\"below\":[\"}\"],\"hint\":\"Edits to one file in a row: edit with edits makes them in one call (one why, all or none; the ranges are found as the file is now, so no order is needed).\"}","type":"text"}],"isError":false}}
```

The file is now 9 lines.

```go
package main

func main() {
	start()
	check()
	run()
	log()
	stop(true)
}
```

The list of errors is in [mcp.md](../reference/mcp.md).
