# look → edit の例

*[English](look-edit.md) | **日本語***

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

AI は MCP の `tools/call` で、`look` と `edit` を呼ぶ。下の `→` が AI（MCP クライアント）から、`←` が `srwr mcp` からの1行。

## 見る → 変える

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2025-11-25","serverInfo":{"name":"srwr","version":"(devel)"}}}
→ {"jsonrpc":"2.0","method":"notifications/initialized"}
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"look","arguments":{"file":"cmd/main.go","startLine":3,"endLine":5,"why":"main に初期化の呼び出しを足せるか確認する"}}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041061E48KVH3K24RN324MN2\",\"startLine\":3,\"endLine\":5,\"lines\":[\"func main() {\",\"\\trun()\",\"}\"]}","type":"text"}],"isError":false}}
→ {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edit","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"func main() {\n\tsetup()\n\trun()\n}","why":"run の前に設定の読み込みが要るので setup を呼ぶ"}}}
← {"jsonrpc":"2.0","id":3,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":3,\"endLine\":6,\"lines\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\",\"}\"],\"above\":[\"package main\",\"\"],\"below\":[]}","type":"text"}],"isError":false}}
```

- `look`（id 2）は、3〜5行目を宣言し、範囲トークン `sel_…` と、範囲の現在の内容 `lines` を返す
- `edit`（id 3）は、そのトークンを**そのまま**渡す。ファイルや行番号は渡さない
- `edit` の応答は、**置き換え後の範囲**（3〜6行目。1行増えた）の新しいトークン。続けて直すなら、これを使う。`lines`（範囲の今の内容）と、前後の行 `above`・`below`（今のファイルの行） も付くので、ファイルを読み直さずに結果を確かめられる

この結果、`cmd/main.go` は次の6行になり、`.srwr/tapes/` にテープができる（[tape.md](../reference/tape_ja.md)、再生の様子は [protocol-session.md](protocol-session_ja.md)）。

```go
package main

func main() {
	setup()
	run()
}
```

## エラーの例

同じトークン（id 2 の `look` が返したもの）を、もう一度使う。`edit` のあと、その範囲は変わっているので、`selection_stale` になる。応答の本文は `ok: false` で、（エラーの文言は AI が読むもので、画面の言語に関わらず英語）`isError` が真になる。`actual` に、今の内容が付く。

```jsonrpc
→ {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"edit","arguments":{"selection":"sel_041061E48KVH3K24RN324MN2","newText":"x","why":"同じトークンをもう一度使う"}}}
← {"jsonrpc":"2.0","id":4,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"selection_stale\",\"message\":\"an edit overlapped the range after the look. Call look again\",\"actual\":[\"func main() {\",\"\\tsetup()\",\"\\trun()\"]}}","type":"text"}],"isError":true}}
```

AI は、`actual` を見て、`look` し直す。

ファイルの行数を超える範囲を選ぶと、`invalid_range` になり、`actual` に行数が付く。

```jsonrpc
→ {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"look","arguments":{"file":"cmd/main.go","startLine":9,"endLine":9,"why":"範囲の外を選ぶ"}}}
← {"jsonrpc":"2.0","id":5,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"invalid_range\",\"message\":\"cmd/main.go has 6 lines; startLine=9 endLine=9 is out of range\",\"actual\":{\"lineCount\":6}}}","type":"text"}],"isError":true}}
```

## トークンなしの edit

`edit` は、範囲を自分で指すこともできる。`file` と、範囲が今持っている行 `expect` を渡す。行番号（`startLine`・`endLine`）は省いてよく、あっても手がかりにすぎない。違っていれば、srwr が `expect` の行を探し、1か所だけなら、そこを範囲にする。同じファイルへの edit を、`look` を挟まずに、順不同でまとめて送れる。

ここでは行番号が違う（`\trun()` は4行目でなく5行目）が、srwr が見つける。

```jsonrpc
→ {"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","startLine":4,"endLine":4,"expect":"\trun()","newText":"\trun()\n\tcleanup()","why":"run のあとに、setup で作ったものを片付ける"}}}
← {"jsonrpc":"2.0","id":6,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_041G61P48KVH3AYWAY6Q99GP\",\"startLine\":5,\"endLine\":6,\"lines\":[\"\\trun()\",\"\\tcleanup()\"],\"above\":[\"func main() {\",\"\\tsetup()\"],\"below\":[\"}\"]}","type":"text"}],"isError":false}}
```

ファイルは7行になる。

```go
package main

