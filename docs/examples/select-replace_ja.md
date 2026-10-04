# select → replace の例

*[English](select-replace.md) | **日本語***

**読者**：srwr を使う人。AI が `srwr mcp` に何を送り、何が返るかを、実際のやり取りで見る。ツールの決まりは [mcp.md](../reference/mcp_ja.md)。

この例は、`srwr mcp` を実際に動かして取ったもの。範囲トークンの値と、バージョンの表記は、実行のたびに変わる。

## 題材

作業場の `cmd/main.go`（5行）。

```go
package main

func main() {
	run()
}
```

AI は MCP の `tools/call` で、`select` と `replace` を呼ぶ。下の `→` が AI（MCP クライアント）から、`←` が `srwr mcp` からの1行。

## 見る → 変える

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2025-11-25","serverInfo":{"name":"srwr","version":"(devel)"}}}
→ {"jsonrpc":"2.0","method":"notifications/initialized"}
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"select","arguments":{"file":"cmd/main.go","startLine":3,"endLine":5,"why":"main に初期化の呼び出しを足せるか確認する"}}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041061E48KVH3K24RN324MN2\",\"startLine\":3,\"endLine\":5,\"lines\":[\"func main() {\",\"\\trun()\",\"}\"]}","type":"text"}],"isError":false}}
→ {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"replace","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"func main() {\n\tsetup()\n\trun()\n}","why":"run の前に設定の読み込みが要るので setup を呼ぶ"}}}
← {"jsonrpc":"2.0","id":3,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":3,\"endLine\":6,\"lines\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\",\"}\"],\"before\":[\"package main\",\"\"],\"after\":[]}","type":"text"}],"isError":false}}
```

- `select`（id 2）は、3〜5行目を宣言し、範囲トークン `sel_…` と、範囲の現在の内容 `lines` を返す
- `replace`（id 3）は、そのトークンを**そのまま**渡す。ファイルや行番号は渡さない
- `replace` の応答は、**置き換え後の範囲**（3〜6行目。1行増えた）の新しいトークン。続けて直すなら、これを使う。`lines`（範囲の今の内容）と、前後の行 `before`・`after` も付くので、ファイルを読み直さずに結果を確かめられる

この結果、`cmd/main.go` は次の6行になり、`.srwr/tapes/` にテープができる（[tape.md](../reference/tape_ja.md)、再生の様子は [protocol-session.md](protocol-session_ja.md)）。

```go
package main

func main() {
	setup()
	run()
}
```

## エラーの例

同じトークン（id 2 の `select` が返したもの）を、もう一度使う。`replace` のあと、その範囲は変わっているので、`selection_stale` になる。応答の本文は `ok: false` で、（エラーの文言は AI が読むもので、画面の言語に関わらず英語）`isError` が真になる。`actual` に、今の内容が付く。

```jsonrpc
→ {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"replace","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"x","why":"同じトークンをもう一度使う"}}}
← {"jsonrpc":"2.0","id":4,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"selection_stale\",\"message\":\"an edit overlapped the range after the select. Call select again\",\"actual\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\"]}}","type":"text"}],"isError":true}}
```

AI は、`actual` を見て、`select` し直す。

ファイルの行数を超える範囲を選ぶと、`invalid_range` になり、`actual` に行数が付く。

```jsonrpc
→ {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"select","arguments":{"file":"cmd/main.go","startLine":9,"endLine":9,"why":"範囲の外を選ぶ"}}}
← {"jsonrpc":"2.0","id":5,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"invalid_range\",\"message\":\"cmd/main.go has 6 lines; startLine=9 endLine=9 is out of range\",\"actual\":{\"lineCount\":6}}}","type":"text"}],"isError":true}}
```

エラーの一覧は [mcp.md](../reference/mcp_ja.md)。
