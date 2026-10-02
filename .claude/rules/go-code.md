# Go のコード（srwr 固有の決まり）

一般的な lint・整形・テストは `.claude/rules/testing.md` と `.golangci.yaml` に任せる。ここには srwr 固有の決まりだけを書く。前のリポジトリで踏んだ落とし穴を含む。

## 依存

- **外部依存ゼロを保つ。** 範囲トークン（varint・Crockford Base32・HMAC）、テープ（JSONL）、ハッシュ、MCP と表示サーバー（JSON-RPC over stdio）はすべて標準ライブラリで書く
- MCP は公式 SDK を使わず、自前で書く（`initialize`・`ping`・`tools/list`・`tools/call` だけ）
- 改行区切りの JSON-RPC の読み書きは `internal/jsonrpc` に1つだけ置き、`mcp` と `viewserver` が共有する
- ファイルの見張り（ライブ）も外部依存を足さず、テープの大きさのポーリングで行う
- 依存を足すのは重要な判断（`.claude/rules/working-with-human.md`）。足したら `qsoku trivy` を通す

## ファイルとテープへの書き込み

- 実ファイルの書き換えは、**同じディレクトリの一時ファイルに書いて rename** する（権限を保つ）
- **先に実ファイルを書き、そのあとテープに追記する**（`.claude/rules/tape.md`）
- テープへの書き込みは、`internal/session` を通す。ロック（`.srwr/lock`）を取り、今のセッションを決め（`.srwr/active`、最後のイベントから30分空いたら新しいセッション）、前回読んだ位置から後ろのテープを読み足してから、処理を呼ぶ。`mcp` も `hook` も同じ道を通る
- 処理の中で、プロセスのメモリに状態を持たない。状態（最後の seq、ファイルの内容、replace の列）は、テープから読み足したものだけから取る（別のプロセスが書いた分が抜けるため）
- **ファイルを共有して作るもの（鍵 `.srwr/key`、`.srwr/active`）は、一時ファイルに全部書いてから置く。** 前のリポジトリで、2つの `srwr mcp` が同時に起動して、書きかけの鍵を読んで起動に失敗した。鍵は `link`（先に置いた方が勝つ）、`active` は `rename`
- トークンの HMAC に入れるのは、テープID 全体。短いID ではない
- flock は OS ごとにビルドタグで分け（`//go:build unix` と `//go:build windows`）、cgo を使わない。**手元でも `qsoku cross` を通す**（前は Windows の CI で初めて落ちた）
- 表示サーバー（`viewserver`・`timeline`）はテープを**読むだけ**。ロックを取らず、書き込み途中の最後の行は保留する
- 表示サーバーが「今のファイル」を読むのは、最後の差分のときだけ。**テープに書かれたパスは信用しない**（共有されたテープが任意のファイルを読ませないよう、作業場の外を指すパス・シンボリックリンクは「存在しない」として扱う）。`tapeId` も裸の名前だけを受ける
- サーバーからの通知（ライブ）を書く goroutine は、接続が終わるときに必ず止めて待つ。通知は、その要求への返事を書いたあとに始める

## エラー

- エラーは握りつぶさない。`docs/reference/mcp.md`・`protocol.md` のエラーコードは定数として定義し、応答までそのまま伝える
- `panic` は使わない（プログラミングミスの検出を除く）
- `why` は入力スキーマとサーバーの両方で検査する（空白だけも不可）

## 記録しないファイル

- `select`・`replace`・hook・external の検知の**すべての入口で**、`internal/ignore` を通す。1か所でも漏れると、秘密情報がテープに入る
- 除外の判定を足したら、4つの入口それぞれについてテストを書く

## テスト

- 表駆動。`t.TempDir()` の中で行い、リポジトリ内のファイルを書き換えない
- `internal/core` の行番号補正は、境界（範囲より上・下・重なり・空範囲・末尾への追記・`external` をまたぐ）をすべてテストで押さえる
- `internal/tape` は `extension/test/fixtures/*.expected.json`、`internal/timeline` は `extension/test/golden/` の全本と一致することを確かめる（`.claude/rules/tape.md`）
- セッションとロック・鍵の作成は、**最初から**2つのプロセス（またはゴルーチン）が同時に動くテストで押さえ、`qsoku race` を通す
- `srwr view` のテストは、`XDG_CACHE_HOME` を `t.TempDir()` に向ける（サンドボックスでは `~/.cache` が読み取り専用）
- わざとロジックを壊してテストが落ちることを確かめる（`.claude/rules/testing.md`「壊して確かめる」）
