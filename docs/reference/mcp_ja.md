# MCP ツール（look・edit・new）

*[English](mcp.md) | **日本語***

**読者**：srwr を使う人。AI エージェントに何をさせ、どんなエラーが返るかを知りたい人。

AI エージェントは、MCP サーバー `srwr mcp` が提供する **3つのツール**だけでファイルを編集する。

| ツール | やること |
|---|---|
| `look` | 範囲を見る。範囲トークンが返る |
| `edit` | 範囲トークンの範囲を、新しいテキストに置き換える |
| `new` | まだないファイルを、内容つきで作る |

`look` を経ない `edit` はない。そのため、テープには「見る → 変える」が必ず揃って残る。同じ変更を何か所もするときは `edits`（1つの `why`、全部成功か何も変えない）を使う。検索や読み取りは、AI がもともと持っている Read・grep に任せる（hook が記録する。[cli.md](cli_ja.md) の `srwr hook`）。

3つとも `why`（理由）が必須。人が [テープ](tape_ja.md) を再生するときの中心になる。

ツールの説明とエラーの文言は、AI が読むものなので、画面の言語に関わらず英語。

## look

見ている範囲を宣言し、その範囲を編集するための**範囲トークン**を返す。

```jsonc
// 入力
{ "file": "cmd/app/main.go", "startLine": 12, "endLine": 14, "why": "main関数に修正が必要か確認中" }
// 出力
{ "ok": true, "selection": "sel_0410R3GZE4KV11C6325D32S7", "startLine": 12, "endLine": 14, "lines": ["func main() {", "…", "}"] }
```

| 項目 | 意味 |
|---|---|
| `file` | 作業場からの相対パス。既存のファイルだけ（新しいファイルは `new` で作る） |
| `startLine`・`endLine` | 1始まり、両端を含む行番号。2つとも渡すか、2つとも渡さない（渡さないときは `expect` が範囲を見つける。`expect` もなければファイル全体を返す。2000行を超えるファイルは先頭の2000行で、結果に `note` が付く） |
| `expect` | 任意。範囲がこの内容であること。行を `\n` でつなぐ。下の「内容の確認」 |
| `search` | 任意。範囲の代わりに、ファイルから探す文字列。下の「検索」 |
| `include`・`exclude`・`offset` | 任意。`search` と一緒に、探すファイルを選び、当たりを順に読む。下の「検索」 |
| `looks` | 任意。`file` の代わりに、複数のファイルを1回で。下の「1回で複数のファイル」 |
| `why` | なぜここを見るか |
| `selection` | 範囲トークン。AI はそのまま `edit` に渡す（[範囲トークンとは](../design/token_ja.md)） |
| 出力の `startLine`・`endLine` | 選んだ範囲（`expect` で見つけたときは、その場所） |
| `lines` | 範囲の現在の内容。常に返る |
| 出力の `lineCount`・`note` | ファイルの終わりを越えた `endLine` を最後の行に切ったときだけ付く。ファイルの行数と、そう言う1文 |

- **挿入位置**：`endLine = startLine - 1` の空範囲は「`startLine` 行目の直前」。たとえば `startLine: 13, endLine: 12` は12行目と13行目の間。ファイルの末尾への追記は `startLine = 行数 + 1`
- 範囲の条件：`1 ≤ startLine ≤ 行数 + 1`、`startLine - 1 ≤ endLine ≤ 行数`。外れると `invalid_range`。**ただし、look は読むだけなので例外がある。** `startLine` がファイルの中（または終わりの次）にあって `endLine` がファイルの終わりを越えるときは、最後の行に切って返し、結果がそう言う（`lineCount`・`note`）。テープには見た範囲が残る。`edit` は切らない
- 作業場の外を指すパス（`../`、絶対パス）も `invalid_range`

### 内容の確認（`expect`）

`expect` は範囲の内容を、行ごとに書いたもの。行を `\n` でつなぐ（数え方は `newText` と同じ。`""` は0行、最後の `\n` は数えない）。空白も照合する。範囲は行単位なので、行の一部は探さない。

