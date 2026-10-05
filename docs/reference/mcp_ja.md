# MCP ツール（look・edit・replace・new）

*[English](mcp.md) | **日本語***

**読者**：srwr を使う人。AI エージェントに何をさせ、どんなエラーが返るかを知りたい人。

AI エージェントは、MCP サーバー `srwr mcp` が提供する **4つのツール**だけでファイルを編集する。

| ツール | やること |
|---|---|
| `look` | 範囲を見る。範囲トークンが返る |
| `edit` | 範囲トークンの範囲を、新しいテキストに置き換える |
| `replace` | 複数のファイルで、文字列を別の文字列に一度に置き換える。場所の数が見込みどおりのときだけ |
| `new` | まだないファイルを、内容つきで作る |

`look` を経ない `edit` はない。そのため、テープには「見る → 変える」が必ず揃って残る。`replace` は、同じ変更を何か所もするときのもので、場所の数を渡させる。何を変えたかの確かめは、その数で行う。検索や読み取りは、AI がもともと持っている Read・grep に任せる（hook が記録する。[cli.md](cli_ja.md) の `srwr hook`）。

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
| `startLine`・`endLine` | 1始まり、両端を含む行番号。2つとも渡すか、2つとも渡さない（渡さないときは `expect` が範囲を見つける） |
| `expect` | 任意。範囲がこの内容であること。行を `\n` でつなぐ。下の「内容の確認」 |
| `why` | なぜここを見るか |
| `selection` | 範囲トークン。AI はそのまま `edit` に渡す（[範囲トークンとは](../design/token_ja.md)） |
| 出力の `startLine`・`endLine` | 選んだ範囲（`expect` で見つけたときは、その場所） |
| `lines` | 範囲の現在の内容。常に返る |

- **挿入位置**：`endLine = startLine - 1` の空範囲は「`startLine` 行目の直前」。たとえば `startLine: 13, endLine: 12` は12行目と13行目の間。ファイルの末尾への追記は `startLine = 行数 + 1`
- 範囲の条件：`1 ≤ startLine ≤ 行数 + 1`、`startLine - 1 ≤ endLine ≤ 行数`。外れると `invalid_range`
- 作業場の外を指すパス（`../`、絶対パス）も `invalid_range`

### 内容の確認（`expect`）

`expect` は範囲の内容を、行ごとに書いたもの。行を `\n` でつなぐ（数え方は `newText` と同じ。`""` は0行、最後の `\n` は数えない）。空白も照合する。範囲は行単位なので、行の一部は探さない。

| 呼び方 | 動き |
|---|---|
| `startLine`・`endLine` だけ | 今までどおり |
| `startLine`・`endLine` と `expect` | 範囲が `expect` の行とまったく同じときだけ通る。違えば `content_mismatch`。行番号がずれたことに気づける |
| `expect` だけ | `expect` の連続した行を、ファイルから探す。ちょうど1か所なら、そこが範囲。なければ `content_not_found`、2か所以上なら `content_ambiguous` |

- 行番号も `expect` もないとき、`startLine` と `endLine` の片方だけのときは、`invalid_input`。行番号なしで `expect` が `""` のときもそう（挿入位置は、行番号で指す）
- `content_mismatch` は、同じ行がファイルのどこにあるかを言う（最大5か所）。たいていは、それが直し方になる。`content_ambiguous` は、当たった場所を言う（最大10か所）。行番号を付けるか、`expect` の行を増やす
- `expect` はテープに書かない

## edit

範囲を新しいテキストに、1回の呼び出しで置き換える。挿入は空範囲への置き換え、削除は `newText` を空文字列にすることで表す。範囲の指し方は2つ：`look` の**範囲トークン**（ふつうの流れ：look して edit）か、トークンなしの **`file` と `expect`**。

