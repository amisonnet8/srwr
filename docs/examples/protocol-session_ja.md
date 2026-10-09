# 表示サーバーとのやり取りの例

*[English](protocol-session.md) | **日本語***

**読者**：srwr の表示を、新しいエディタに対応させたい人。クライアントが `srwr view-server` と、どんなやり取りをするかを、実際の出力で見る。決まりは [protocol.md](../reference/protocol_ja.md)。

この例は、`srwr view-server` を実際に動かして取ったもの。バージョンの表記と、テープの更新時刻 `updatedAt` は、環境で変わる。

## 題材

[look-edit.md](look-edit_ja.md) の作業が記録されたテープ `20261001-1706-1795` を持つ作業場（`internal/docs/testdata/demo/`）。操作は2つ（`look` と `edit`）。クライアントは `srwr view-server --root <作業場>` を起動し、標準入力に1行ずつ書き、標準出力から1行ずつ読む。下の `→` がクライアントから、`←` がサーバーからの1行。

## リプレイ

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"client":"vim","protocolVersion":3,"options":{"diffFrames":true}}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":3,"serverVersion":"(devel)"}}
→ {"jsonrpc":"2.0","id":2,"method":"tapes/list","params":{}}
← {"jsonrpc":"2.0","id":2,"result":{"tapes":[{"tapeId":"20261001-1706-1795","startedAt":"2026-10-01T17:06:15.381+09:00","updatedAt":"2026-10-01T08:06:21.026Z","ops":2,"files":["cmd/main.go"]}]}}
→ {"jsonrpc":"2.0","id":3,"method":"tape/open","params":{"tapeId":"20261001-1706-1795"}}
← {"jsonrpc":"2.0","id":3,"result":{"frames":[{"index":0,"kind":"look","seq":2,"ts":1790841975381,"file":"cmd/main.go","range":{"start":3,"end":5},"why":"main に初期化の呼び出しを足せるか確認する","selection":"sel_041061E48KVH3K24RN324MN2","from":null,"parent":null},{"index":1,"kind":"edit","seq":3,"ts":1790841975384,"file":"cmd/main.go","range":{"start":3,"end":6},"oldRange":{"start":3,"end":5},"why":"run の前に設定の読み込みが要るので setup を呼ぶ","selection":"sel_041G61P48KVH3AYWAY6Q99GP","from":"sel_041061E48KVH3K24RN324MN2","parent":0}],"tapeId":"20261001-1706-1795"}}
→ {"jsonrpc":"2.0","id":4,"method":"frame/state","params":{"tapeId":"20261001-1706-1795","index":1}}
← {"jsonrpc":"2.0","id":4,"result":{"after":"package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n","before":"package main\n\nfunc main() {\n\trun()\n}\n","content":"package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n"}}
→ {"jsonrpc":"2.0","id":5,"method":"tape/close","params":{"tapeId":"20261001-1706-1795"}}
← {"jsonrpc":"2.0","id":5,"result":{}}
```

- `initialize`（id 1）：クライアントの種類と、設定（差分のコマ）を渡す。これより前の要求は `not_initialized`
- `tapes/list`（id 2）：操作を1つ以上持つテープの一覧
- `tape/open`（id 3）：**コマの列**。2つのコマが返る。`edit` のコマ（`index` 1）は、`oldRange`（変更前の3〜5行目）と `range`（変更後の3〜6行目）を持ち、`parent` が `look` のコマ（`index` 0）を指す。実ファイルが、テープの最後の内容と同じなので、`final`（最後の差分）のコマは付かない
- `frame/state`（id 4）：`index` 1 のコマの、変更前（`before`）と変更後（`after`）の全文と、そのコマを終えた時点のファイルの内容（`content`）。クライアントは、コマを移るたびにこれを取り、バッファの内容を差し替える
- `tape/close`（id 5）：閉じる

コマの形（`why` の行に使うフィールドなど）は [protocol.md](../reference/protocol_ja.md)、見せ方は [vscode.md](../reference/vscode_ja.md)。

## エラー

存在しないテープを開くと、JSON-RPC のエラーが返る。`error.data.code` が、srwr のエラーコード。

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"client":"vim","protocolVersion":3,"options":{}}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":3,"serverVersion":"(devel)"}}
→ {"jsonrpc":"2.0","id":2,"method":"tape/open","params":{"tapeId":"nothing"}}
← {"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"no such tape: nothing","data":{"code":"tape_not_found"}}}
→ {"jsonrpc":"2.0","id":3,"method":"shutdown","params":{}}
← {"jsonrpc":"2.0","id":3,"result":{}}
```

終わるときは `shutdown`。サーバーは、返事を書いたあとに終了する。

## ライブ

`live/start` の返事のあと、サーバーはテープを見張り、追記されたコマを、返事の要求とは別に、通知 `live/frame` として送る。通知には `id` がない。

```
→ {"jsonrpc":"2.0","id":2,"method":"live/start","params":{}}
← {"jsonrpc":"2.0","id":2,"result":{"tapeId":"…","frames":[ …ここまでのコマ… ]}}
← {"jsonrpc":"2.0","method":"live/frame","params":{"tapeId":"…","frame":{ …追記されたコマ… }}}
```

ライブは、時間に依存する（AI の操作を待つ）ので、この例には載せない。通知の形は [protocol.md](../reference/protocol_ja.md) の `live/frame`。