| 呼び方 | 動き |
|---|---|
| `startLine`・`endLine` だけ | 今までどおり |
| `startLine`・`endLine` と `expect` | 範囲が `expect` の行とまったく同じときだけ通る。違えば `content_mismatch`。行番号がずれたことに気づける |
| `expect` だけ | `expect` の連続した行を、ファイルから探す。ちょうど1か所なら、そこが範囲。なければ `content_not_found`、2か所以上なら `content_ambiguous` |

- 行番号も `expect` もないときはファイル全体を読む。`startLine` と `endLine` の片方だけのときは、`invalid_input`。行番号なしで `expect` が `""` のときもそう（挿入位置は、行番号で指す）
- `content_mismatch` は、同じ行がファイルのどこにあるかを言う（最大5か所）。たいていは、それが直し方になる。`content_ambiguous` は、当たった場所を言う（最大10か所）。行番号を付けるか、`expect` の行を増やす
- `expect` はテープに書かない

### 検索（`search`）

`file` と `search` を渡し、`startLine`・`endLine`・`expect` は渡さないと、その文字列を含む行を全部返す。AI は、別の道具でファイルを読まなくても、場所を見つけて、そのまま編集できる。

```jsonc
// 入力
{ "file": "cmd/main.go", "search": "up()", "why": "setup と cleanup の呼び出しを探す" }
// 出力
{ "ok": true, "count": 2, "matches": [
  { "selection": "sel_…", "startLine": 4, "endLine": 4, "lines": ["\tsetup()"], "above": ["", "func main() {"], "below": ["\tcheck()", "\trun()"] },
  { "selection": "sel_…", "startLine": 7, "endLine": 7, "lines": ["\tcleanup()"], "above": ["\tcheck()", "\trun()"], "below": ["}"] } ] }
```

- **ディレクトリ**：`search` のとき、`file` にディレクトリ（`"."` は作業場全体）を渡せる。その下の、git が数えるファイル（追跡中のものと、無視されていない新しいもの。git がなければ、名前が `.` で始まらないディレクトリのすべてのファイル）を、パスの順に探し、当たりごとに `file` が付く。記録しないファイル・テキストでないもの・1 MiB を超えるものは、何も言わずに飛ばす。探すのは5000ファイルまで（超えたら `note` が言う）。20件は全体で20件で、`count` は文字列を含む行の全部の数。当たりのあったファイルだけが、look のほかにテープにスナップショットを書く。存在しないものは今までどおり `file_not_found`
- `search` はただの文字列（正規表現ではない）で、1行の中のもの（改行を含む・空は `invalid_input`）。大文字小文字と空白は区別する。1行に何度あっても、その行は1件
- 当たりは、**その1行の `look`** になる。それぞれに `selection`（そのまま `edit` に渡せる）と、`above`・`below`（ファイルの前後それぞれ最大2行）が付く。並びは行の順。`count` は当たった行の数。返す（テープに書く）のは20件まで。載せなかった行の数が `more`（なければ付かない）。当たりがないのはエラーではなく、`matches` が `[]` で、テープには何も書かない
- **ファイルを選ぶ**（`include`・`exclude`）：`.gitignore` の書き方のパターンの配列（文字列1つは、1つの配列として受ける）（`"*.go"`・`"internal/"`・`"docs/**/*.md"`。`/` のないパターンは、どの深さでも名前に当たる。大文字小文字は区別しない）。`include` のどれかに当たり（`include` がなければ全部）、`exclude` のどれにも当たらないファイルを探す。1つのファイルを渡したときも効き、通らなければ当たりは0件。空のパターンは無視する
- **長い行**：当たった行が200文字より長いときは、文字列の最初の場所の前後80文字に切って返し、切ったところに `…` を付ける。200文字を超える `above`・`below` の行は、先頭の200文字に切る。そのとき当たりに `cut: true` が付く。`selection` と、テープに書くものは、行全体のまま
- **20件を超えるとき**：`offset` 件の当たりを（上の順で）飛ばして、次の20件を返す。`more` はその後に残る数（`count - offset - 返した数`）。テープに書くのは返した当たりだけ。ディレクトリで、当たりを全部は返さなかったときは、`byFile` が、当たりの多いファイルを `[{"file", "count"}, …]` の形で20件まで返す。AI は `include`・`exclude` で絞り直せる
- `include` の `!` で始まるパターン、`exclude` の最初が `!` で始まるパターンは `invalid_input`（何にも当たらないため）。除きたいファイルは、`!` なしで `exclude` に書く。`exclude` の途中の `!` は、前のパターンを取り消す。
- `startLine`・`endLine`・`expect` と一緒なら `invalid_input`。`search` なしの `include`・`exclude`・`offset`、負の `offset` も同じ。探した文字列はテープに書かない。断られるファイルは、ふつうの `look` と同じ

