# テープ

*[English](tape.md) | **日本語***

**読者**：テープを共有する人、テープを読む道具を作る人。再生の仕方は [vscode.md](vscode_ja.md)・[vim.md](vim_ja.md)。

**テープ**は、srwr が記録する操作の列。追記のみの JSONL（1行1イベント）で、`.srwr/tapes/<id>.tape.jsonl` に置かれる。**テープだけで、再生を完全に再現できる**（実ファイルがその後どう変わっても）。共有したいときは、テープそのものを渡す。受け取った人は、自分のエディタで開く。

## ファイル名とセッション

- ファイル名は `<日時>-<短いID>.tape.jsonl`（例：`20260929-0237-1359`）。日時は **UTC**。`.tape.jsonl` を除いたものが**テープID**。最後の短いID（例：`1359`）は `header` の `session` と同じ
- テープは、**最初の操作を記録するときに作る**。何も操作しなければ、空のテープは残らない
- 1本のテープは、作業のひとまとまり（**セッション**）に当たる

今のセッションは、作業場の `.srwr/active`（今のテープID）が指す。**複数の `srwr mcp` を起動しても、同じ作業場なら同じテープに書く**（書き込みは `.srwr/lock` で順番に行う。別のプロセスが書いた分は、書く前にテープから読み足す）。次のどれかのとき、新しいセッション（新しいテープ）になる。

1. 今のセッションがない（`.srwr/active` がない、指すテープがない）
2. 今のセッションの最後のイベントから、一定時間（既定30分）が過ぎた
3. 利用者が `srwr tapes new` を実行した

`srwr hook` も、`srwr mcp` と同じセッション（同じテープ）に書く。

## 閉じたテープは圧縮する

セッションが終わると、そのテープは **gzip で圧縮**され、`<id>.tape.jsonl.gz` という名前になる。テープIDは同じで、中身も同じ JSONL（`zcat` で読める）。圧縮するのは、次のセッションを始める側（間が空いたあとの書き込み、または `srwr tapes new`）で、ロックを持っている間に行う。だから、書き込み中のテープが圧縮されることはない。今のセッションのテープと、次に書き込むまでの最後のテープは、そのままの形で残る。

- テープを読むものは、どちらの形も開く。途中で落ちて、圧縮していないテープと圧縮したテープが両方残ったときは、圧縮していない方が正
- 閉じたテープには追記しない。圧縮したバイト列は中身だけで決まり（gzip のヘッダーに時刻も名前も入れない）、ファイルの更新時刻は圧縮前のテープのものを引き継ぐ
- 圧縮は、場所を節約するだけ（テープの大半は、各ファイルに最初に触れたときの全文で、約3分の1になる）。失敗したら、圧縮していないテープが残り、ほかには何も起きない
- これより前に作ったテープは、そのまま。変換はしない

## 書き方の決まり

- すべてのイベントに `"v":1`（形式のバージョン）
- 追記のみ。既存の行を書き換えたり消したりしない
- `seq` はテープ内で1から始まる連番で、欠番がない（`header` は持たない）
- `ts` は RFC 3339 の **UTC** で、ミリ秒まで、末尾は `Z`（`2026-09-29T02:20:04.123Z`）。古い版が書いたテープには `+09:00` のようなオフセット（そのときの機械の時間帯）が付いている。同じ瞬間として読み、古い行を書き換えることはしない
- 値のないフィールド（`why`・`selection`・`from` など）は、省略せず `null`。例外は、任意の `source`・`tool`（空なら書かない）と `deleted`（真のときだけ書く）
- 1イベント1行。改行で終わっていない最後の行は、書き込み途中として扱う
- 読む側は、知らないフィールドを無視する
- **v1 までは、テープの形式を互換なしで変えることがある。** ある版が書いたテープを別の版が読めることは、約束しない。v1 からは、フィールドは足せるが、既存の意味は変えない

## イベント

### header

テープ先頭に1行。

```json
{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T02:20:00.000Z","author":{"kind":"ai","name":"claude"}}
```

そのほか、`vcs` と `tool`（`{"name":"srwr","version":"…"}`）を持つ。

`vcs` は、テープを作った（セッションの最初の記録をした）ときの git の状態。あとで変わっても書き換えない。