```jsonc
// 入力
{ "selection": "sel_0410R3GZE4KV11C6325D32S7", "newText": "func main() {\n    setupSignals()\n    …\n}", "why": "シグナル処理の初期化が漏れていたので追加" }
// 出力
{ "ok": true, "selection": "sel_041GR3RZE4KV0BBH2S177Q36", "startLine": 12, "endLine": 15,
  "lines": ["func main() {", "    setupSignals()", "    …", "}"], "before": ["", "// main starts the app."], "after": ["", "func run() {"] }
```

| 項目 | 意味 |
|---|---|
| `selection` | `look`、`edit`、`new` が返したトークン。ファイルはここから決まる（`file` は不要）。これか、`file` と `expect` のどちらか（両方は不可） |
| `file`・`startLine`・`endLine`・`expect` | トークンなしのとき：ファイル、AI が見た範囲（行番号は両方か、どちらもなし）、その範囲が今持っている行（`\n` でつなぐ。`look` と同じ）。下を参照 |
| `newText` | 置き換え後のテキスト。`""` は削除 |
| `why` | なぜこう変えるか |
| 出力の `selection` | **置き換え後の範囲**の新しいトークン。同じ箇所を続けて直すときは、`look` し直さずにこれを使える |
| 出力の `lines` | 置き換え後の範囲の内容（削除なら `[]`） |
| 出力の `before`・`after` | その範囲の直前・直後の行（それぞれ最大2行。ファイルの先頭・末尾に近いと少なく、なければ `[]`）。ファイルを読み直さずに、結果を確かめられる |

**`newText` の行の数え方**：`""` は0行（削除）。それ以外は `\n` で行に分け、末尾が `\n` なら最後の空要素は数えない（`"x\n"` は1行、`"\n"` は空行1つ）。返すトークンの範囲は、この数え方による。

**行番号は AI が計算しなくてよい。** 別の場所の編集で行がずれても、srwr が補正する。範囲と重なる編集があったときだけ、`selection_stale` になる。

### トークンなし（`file` と `expect`）

`expect` は必須。範囲を行番号だけで決めないための確かめだから（`expect` が要らないのは、下の挿入だけ）。srwr は次の順で範囲を決める。

1. 渡された行番号の範囲が、`expect` の行を持っていれば、それ
2. その行番号を、**ファイルの最後の look**（`look` か、hook の読み取り）のあとにそのファイルへ行われた `edit`・`replace`・`new` で、今の行へずらした範囲が、`expect` の行を持っていれば、それ
3. どちらでもなければ、ファイルの中で `expect` の行がある、ただ1か所

0か所なら `content_not_found`（行番号があれば、`actual` にその行を付ける）。2か所以上、または 1 と 2 が別の場所を指すときは `content_ambiguous`（場所を知らせる。行番号か、`expect` の行を増やす）。だから、同じファイルへの edit を、順不同で、並列でもまとめて送れる。先の編集で行がずれても、それぞれが見つかる。

**挿入**（`endLine = startLine - 1`、`expect` なし）は、確かめる行がない。ファイルが最後の look のあと変わっていない（`edit`・`replace`・`new`・`external` が後にない）ときだけ受け付け、行番号をそのまま使う。そうでなければ `content_not_found`。`look` し直すか、置きたい場所の隣の行を `expect` にして edit し、`newText` に新しい行を含める。

テープには、トークンの場合と同じ `edit` を書く（`from` は `null`）。`expect` はテープに書かない。

扱えないファイル：LF 以外の改行（CRLF）を含むファイルと、バイナリ。`unsupported_file` になる。

## replace

**2か所以上**で、1つのファイルでも複数のファイルでも、文字列を別の文字列に一度に置き換える（簡単な sed のようなもの）。理由も記録する。1か所なら `edit` を使う（`replace` は断られる。`use_edit`、下）。文字列は、そのままの文字として探す（正規表現ではない）。各ファイルを左から探し、場所は重ならない。**全部のファイルでの場所の数 `count` は必須で、2以上。見つかった数が違えば、何も変えない。**