### 1回で複数のファイル（`looks`）

`looks` は1〜10件の項目を持ち、各項目は `file` と、必要なら `startLine`・`endLine`（2つとも渡す。渡さなければファイル全体）。`file`・`startLine`・`endLine`・`expect`・`search` の代わりに渡し、`why` は1つ。

```jsonc
// 入力
{ "looks": [ { "file": "a.go" }, { "file": "b.go", "startLine": 10, "endLine": 40 } ], "why": "インターフェースを定義する2つのファイルを読む" }
// 出力
{ "ok": true, "looks": [ { "file": "a.go", "selection": "sel_…", "startLine": 1, "endLine": 30, "lines": ["…"] }, … ] }
```

- **全部行うか、何も行わないか**：テープに書く前に、全項目を確かめる。悪い項目（存在しない・記録しないファイル、ファイルの外の範囲、合計2000行を超える）は、その項目のエラーで、`message` の頭に `looks[1]: `、末尾に「Nothing was looked at」（英語）が付く。書く `failure` は、その項目についての1件。終わりを越えた `endLine` は、1つの `look` と同じく切る
- 各項目はテープの `look` になり（`why` は同じ）、それぞれの `selection` を持つ。結果は渡した順

## edit

範囲を新しいテキストに、1回の呼び出しで置き換える。挿入は空範囲への置き換え、削除は `newText` を空文字列にすることで表す。範囲の指し方は2つ：`look` の**範囲トークン**（ふつうの流れ：look して edit）か、トークンなしの **`file` と `expect`**。

```jsonc
// 入力
{ "selection": "sel_0410R3GZE4KV11C6325D32S7", "newText": "func main() {\n    setupSignals()\n    …\n}", "why": "シグナル処理の初期化が漏れていたので追加" }
// 出力
{ "ok": true, "selection": "sel_041GR3RZE4KV0BBH2S177Q36", "startLine": 12, "endLine": 15,
  "lines": ["func main() {", "    setupSignals()", "    …", "}"], "above": ["", "// main starts the app."], "below": ["", "func run() {"] }
```