func main() {
	setup()
	run()
	cleanup()
}
```

## 行が合わないとき

`expect` が、ファイルと空白（タブ・スペース）だけ違うとき（ここでは、タブのところにスペース4つを書いた）、何も変えず、エラーの `nearMatches` に、空白を除けば同じ場所を、今の行のまま入れる。`message` に書くのは場所だけで、中身は書かない。`lines` から `expect` を写して、もう一度呼ぶ。

```jsonrpc
→ {"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","expect":"    run()","newText":"    run()\n    wait()","why":"run の終わりを待つ"}}}
← {"jsonrpc":"2.0","id":7,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"content_not_found\",\"message\":\"the lines of expect are not in cmd/main.go (7 lines). Check the content, or call look again. Line 5 differs from expect only in spaces or tabs: see nearMatches\",\"nearMatches\":[{\"startLine\":5,\"endLine\":5,\"lines\":[\"\\trun()\"]}]}}","type":"text"}],"isError":true}}
```

## 1か所は `edit` で

`replace` は2か所以上のためのもの。1か所の文字列には `use_edit` を返し、何も変えない。`actual.edit` は、同じ変更をする `edit` の呼び出しで、`why` だけを足せばよい。

```jsonrpc
→ {"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"replace","arguments":{"files":["cmd/main.go"],"old":"cleanup()","new":"teardown()","count":1,"why":"cleanup の名前を替える"}}}
← {"jsonrpc":"2.0","id":8,"result":{"content":[{"text":"{\"ok\":false,\"error\":{\"code\":\"use_edit\",\"message\":\"replace is for 2 or more places, and the text is in one place only (cmd/main.go line 6). Use edit for it: actual.edit is the call to make (add why)\",\"actual\":{\"edit\":{\"file\":\"cmd/main.go\",\"startLine\":6,\"endLine\":6,\"expect\":\"\\tcleanup()\",\"newText\":\"\\tteardown()\"},\"hits\":[{\"file\":\"cmd/main.go\",\"startLine\":6,\"endLine\":6,\"lines\":[\"\\tcleanup()\"]}]}}}","type":"text"}],"isError":true}}
```

## 行の前後に足す

行を変えずに、その後ろ（前）に行を足すには、いつもと同じに行を指して `insert: "after"`（または `"before"`）を付ける。`expect` を確かめるので、ファイルがどう変わっていても使える。返るのは、足した行のこと。

```jsonrpc
→ {"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"edit","arguments":{"file":"cmd/main.go","expect":"\tsetup()","insert":"after","newText":"\tcheck()","why":"run の前に確認する"}}}
← {"jsonrpc":"2.0","id":9,"result":{"content":[{"text":"{\"ok\":true,\"selection\":\"sel_044GA1E48KVH269F6JJ94T8Z\",\"startLine\":5,\"endLine\":5,\"lines\":[\"\\tcheck()\"],\"above\":[\"func main() {\",\"\\tsetup()\"],\"below\":[\"\\trun()\",\"\\tcleanup()\"]}","type":"text"}],"isError":false}}
```

ファイルは8行になった。`setup()` はそのままで、その後ろに `check()` が入っている。

```go
package main

func main() {
	setup()
	check()
	run()
	cleanup()
}
```

エラーの一覧は [mcp.md](../reference/mcp_ja.md)。