```jsonc
// 入力
{ "files": ["a.go", "b.go"], "old": "oldName(", "new": "newName(", "count": 3, "why": "新しい命名規則に合わせて、ヘルパーの名前を変える" }
// 出力
{ "ok": true, "count": 3, "files": [
  { "file": "a.go", "count": 2, "hits": [
      { "startLine": 12, "endLine": 12, "lines": ["…"], "before": ["…"], "after": ["…"] },
      { "startLine": 30, "endLine": 31, "lines": ["…", "…"], "before": [], "after": ["…"] } ] },
  { "file": "b.go", "count": 1, "hits": [ { "startLine": 7, "endLine": 7, "lines": ["…"], "before": ["…"], "after": ["…"] } ] } ] }
```

| 項目 | 意味 |
|---|---|
| `files` | 作業場からの相対パス。既存のファイルだけ。同じパスは1回だけ |
| `old` | 探す文字列。空は不可。複数行（LF）でもよい |
| `new` | 置き換える文字列。`""` は削除 |
| `count` | 全部のファイルを合わせた、場所の数の見込み。2以上。文字列が1か所だけにあるとき、`1` は `use_edit` で返される |
| `why` | なぜこう変えるか |
| 出力の `files` | 変わったファイルだけ。`count` はそのファイルの場所の数。`hits` は場所ごとに1件で、ファイルの今の状態の：`startLine`・`endLine`、`lines`（その行の内容）、`before`・`after`（直前・直後の1行。なければ `[]`）。同じ行にある場所は1件。1ファイルにつき20件まで載せ、`more` が載せなかった件数。**範囲トークンは返さない**：続けて直すときは `look` を使う |