| 項目 | 意味 |
|---|---|
| `selection` | `look`、`edit`、`new` が返したトークン。ファイルはここから決まる（`file` は不要）。これか、`file` と `expect` のどちらか（両方は不可） |
| `file`・`startLine`・`endLine`・`expect` | トークンなしのとき：ファイル、AI が見た範囲（行番号は両方か、どちらもなし）、その範囲が今持っている行（`\n` でつなぐ。`look` と同じ）。下を参照 |
| `old`・`new` | `expect` と `newText` の代わりに、行の一部を直すとき：`file` と一緒に、ファイルの中にただ1か所ある文字列 `old` と、その代わりの `new`。下を見る。`selection`・`expect`・`newText`・`insert` とは一緒に使えない |
| `newText` | 置き換え後のテキスト。`""` は削除 |
| `insert` | 省略可。`"after"` か `"before"`。範囲（トークンの範囲、または `expect` の行）を残し、その後ろ（前）に `newText` を足す。置き換えない。`newText` が空なら空行を1つ足す。`"start"`・`"end"` は、`file` と `newText` だけで、ファイルの先頭（末尾）に `newText` を足す |
| `brief` | 省略可。`true`（真偽値。引用符は付けない）で、結果は `selection`・`startLine`・`endLine`（と `hint`）だけ。`lines`・`above`・`below` を返さない。読み返さなくてよい多数の編集のため。`edits` でも使える。`edits` が6件以上のときは既定でこの短い形（長い形が要るときは `false`） |
| `why` | なぜこう変えるか |
| 出力の `selection` | **置き換え後の範囲**の新しいトークン。同じ箇所を続けて直すときは、`look` し直さずにこれを使える |
| 出力の `lines` | 置き換え後の範囲の内容（削除なら `[]`） |
| 出力の `hint` | 次の2つのときに付く1行。この編集が、同じファイルへの編集の直後（間にテープのイベントがない）のとき：`edits` を知らせる（1件の編集だけ）。2行以上を挿入して、上（下）の行との間に空行がなく、その行が空でなく字下げが同じとき：関数や段落のような2つのかたまりをくっつけると、離したかったことが多いので、`newText` の頭（末尾）に空行を、と知らせる。srwr が空行を自分で足さないのは、かたまりが何かは言語で違うため。`edits` では後者は項目の結果に付く。テープには書かない |
| 出力の `above`・`below` | 新しい範囲の直前・直後の、**今のファイルの**行（それぞれ最大2行。ファイルの先頭・末尾に近いと少なく、なければ `[]`）。置き換える前の内容ではない（置き換えた行は返さない。AI が `expect` で渡したもの）。ファイルを読み直さずに、結果を確かめられる |

**`newText` の行の数え方**：`""` は0行（削除）。それ以外は `\n` で行に分け、末尾が `\n` なら最後の空要素は数えない（`"x\n"` は1行、`"\n"` は空行1つ）。返すトークンの範囲は、この数え方による。

**行番号は AI が計算しなくてよい。** 別の場所の編集で行がずれても、srwr が補正する。範囲と重なる編集があったときだけ、`selection_stale` になる。

### トークンなし（`file` と `expect`）

`expect` は必須。範囲を行番号だけで決めないための確かめだから（`expect` が要らないのは、下の挿入だけ）。srwr は次の順で範囲を決める。

1. 渡された行番号の範囲が、`expect` の行を持っていれば、それ
2. その行番号を、**ファイルの最後の look**（`look` か、hook の読み取り）のあとにそのファイルへ行われた `edit`・`new` で、今の行へずらした範囲が、`expect` の行を持っていれば、それ
3. どちらでもなければ、ファイルの中で `expect` の行がある、ただ1か所

0か所なら `content_not_found`（行番号があれば、`actual` にその行を付ける）。2か所以上、または 1 と 2 が別の場所を指すときは `content_ambiguous`（場所を知らせる。行番号か、`expect` の行を増やす）。だから、同じファイルへの edit を、順不同で、並列でもまとめて送れる。先の編集で行がずれても、それぞれが見つかる。

**挿入**（`endLine = startLine - 1`、`expect` なし）は、確かめる行がない。ファイルが最後の look のあと変わっていない（`edit`・`new`・`external` が後にない）ときだけ受け付け、行番号をそのまま使う。そうでなければ `content_not_found`。`look` し直すか、置きたい場所の隣の行を `expect` と `insert` で指す。

**`insert`**（`"after"` か `"before"`）は、最後の look のあとファイルが変わっていても、行の隣に挿入できる。いつもと同じに行を指す（トークン、または `file` と `expect`。`expect` は確かめる）と、その行は残り、`newText` がその直後（直前）に入る。返事（`selection`・`startLine`・`endLine`・`lines`・`above`・`below`）は足した行のことで、テープには、ほかの挿入と同じく空範囲の `edit` が入る。`insert` が `invalid_input` になるのは、範囲が空のとき（指す行がない）と、`"after"`・`"before"` 以外のとき。`insert` つきで `newText` が空なら、**空行を1つ**足す（`insert` がなければ削除）。

