# テープ

**読者**：テープを共有する人、テープを読む道具を作る人。再生の仕方は [vscode.md](vscode.md)・[vim.md](vim.md)。

**テープ**は、srwr が記録する操作の列。追記のみの JSONL（1行1イベント）で、`.srwr/tapes/<id>.tape.jsonl` に置かれる。**テープだけで、再生を完全に再現できる**（実ファイルがその後どう変わっても）。共有したいときは、テープそのものを渡す。受け取った人は、自分のエディタで開く。

## ファイル名とセッション

- ファイル名は `<日時>-<短いID>.tape.jsonl`（例：`20260929-1837-1359`）。`.tape.jsonl` を除いたものが**テープID**。最後の短いID（例：`1795`）は `header` の `session` と同じ
- テープは、**最初の操作を記録するときに作る**。何も操作しなければ、空のテープは残らない
- 1本のテープは、作業のひとまとまり（**セッション**）に当たる

今のセッションは、作業場の `.srwr/active`（今のテープID）が指す。**複数の `srwr mcp` を起動しても、同じ作業場なら同じテープに書く**（書き込みは `.srwr/lock` で順番に行う。別のプロセスが書いた分は、書く前にテープから読み足す）。次のどれかのとき、新しいセッション（新しいテープ）になる。

1. 今のセッションがない（`.srwr/active` がない、指すテープがない）
2. 今のセッションの最後のイベントから、一定時間（既定30分）が過ぎた
3. 利用者が `srwr tapes new` を実行した

`srwr hook` も、`srwr mcp` と同じセッション（同じテープ）に書く。

## 書き方の決まり

- すべてのイベントに `"v":1`（形式のバージョン）
- 追記のみ。既存の行を書き換えたり消したりしない
- `seq` はテープ内で1から始まる連番で、欠番がない（`header` は持たない）
- `ts` は RFC 3339 で、ミリ秒まで、タイムゾーン付き
- 値のないフィールド（`why`・`selection`・`from` など）は、省略せず `null`
- 1イベント1行。改行で終わっていない最後の行は、書き込み途中として扱う
- 読む側は、知らないフィールドを無視する。古い読み手を壊さないため、フィールドは足せるが、既存の意味は変えない

## イベント

### header

テープ先頭に1行。

```json
{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T11:20:00+09:00","author":{"kind":"ai","name":"claude"}}
```

そのほか、`vcs`（git の HEAD と、未コミットの変更があるか。git の管理下でなければ `null`）と `tool`（`{"name":"srwr","version":"…"}`）を持つ。

### snapshot

ファイルの**全文**。そのセッションでそのファイルに初めて触れたときと、`external` の直後に記録する。再生は、最後の `snapshot` から、`replace` を順に適用して作る。

```json
{"v":1,"seq":1,"ts":"…","type":"snapshot","file":"cmd/app/main.go","fileHash":"a3f09c21","text":"package main\n…","sha":"sha256:…"}
```

### select

```json
{"v":1,"seq":2,"ts":"…","type":"select","file":"cmd/app/main.go","startLine":12,"endLine":14,"why":"main関数に修正が必要か確認中","selection":"sel_7K3M9QX2F4HD8R1WTB"}
```

hook が記録した `select`（Read など）は、`why` が `null`。`source`（`mcp` または `hook`）と、hook のときの元のツール名 `tool`（`Read`・`Bash`・`Grep`・`Edit`）を持つ。範囲トークンは持たず、`selection` も `null`。`source` のない古いテープは `mcp` として読む。

### replace

```json
{"v":1,"seq":3,"ts":"…","type":"replace","file":"cmd/app/main.go","from":"sel_7K3M9QX2F4HD8R1WTB","startLine":12,"endLine":14,"oldText":"…","newText":"…","newStartLine":12,"newEndLine":15,"selection":"sel_8M1R4TW6ZC2NQ9HXKD","why":"シグナル処理の初期化が漏れていたので追加","fileShaBefore":"sha256:…","fileShaAfter":"sha256:…"}
```

- `from` は入力された範囲トークン、`selection` は返したトークン。`from` → `selection` をたどると、どの `select` からどの `replace` が生まれたかの**系譜**が分かる
- `startLine`・`endLine` は、補正後の実際の範囲
- `oldText`・`newText` は、範囲の行を `\n` でつないだもの（末尾の改行は含まない）。削除は `newEndLine = newStartLine - 1` で、`newText` は空。空行1つは `newEndLine = newStartLine` で `newText` も空なので、行の数は `newStartLine`・`newEndLine` から読む
- 範囲トークンの中の `seq` は、そのトークンを発行したイベントの `seq`
- hook が記録した `replace`（Edit）は、`from`・`selection`・`why` が `null` で、`source` が `hook`、`tool` が `Edit`。範囲は置換位置を含む行全体

### external

srwr の外でファイルが変わったことを検知したとき。直後に、そのファイルの `snapshot` を続けて記録する。

```json
{"v":1,"seq":4,"ts":"…","type":"external","file":"cmd/app/main.go","author":{"kind":"external"},"detectedBy":"select","expectedSha":"sha256:…","actualSha":"sha256:…","text":"package main\n…（変更後の全文）"}
```

- `text`：**変更後のファイル全文**。差分のコマ（左右に並べた diff）として再生するために持つ。ファイルが消えていたときは `null` で、`deleted: true` が付く
- `detectedBy`：検知のきっかけ（`select`・`replace`・`hook`）
- `author.kind` は `external` 固定（誰が変えたかは srwr には分からない）
- `text` のない古い形式の `external` も読める。そのときは、直後の `snapshot` を変更後の内容として見せる

**検知できる範囲**：`external` になるのは、**テープがすでに内容（`snapshot`）を持つファイル**が、あとで食い違ったときだけ。そのセッションで初めて触れるファイルは、そのときの内容が最初の `snapshot` になる。一度も触れないファイルの変更は見えない。

`external` より前に発行した範囲トークンで `replace` すると、内容の照合で `selection_mismatch` になる。外部変更の中身から、行のずれを推定することはしない。

## 範囲トークン

`select` が返す `sel_…` の文字列。`from`・`selection` に入る。AI はそのまま渡すだけで、中身を知らなくてよい。

- **テープの中でだけ有効**。別のテープ（別のセッション）で発行されたものは、`invalid_selection` になる。セッションが替わる（30分空く、など）と、前のセッションのトークンは使えない
- 形式と検証の詳細は、開発者向けの [token.md](../design/token.md)

## 共有するときの注意

- テープは**ファイルの全文**を持つ。秘密情報を含むファイルは記録しない（[cli.md](cli.md)「記録しないファイル」）。共有の前に、中身を確かめる
- 鍵（`.srwr/key`）は、再生に不要。共有しない。表示サーバーもエディタも、鍵を読まない
