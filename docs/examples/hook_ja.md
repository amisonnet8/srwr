# hook の例

[English](hook.md)

**読者**：srwr を使う人、Claude Code の hook を登録する人。`srwr hook` が、Claude Code が渡す JSON から何を読み、テープに何を書くかを、実際の出力で見る。決まりは [cli.md](../reference/cli_ja.md)、テープの形は [tape.md](../reference/tape_ja.md)。

この例は、`srwr hook` を実際に動かして取ったもの。時刻 `ts` は、実行のたびに変わる。

## 題材

作業場 `/work` の `cmd/main.go`（5行）。

```go
package main

func main() {
	run()
}
```

AI は `srwr mcp` を使わずに、Claude Code が持つ道具で、ファイルを読み（Read）、行を絞って見て（Bash の `sed -n`）、直した（Edit）。Claude Code は、道具を使い終えるたびに、`srwr hook` の標準入力へ JSON を1つ渡す（`PostToolUse`）。下の `→` がその JSON の1つ。ここでは読みやすいように、使う項目だけを書いている。

## Claude Code が渡すもの

```hook
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Read","tool_input":{"file_path":"/work/cmd/main.go"}}
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Bash","tool_input":{"command":"sed -n '3,5p' cmd/main.go"},"tool_response":{"stdout":"func main() {\n\trun()\n}\n"}}
→ {"hook_event_name":"PostToolUse","cwd":"/work","tool_name":"Edit","tool_input":{"file_path":"/work/cmd/main.go","old_string":"\trun()","new_string":"\tsetup()\n\trun()","replace_all":false},"tool_response":{"filePath":"/work/cmd/main.go","originalFile":"package main\n\nfunc main() {\n\trun()\n}\n"}}
```

- Read は、`offset`・`limit` がなければファイル全体の `select`
- Bash は `sed -n '3,5p'` を読み取りと見て、3〜5行目の `select`。ほかに `cat`・`nl`・`head`・`tail`・`grep -n` を読む（[cli.md](../reference/cli_ja.md)）。Bash のあとは、テープが内容を持つ全ファイルを読み直す
- Edit は、編集前の内容（`originalFile`）に `old_string` → `new_string` を当てて今のファイルと一致を確かめ、置換位置を含む行全体の `replace`。この例では `run()` の1行（4行目）が、2行（4〜5行目）になる

## できるテープ

最初に触れたファイルの `snapshot` に続いて、3つの操作が並ぶ。どれも `source` が `hook` で、`tool` に元の道具の名前が入る。`why`・`selection`・`from` は `null`（理由の行が出ない）。`srwr mcp` の `select` / `replace` も、同じテープに並ぶ。

```jsonl
{"v":1,"seq":1,"ts":"2026-10-03T05:00:00.000Z","type":"snapshot","file":"cmd/main.go","fileHash":"c444f711","text":"package main\n\nfunc main() {\n\trun()\n}\n","sha":"sha256:4fdbdb618bff60f0d8482b0a3394db33e9af6dd728c041d220aeecfd15247e2f"}
{"v":1,"seq":2,"ts":"2026-10-03T05:00:00.000Z","type":"select","file":"cmd/main.go","startLine":1,"endLine":5,"why":null,"selection":null,"source":"hook","tool":"Read"}
{"v":1,"seq":3,"ts":"2026-10-03T05:00:00.000Z","type":"select","file":"cmd/main.go","startLine":3,"endLine":5,"why":null,"selection":null,"source":"hook","tool":"Bash"}
{"v":1,"seq":4,"ts":"2026-10-03T05:00:00.000Z","type":"replace","file":"cmd/main.go","from":null,"startLine":4,"endLine":4,"oldText":"\trun()","newText":"\tsetup()\n\trun()","newStartLine":4,"newEndLine":5,"selection":null,"why":null,"fileShaBefore":"sha256:4fdbdb618bff60f0d8482b0a3394db33e9af6dd728c041d220aeecfd15247e2f","fileShaAfter":"sha256:8e04cee4c24d4e79bb05712d7c98c504ef535fa5f3cb970a5ed69ef4fcf78f59","source":"hook","tool":"Edit"}
```

編集後の `cmd/main.go` は、次の6行。

```go
package main

func main() {
	setup()
	run()
}
```