**`insert: "start"`・`"end"`** は、`file` と `newText` だけで（`selection`・`expect`・行番号は一緒に渡さない。`invalid_input`）、ファイルの先頭か末尾に `newText` を足す。look は要らず、ファイルが変わっていてもよい（場所が動かないため）。テープには、ほかの挿入と同じく空範囲（`1..0`、`n+1..n`）の `edit` が入る。`edits` の項目にもでき、同じ場所への2つは重なりになる。

テープには、トークンの場合と同じ `edit` を書く（`from` は `null`）。`expect` はテープに書かない。

### 行の一部を直す（`old` と `new`）

`expect` は行全体で、編集の安全はそこから来る。長い行の数語だけを直すときは、`expect` と `newText` の代わりに、`file`、**`old`**（置き換える文字列）、**`new`**（その代わりの文字列）を渡す。`old` はただの文字列で、複数行でもよく、正規表現ではない。`startLine`・`endLine` を渡せば、その行の中で探す。

```jsonc
// 入力
{ "file": "cmd/main.go", "old": "cleanup()", "new": "teardown()", "why": "cleanup の名前を変えたため" }
```

- `old` は、ファイルの中で**1か所**でなければならない（`aaa` の中の `aa` のように重なる場所は2か所）。行番号があれば、その行、最後の look のあとの変更でずらしたその行、ファイル全体の順に探し、見つかった最初のもので決める。0か所は `content_not_found`（空白やタブだけ違う場所は `nearMatches`）、2か所以上は `content_ambiguous`（行を言う）。`startLine`・`endLine` を渡すか、`old` を長くする
- 結果はほかの編集と同じ。範囲は `old` にかかる行を丸ごと取ったもので、その行は `old` を `new` に替えたものになる。`old` が改行で終わり `new` がそうでないときは、次の行が後ろにつながる。テープには `expect` のときと同じ `edit`（`from` は `null`）で、`oldText`・`newText` は行全体。`old`・`new` そのものはテープに書かない
- `old` のときは、**行番号が片方だけでもよい**：`startLine` だけならその行から末尾まで、`endLine` だけなら先頭からその行まで探す（`expect` のときは今までどおり両方が要る）。最後の look からずらした行は、両方あるときだけ試す
- `old` は空でなく、`old` と `new` は同じでなく、どちらにも CR を含まず、`selection`・`expect`・`newText`・`insert` とは一緒に渡さない（`invalid_input`）。`edits` の項目でも使える

扱えないファイル：LF 以外の改行（CRLF）を含むファイルと、バイナリ。`unsupported_file` になる。

### 1回で複数の編集をする（`edits`）

`edits` は、1つの `why` を共有する編集を1〜50件持つ。`selection`・`file`・`startLine`・`endLine`・`expect`・`newText`・`insert`・`old`・`new` は、`edits` の外には**渡さず**、項目ごとに持つ：`selection`、または `file` と `expect`（分かれば `startLine`・`endLine`）、または `file`・`old`・`new`、`newText`、必要なら `insert`。

```jsonc
// 入力
{ "edits": [
    { "file": "cmd/main.go", "expect": "\tsetup()", "newText": "\tstart()" },
    { "file": "cmd/main.go", "expect": "\trun()", "insert": "after", "newText": "\tlog()" } ],
  "why": "呼び出しの名前を替え、実行をログに残す" }
// 出力
{ "ok": true, "edits": [ { "selection": "sel_…", "startLine": 4, "endLine": 4, "lines": ["…"], "above": ["…"], "below": ["…"] }, … ] }
```