```json
"vcs":{"type":"git","head":"3f2a…（40桁の16進）","dirty":true}
```

- `head`：HEAD のコミット。コミットがまだ無いリポジトリでは `null`
- `dirty`：コミットしていない変更があるか。追跡中のファイルの変更・ステージ・削除と、`.gitignore` で無視されていない新しいファイル。作業場の下だけを見る。**`.srwr/` の中は数えない**（テープ自身で常に変更ありになるため）
- git の管理下でないとき、git が使えないとき（入っていない、拒否された、5秒で終わらない）は `null`。AI の作業は止めない
- ブランチ名やリモートの URL は書かない（テープは共有されるため）。読む側は、`vcs` の知らない項目を無視する

### snapshot

ファイルの**全文**。そのセッションでそのファイルに初めて触れたときと、`external` で消えたファイルが戻ったときに記録する。それ以後は、変わった行だけを持つ `replace` と `external` で追う。再生は、最後の `snapshot` から、それらを順に適用して作る。

```json
{"v":1,"seq":1,"ts":"…","type":"snapshot","file":"cmd/app/main.go","fileHash":"a3f09c21","text":"package main\n…","sha":"sha256:…"}
```

### select

```json
{"v":1,"seq":2,"ts":"…","type":"select","file":"cmd/app/main.go","startLine":12,"endLine":14,"why":"main関数に修正が必要か確認中","selection":"sel_0410R3GZE4KV11C6325D32S7","source":"mcp"}
```

hook が記録した `select`（Read など）は、`why` が `null`。`source`（`mcp` または `hook`）と、hook のときの元のツール名 `tool`（`Read`・`Bash`・`Grep`・`Edit`）を持つ。範囲トークンは持たず、`selection` も `null`。`source` のない古いテープは `mcp` として読む。

### replace

```json
{"v":1,"seq":3,"ts":"…","type":"replace","file":"cmd/app/main.go","from":"sel_0410R3GZE4KV11C6325D32S7","startLine":12,"endLine":14,"oldText":"…","newText":"…","newStartLine":12,"newEndLine":15,"selection":"sel_041GR3RZE4KV0BBH2S177Q36","why":"シグナル処理の初期化が漏れていたので追加","fileShaBefore":"sha256:…","fileShaAfter":"sha256:…","source":"mcp"}
```

- `from` は入力された範囲トークン、`selection` は返したトークン。`from` → `selection` をたどると、どの `select` からどの `replace` が生まれたかの**系譜**が分かる
- `startLine`・`endLine` は、補正後の実際の範囲
- `oldText`・`newText` は、範囲の行を `\n` でつないだもの（末尾の改行は含まない）。削除は `newEndLine = newStartLine - 1` で、`newText` は空。空行1つは `newEndLine = newStartLine` で `newText` も空なので、行の数は `newStartLine`・`newEndLine` から読む
- 範囲トークンの中の `seq` は、そのトークンを発行したイベントの `seq`
- hook が記録した `replace`（Edit）は、`from`・`selection`・`why` が `null` で、`source` が `hook`、`tool` が `Edit`。範囲は置換位置を含む行全体

### external

srwr の外でファイルが変わったことを検知したとき。`snapshot` は続けない。何が変わったかは、この行が持つ。

```json
{"v":1,"seq":4,"ts":"…","type":"external","file":"cmd/app/main.go","author":{"kind":"external"},"detectedBy":"select","expectedSha":"sha256:…","actualSha":"sha256:…","hunks":[{"startLine":3,"endLine":3,"newText":"b","newStartLine":3,"newEndLine":3},{"startLine":40,"endLine":41,"newText":"x\ny","newStartLine":40,"newEndLine":41}]}
```