- **1か所は `edit` で行う。** `count` が 1 で、文字列がちょうど1か所にあるとき、エラーは `use_edit` で、何も変えない。メッセージは場所（`a.go line 12`）だけを言い、ファイルの中身は入れない。`actual` に、`hits`（`file`・`startLine`・`endLine`・`lines`：その場所の今の位置）と、`edit`（`file`・`startLine`・`endLine`・`expect`・`newText`：同じ変更をする `edit` の呼び出し。足すのは `why` だけ）が付く
- 見つかった数が `count` と違えば（`count` が 1 で、0か所や2か所以上のときも）`count_mismatch`。メッセージと `actual` が、ファイルごとの場所の数を言う（`{"a.go": 3, "b.go": 1}`）。**どのファイルも変えず、テープには `failure` しか書かない**
- どのファイルも、`look` と同じ検査をする（記録しない・CRLF・バイナリ・作業場の外）。1つでも使えなければ、何も変えない
- 行で表せない変更（ファイルの最後の改行を足す・消す）は `invalid_input`。`look` と `edit` で行う
- テープには、変わったファイルごとに `replace` を1つ、同じ `why` で書く（[tape.md](tape_ja.md#replace)。トークンを返さないので `selection` は `null`）。ビューワーは、それぞれをファイルの差分として、上に `why` を付けて見せる
- 正規表現は使えない。場所を1つずつ選びたいときは `look` と `edit` を使う

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

- `look`・`edit`・`replace`・`new` の**4つとも必須**。空白だけも不可（`invalid_input`）
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
| `count_mismatch` | `replace` で見つかった場所の数が `count` と違う | ファイルごとの数を読み、正しい `count` で `replace` し直す（または `look` を使う）。`nearMatches` があれば、そこから `old` を写す |
| `use_edit` | 文字列が1か所だけにあるのに、`replace` を `count` が 1 で使った | `actual.edit` のとおりに `edit` を呼ぶ（`why` を足す） |
| `ignored_file` | 記録しないファイルに `look`・`edit`・`replace`・`new` した。 | srwr では扱えない。ユーザーに頼む |
| `invalid_input` | 必須の入力がない、型が違う、`why` が空。`file` が空か NUL を含む、`startLine` と `endLine` の片方だけがある、または両方なく `expect` もない、`selection` が空白だけ、`newText` に CR がある。`replace` では、`files`・`old`・`count` がない・空、同じパスが2回、`old` か `new` に CR がある。`new` では、`content` がない・CR がある、ファイルの上のディレクトリがファイルになっている | 入力を直す |
| `unsupported_file` | CRLF やバイナリ | — |
| `internal_error` | I/O エラーなど | — |

エラーには、現在の内容（`actual`）が付くことがある。中身はコードごとに決まっている。

| コード | `actual` |
|---|---|
| `selection_mismatch` | 補正後の範囲の現在の内容（行の配列） |
| `content_mismatch` | 範囲の現在の内容（行の配列） |
| `selection_stale` | 重なった編集の直前まで補正した範囲の、現在の内容（ファイルの範囲内に収める） |
| `count_mismatch` | ファイルごとの場所の数（`{"a.go": 3, "b.go": 1}`） |
| `use_edit` | `{"hits": [{file, startLine, endLine, lines}], "edit": {file, startLine, endLine, expect, newText}}`。行で表せない変更では `edit` を付けない |
| `invalid_range` | `{"lineCount": 行数}`。作業場の外のパスのときはなし |
| ほか | なし |

### `nearMatches`

文字列が見つからないとき、いちばんありそうな原因は、空白（スペース・タブ）の取り違えである。そこで、`expect`（`look`・`edit`）や `old`（`replace`。見つかった数が `count` より少ないときだけ）が、空白を除けば見つかるとき、エラーに `nearMatches` が付く（`actual` の中でなく隣。`actual` の形は変えない）：`[{"file", "startLine", "endLine", "lines"}]`。`lines` はファイルの今の行のままで、`file` は `replace` のときだけ。比べるときは、各行の空白（スペース・タブ）の連続を1つのスペースにし、行末の空白を削る。文字列がそのまま見つかる場所は入れない。5件まで。なければ `nearMatches` 自体を出さない。空白だけ違う場所がなくても、`expect` が行の一部として見つかる（行 `x := foo(1)` に対する `foo(`）ときは、その行を入れ、`message` は「`expect` は行全体で書く」と言う（`Line 12 holds expect only as part of the line: expect must be whole lines. Copy them from nearMatches`）。これは `look`・`edit` だけ。`message` は場所だけを言い（`Line 12 differs from expect only in spaces or tabs: see nearMatches`）、ファイルの中身は入れない。**`nearMatches` はテープに書かない**（行はファイルの中身のため）。

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
- **新しい `look` の行番号は、今のファイルの行番号で渡す。** ほかの編集で行がずれたとき、すでに発行したトークンは srwr が補正するが、新しい `look` の `startLine`・`endLine` は補正しない。ほかの編集のあとは、ファイルを読み直すか、返ってきた `lines`（`edit` のあとは `before`・`after` も）で確かめる。もっと良いのは、`expect` に狙った行を渡すこと。番号が違えば、違う場所を選ばずに断られる。
- **新しいファイルは `new` で作る。** 存在しないファイルへの `look` は `file_not_found` になる。シェルのコマンドで作ったファイルも記録されるが、`created: true` の [`external`](tape_ja.md#external) で、`why` はない。
- **同じ場所を続けて直すときは、`edit` が返した新しいトークンを使う。** 使ったトークンは使い切りで、もう一度使うと `selection_stale` になる。
- **同じ変更を何か所もするときは、`look` と `edit` を何度も使わず、`count` つきの `replace` を使う。** `count` が違えば、ファイルごとの数を言って断られる。
- **エラーを読む。** `invalid_range` は `lineCount`（ファイルの行数）を返すので、番号を直すにはそれで足りる。

## 関連

- 記録されるもの：[tape.md](tape_ja.md)
- コマンドラインと導入：[cli.md](cli_ja.md)