- **項目に `why` はない。** 項目が `why` を渡しても呼び出しは通り、結果の `note` が、どの項目の `why` を使わなかったかを言う（呼び出しの `why` が全部の編集に付く）
- **全部行うか、何も行わないか。** 先に範囲を全部決め、次にファイルを全部書き、最後にテープに書く。1つでも失敗したら何も変えず、エラーはその項目のもので、`message` の頭に `edits[2]: `（3番目の項目）が付き、末尾は「何も変えていない。その項目を直して、全部の項目を送り直す」（英語）。呼び出し全体の誤り（項目がない・50件を超える・`why` が空白・範囲が重なる）は `invalid_input`
- **範囲は全部、呼ぶ前のファイルで決める。** だから項目同士は関係せず、順番は自由で、項目の `startLine`・`endLine` は、ほかの項目が変えたあとでなく、AI が見た行番号でよい
- **`content` は `edit` の入力ではない。** `content` のある項目は断られる（`invalid_input`。新しいファイルは `new` で作る）
- **範囲は重なってはいけない。** 行を共有する2つの範囲、別の範囲（`a..b`）の中への挿入（`a+1` 行目から `b` 行目の前）、同じ場所への2つの挿入は `invalid_input`（`edits[0] and edits[1] overlap in a.go`）。別の範囲のすぐ前・すぐ後ろへの挿入はよい
- 出力の `edits` は、項目ごとに1つで、**渡した順**。1つ1つは単独の `edit` の出力と同じ形だが、`lines` などは**呼び出し全体のあと**のファイルの行で、`selection` はそのまま使える
- テープには、項目ごとに `edit` が1つ、同じ `why` で入る。行番号は、上の項目が変えたあとのもの（別々に呼んだときと同じ中身のイベントを、ファイルの上から順に書く。[tape.md](tape.md#edit)）。失敗した呼び出しは、失敗した項目についての `failure` が1つ。ビューアーでは、単独の呼び出しと同じに見える

## new

まだないファイルを、内容つきで作る。理由も記録する。

```jsonc
// 入力
{ "file": "internal/config/config.go", "content": "package config\n\nfunc Read() {}", "why": "config パッケージを作り始める" }
// 出力
{ "ok": true, "selection": "sel_041G417ZRKYSPWJ7X4Z5PPKP", "startLine": 1, "endLine": 3 }
```

| 項目 | 意味 |
|---|---|
| `file` | 作業場からの相対パス。ファイルがまだないこと |
| `content` | ファイルの内容。改行は LF（CR は `invalid_input`）。`""` なら空のファイル |
| `why` | なぜそのファイルを作るか |
| 出力の `selection` | 内容全体のトークン（`edit` に使える）。`startLine`・`endLine` はその行（空のファイルは `endLine` が 0）。内容は返さない（AI が今書いたもの） |

- ファイルが既にあれば `file_exists` で、何も変えない。変えるときは `look` と `edit` を使う
- ファイルの上のディレクトリがなければ作る。作業場の外や、記録しない場所を指すリンクが途中にあれば断る（`invalid_range`・`ignored_file`）。何も作らない
- ファイルは、`content` が改行で終わっていてもいなくても、必ず改行で終わる（`"a"` と `"a\n"` は同じファイルになる）
- 記録しないファイル（`.env` など）は作れない（`ignored_file`）
- テープでは `new` 1つ（[tape.md](tape_ja.md#new)）。ビューワーは、ファイル全体を `edit` と同じように橙で塗り、上に `why` を出す
- シェルのコマンドで作ったファイルも記録される。そのときは `created: true` の [`external`](tape_ja.md#external) で、`why` はない

## why

- `look`・`edit`・`new` の**3つとも必須**。空白だけも不可（`invalid_input`）
- `look` は「なぜここを見るか」、`edit` は「なぜこう変えるか」。「何をしているか」の言い換えではなく、理由を1文で書く
- **ユーザーとの会話と同じ言語**で書く（ツールの説明文と入力スキーマで求めている）。人が読むためのもの

## エラー

MCP の応答では `isError: true` になり、本文は次の JSON。

```json
{"ok": false, "error": {"code": "selection_stale", "message": "…", "actual": ["…"]}}
```

| コード | 意味 | AI の取るべき行動 |
|---|---|---|
| `invalid_selection` | トークンの形式が不正、または書き換えられた。別のテープ（別のセッション）で発行されたものも、これになる。30分空いて新しいセッションになったときも同じ | `look` し直す |
| `selection_stale` | トークンを発行したあとに、その範囲と重なる編集があった | `look` し直す |
| `selection_mismatch` | 行番号を補正しても、範囲の内容が `look` したときと違う（srwr の外で変更された疑い） | 内容を確認して `look` し直す |
| `file_not_found` | 対象のファイルがない、または通常のファイルでない（ディレクトリなど） | — |
| `invalid_range` | 行番号がファイルの範囲外、または作業場の外のパス | 行数を確認して `look` し直す |
| `content_mismatch` | 範囲の内容が `expect` と違う | メッセージが言う場所を読んで、`look` し直す（または `nearMatches` から写す） |
| `content_not_found` | `expect` がファイルにない | 内容を確認する。または行番号を付ける。`nearMatches` があれば、そこから `expect` を写す |
| `content_ambiguous` | `expect`（行番号なし）がファイルの2か所以上にある | 行番号を付ける。または `expect` の行を増やす |
| `file_exists` | `new` で、既にあるファイルを作ろうとした | 変えるなら `look` と `edit` を使う |
| `ignored_file` | 記録しないファイルに `look`・`edit`・`new` した。 | srwr では扱えない。ユーザーに頼む |
| `invalid_input` | 必須の入力がない、型が違う（`message` が項目と、期待する形を言う：`edits must be an array of objects, got string. Pass it as JSON, not as a string that holds JSON`）、`why` が空。`file` が空か NUL を含む、`startLine` と `endLine` の片方だけがある、または両方なく `expect` もない、`selection` が空白だけ、`newText` に CR がある。`look` では、`search` が空・改行を含む・`startLine`・`endLine`・`expect` と一緒、`search` なしの `include`・`exclude`・`offset`、負の `offset`、`looks` が0件か10件を超える・合計2000行を超える・`file`・`startLine`・`endLine`・`expect`・`search` と一緒。`edit` の `edits` では、項目が0件か50件を超える、範囲が重なる、項目や呼び出しに上の誤りがある、項目に `content` がある。`new` では、`content` がない・CR がある、ファイルの上のディレクトリがファイルになっている | 入力を直す |
| `unsupported_file` | CRLF やバイナリ | — |
| `internal_error` | I/O エラーなど | — |

エラーには、現在の内容（`actual`）が付くことがある。中身はコードごとに決まっている。

| コード | `actual` |
|---|---|
| `selection_mismatch` | 補正後の範囲の現在の内容（行の配列） |
| `content_mismatch` | 範囲の現在の内容（行の配列） |
| `selection_stale` | 重なった編集の直前まで補正した範囲の、現在の内容（ファイルの範囲内に収める） |
| `invalid_range` | `{"lineCount": 行数}`。作業場の外のパスのときはなし |
| ほか | なし |

### `nearMatches`

文字列が見つからないとき、いちばんありそうな原因は、空白（スペース・タブ）の取り違えである。そこで、`expect`（`look`・`edit`）が、空白を除けば見つかるとき、エラーに `nearMatches` が付く（`actual` の中でなく隣。`actual` の形は変えない）：`[{"file", "startLine", "endLine", "lines"}]`。`lines` はファイルの今の行のままで、`file` はそのファイル。比べるときは、各行の空白（スペース・タブ）の連続を1つのスペースにし、行末の空白を削る。文字列がそのまま見つかる場所は入れない。5件まで。なければ `nearMatches` 自体を出さない。空白だけ違う場所がなくても、`expect` が行の一部として見つかる（行 `x := foo(1)` に対する `foo(`）ときは、その行を入れ、`message` は「`expect` は行全体で書く」と言う（`Line 12 holds expect only as part of the line: expect must be whole lines. Copy them from nearMatches`）。これは `look`・`edit` だけ。`message` は場所だけを言い（`Line 12 differs from expect only in spaces or tabs: see nearMatches`）、ファイルの中身は入れない。**`nearMatches` はテープに書かない**（行はファイルの中身のため）。

### `retry`

AI が指したはずの場所がただ1つに決まるとき、`look`・`edit` のエラーに `retry` が付く。もう一度呼ぶ引数（`why` を除く）で、行は今のファイルのとおり。付くのは、`expect` が見つからず、空白やタブだけ違う場所がちょうど1つのとき（なければ、行の一部として含む場所がちょうど1つのとき）と、`look` で `expect` が、指した行とは別の行にただ1か所あるとき。中身は、その場所の `{"file", "startLine", "endLine", "expect"}`。`edit` では、渡された `newText`（と `insert`）がそのまま付く。何も適用しない：AI が読んで、呼び直す。場所がないとき、2か所以上のときは `retry` を付けない。`edits` の項目では、1件の `edit` の呼び出し。**`retry` はテープに書かない**（行はファイルの中身だから）。

失敗した呼び出しは、AI のミスをあとで読めるように、[`failure`](tape_ja.md#failure) としてテープにも書く。絶対パスや作業場の外のパスは、実際のパスを伏せる。

## 処理の順序

`edit` を受け取ると、srwr は次の順で処理する。

1. トークンの復号と検証（失敗は `invalid_selection`）
2. ファイルの特定
3. srwr の外で起きたファイルの変更を検知する（あれば [`external`](tape_ja.md#external) として記録）
4. 行番号の補正（重なる編集があれば `selection_stale`）
5. 内容の照合（一致しなければ `selection_mismatch`）
6. **先に実ファイルを書き、そのあとテープに追記する**

検知するファイルをトークンから知るので、復号が先になる。偽のトークンでは、外部変更を記録しない。`look` は、パス・記録しないファイル・ファイルの種類の検査（`invalid_range`・`ignored_file`・`file_not_found`・`unsupported_file`）→ 検知 → 範囲の検査（`invalid_range`）→ 内容の確認（`content_mismatch`・`content_not_found`・`content_ambiguous`）→ 記録の順。

途中で落ちても、次に srwr が触れたとき、実ファイルとの食い違いが `external` として見える。

## AI が犯しやすいミス

AI がこれらのツールを使って実際に犯したミス。どれも、コードのあるふつうのエラーで、失敗した呼び出しは [`failure`](tape_ja.md#failure) としてテープに書かれる。

- **`file` は、作業場からの相対パスで渡す。** `/home/me/app/main.go` のような絶対パスは `invalid_range`、`../main.go` も `invalid_range` になる。`cmd/app/main.go` の形で書く。
- **空範囲は、1 行ずれやすい。** `endLine = startLine - 1` は「`startLine` 行目の直前」を指すので、`startLine: 13, endLine: 12` は 12 行目と 13 行目の間になる。先にその場所の前後の行を読み、番号を確かめてから `look` を呼ぶ。
- **新しい `look` の行番号は、今のファイルの行番号で渡す。** ほかの編集で行がずれたとき、すでに発行したトークンは srwr が補正するが、新しい `look` の `startLine`・`endLine` は補正しない。ほかの編集のあとは、ファイルを読み直すか、返ってきた `lines`（`edit` のあとは `above`・`below` も）で確かめる。もっと良いのは、`expect` に狙った行を渡すこと。番号が違えば、違う場所を選ばずに断られる。
- **新しいファイルは `new` で作る。** 存在しないファイルへの `look` は `file_not_found` になる。シェルのコマンドで作ったファイルも記録されるが、`created: true` の [`external`](tape_ja.md#external) で、`why` はない。
- **同じ場所を続けて直すときは、`edit` が返した新しいトークンを使う。** 使ったトークンは使い切りで、もう一度使うと `selection_stale` になる。
- **同じ変更を何か所もするときは、`look` と `edit` を何度も使わず、`edits`（場所ごとに `old` と `new`）を使う。**
- **エラーを読む。** `invalid_range` は `lineCount`（ファイルの行数）を返すので、番号を直すにはそれで足りる。

## 関連

- 記録されるもの：[tape.md](tape_ja.md)
- コマンドラインと導入：[cli.md](cli_ja.md)