- `hunks`：**変わった行**。テープが直前に持っていた内容（`expectedSha` のハッシュの内容）に対するもの。箇所は上から順で、重ならない。`startLine`・`endLine` は変更前の行、`newText` は変更後の行（`\n` でつなぐ）、`newStartLine`・`newEndLine` はその行番号で、`replace` と同じ。挿入は `endLine = startLine - 1`、削除は `newText` が空で `newEndLine = newStartLine - 1`。変更前の内容に当てると、変更後の内容（`actualSha` のハッシュ）になる。差分のコマ（左右に並べた diff）として再生する
- `text`：**変更後のファイル全文**。行で表せない変更（ファイル末尾の改行だけが変わった）や、比べるには大きすぎる変更のとき、`hunks` の代わりに書く。ファイルが消えていたときは `null`、`actualSha` は空文字列で、`deleted: true` が付く
- `created`：ファイルが**新しい**とき（テープがその内容を持たず、シェルのコマンドが作った）に `true` で書く。`expectedSha` は空文字列で、`hunks`（または `text`）がファイル全体を持つので、左が空の差分のコマとして再生する。見つけるのは hook だけ（[cli_ja.md](cli_ja.md) の `srwr hook`）
- `detectedBy`：検知のきっかけ（`select`・`replace`・`hook`）
- `author.kind` は `external` 固定（誰が変えたかは srwr には分からない）
- 古い形式の `external` も読める：全文の `text` を持ち直後に `snapshot` が続くもの、`text` のないもの（そのときは、直後の `snapshot` を変更後の内容として見せる）

**検知できる範囲**：`external` になるのは、**テープがすでに内容（`snapshot`）を持つファイル**が、あとで食い違ったときだけ。そのセッションで初めて触れるファイルは、そのときの内容が最初の `snapshot` になる。一度も触れないファイルの変更は見えない（ただし、Bash の後に git の作業ツリーにある**新しい**ファイルは、`created` つきの `external` として記録する）。

`external` より前に発行した範囲トークンで `replace` したときも、ふつうに追う（行番号を補正するのは、srwr 自身の編集だけ）。外部変更が、範囲の中身も、範囲より上の行数も変えていなければ、そのトークンは使える。変えていれば、内容の照合で `selection_mismatch` になる。外部変更の中身から、行のずれを推定することはしない。

### failure

`select`・`replace` が失敗したとき（AI がエラーを受け取ったとき）に記録する。AI がどんなミスをするかを知るためのもの。ビューワーは、頼まれたときだけコマとして出す（赤。[vscode_ja.md](vscode_ja.md)）。ふだんは出さない。

```json
{"v":1,"seq":7,"ts":"…","type":"failure","tool":"select","file":null,"startLine":3,"endLine":9,"selection":null,"why":"main 関数を確認する","code":"invalid_range","message":"The path is absolute. Give a path relative to the workspace"}
{"v":1,"seq":9,"ts":"…","type":"failure","tool":"replace","file":"cmd/app/main.go","startLine":null,"endLine":null,"selection":"sel_0410R3GZE4KV11C6325D32S7","why":"…","code":"selection_stale","message":"an edit overlapped the range after the select. Call select again"}
```

- `tool`：`select` または `replace`。`code` と `message` は、AI が受け取ったエラー（[mcp_ja.md](mcp_ja.md)）
- `file`：作業場の中のパス。分からないときと、伏せるときは `null`。`startLine`・`endLine` は `select` のとき、`selection`（渡されたトークン）は `replace` のとき。`why` は AI が書いたもの。値がないものは `null`
- **実際のパスは伏せる**：絶対パス、作業場の外を指すパス、記録しないファイルのとき、`file` は `null` で、`message` はパスを含まない文にする（`The path is absolute. Give a path relative to the workspace`、`The path points outside the workspace`、`The file is not recorded`）
- `replace` の `newText` は書かない。`message` は 300 文字で切る
- srwr の編集に届く前に見つかる失敗（必須の入力がない、値の型が違う）も記録する。`file` は `null`

## 範囲トークン

`select` が返す `sel_…` の文字列。`from`・`selection` に入る。AI はそのまま渡すだけで、中身を知らなくてよい。

- **テープの中でだけ有効**。別のテープ（別のセッション）で発行されたものは、`invalid_selection` になる。セッションが替わる（30分空く、など）と、前のセッションのトークンは使えない
- 形式と検証の詳細は、開発者向けの [token.md](../design/token_ja.md)

## 共有するときの注意

- テープは**ファイルの全文**を持つ。秘密情報を含むファイルは記録しない（[cli.md](cli_ja.md)「記録しないファイル」）。共有の前に、中身を確かめる
- 鍵（`.srwr/key`）は、再生に不要。共有しない。表示サーバーもエディタも、鍵を読まない
