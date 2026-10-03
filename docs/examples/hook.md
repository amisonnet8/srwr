# An example of the hook

[日本語](hook_ja.md)

**Readers**: people who use srwr, and people who register the hook of Claude Code. See, in real output, what `srwr hook` reads from the JSON that Claude Code hands over and what it writes on the tape. The rules are in [cli.md](../reference/cli.md), and the shape of the tape is in [tape.md](../reference/tape.md).

This example was taken by actually running `srwr hook`. The time `ts` changes on every run.

## The subject

`cmd/main.go` of the workspace `/work` (5 lines).

```go
package main

func main() {
	run()
}
```

The AI did not use `srwr mcp`. With the tools that Claude Code has, it read the file (Read), looked at some lines (Bash `sed -n`), and fixed it (Edit). Each time a tool has finished, Claude Code hands one JSON to the standard input of `srwr hook` (`PostToolUse`). In what follows `→` is one of those JSONs. To make it easy to read, only the items that are used are written here.

## What Claude Code hands over

```hook
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Read","tool_input":{"file_path":"/work/cmd/main.go"}}
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Bash","tool_input":{"command":"sed -n '3,5p' cmd/main.go"},"tool_response":{"stdout":"func main() {\n\trun()\n}\n"}}
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Edit","tool_input":{"file_path":"/work/cmd/main.go","old_string":"\trun()","new_string":"\tsetup()\n\trun()","replace_all":false},"tool_response":{"filePath":"/work/cmd/main.go","originalFile":"package main\n\nfunc main() {\n\trun()\n}\n"}}
```

- A Read is a `select` of the whole file when there is no `offset` or `limit`
- Bash `sed -n '3,5p'` is seen as a read and becomes a `select` of lines 3 to 5. Others read are `cat`, `nl`, `head`, `tail` and `grep -n` ([cli.md](../reference/cli.md)). After a Bash command, every file whose content the tape holds is read again
- An Edit applies `old_string` → `new_string` to the content before the edit (`originalFile`), checks that it matches the current file, and becomes a `replace` of the whole lines that contain the replaced place. In this example the one line `run()` (line 4) becomes two lines (lines 4 to 5)

## The tape that results

After the `snapshot` of the file touched first, the three operations follow. All of them have a `source` of `hook`, and `tool` holds the name of the original tool. `why`, `selection` and `from` are `null` (no reason line is shown). The `select` / `replace` of `srwr mcp` line up on the same tape.

```jsonl
{"v":1,"seq":1,"ts":"2026-10-03T05:00:00.000Z","type":"snapshot","file":"cmd/main.go","fileHash":"c444f711","text":"package main\n\nfunc main() {\n\trun()\n}\n","sha":"sha256:4fdbdb618bff60f0d8482b0a3394db33e9af6dd728c041d220aeecfd15247e2f"}
{"v":1,"seq":2,"ts":"2026-10-03T05:00:00.000Z","type":"select","file":"cmd/main.go","startLine":1,"endLine":5,"why":null,"selection":null,"source":"hook","tool":"Read"}
{"v":1,"seq":3,"ts":"2026-10-03T05:00:00.000Z","type":"select","file":"cmd/main.go","startLine":3,"endLine":5,"why":null,"selection":null,"source":"hook","tool":"Bash"}
{"v":1,"seq":4,"ts":"2026-10-03T05:00:00.000Z","type":"replace","file":"cmd/main.go","from":null,"startLine":4,"endLine":4,"oldText":"\trun()","newText":"\tsetup()\n\trun()","newStartLine":4,"newEndLine":5,"selection":null,"why":null,"fileShaBefore":"sha256:4fdbdb618bff60f0d8482b0a3394db33e9af6dd728c041d220aeecfd15247e2f","fileShaAfter":"sha256:8e04cee4c24d4e79bb05712d7c98c504ef535fa5f3cb970a5ed69ef4fcf78f59","source":"hook","tool":"Edit"}
```

After the edit, `cmd/main.go` is the following 6 lines.

```go
package main

func main() {
	setup()
	run()
}
```
