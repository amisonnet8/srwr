# MCP ツール（select・replace）

*[English](mcp.md) | **日本語***

**読者**：srwr を使う人。AI エージェントに何をさせ、どんなエラーが返るかを知りたい人。

AI エージェントは、MCP サーバー `srwr mcp` が提供する **2つのツール**だけでファイルを編集する。

| ツール | やること |
|---|---|
| `select` | 見ている範囲を宣言する。範囲トークンが返る |
| `replace` | 範囲トークンの範囲を、新しいテキストに置き換える |

`select` を経ない `replace` はない。そのため、テープには「見る → 変える」が必ず揃って残る。検索や読み取りは、AI がもともと持っている Read・grep に任せる（hook が記録する。[cli.md](cli_ja.md) の `srwr hook`）。

どちらにも `why`（理由）が必須。人が [テープ](tape_ja.md) を再生するときの中心になる。

ツールの説明とエラーの文言は、AI が読むものなので、画面の言語に関わらず英語。

## select

見ている範囲を宣言し、その範囲を編集するための**範囲トークン**を返す。

```jsonc
// 入力
{ "file": "cmd/app/main.go", "startLine": 12, "endLine": 14, "why": "main関数に修正が必要か確認中" }
// 出力
{ "ok": true, "selection": "sel_0410R3GZE4KV11C6325D32S7", "lines": ["func main() {", "…", "}"] }
```

| 項目 | 意味 |
|---|---|
| `file` | 作業場からの相対パス。既存のファイルだけ（新しいファイルは作れない） |
| `startLine`・`endLine` | 1始まり、両端を含む行番号 |
| `why` | なぜここを見るか |
| `selection` | 範囲トークン。AI はそのまま `replace` に渡す（[範囲トークンとは](../design/token_ja.md)） |
| `lines` | 範囲の現在の内容。常に返る |

- **挿入位置**：`endLine = startLine - 1` の空範囲は「`startLine` 行目の直前」。たとえば `startLine: 13, endLine: 12` は12行目と13行目の間。ファイルの末尾への追記は `startLine = 行数 + 1`
- 範囲の条件：`1 ≤ startLine ≤ 行数 + 1`、`startLine - 1 ≤ endLine ≤ 行数`。外れると `invalid_range`
- 作業場の外を指すパス（`../`、絶対パス）も `invalid_range`

## replace

範囲を新しいテキストに置き換える。挿入は空範囲への置き換え、削除は `newText` を空文字列にすることで表す。

```jsonc
// 入力
{ "selection": "sel_0410R3GZE4KV11C6325D32S7", "newText": "func main() {\n    setupSignals()\n    …\n}", "why": "シグナル処理の初期化が漏れていたので追加" }
// 出力
{ "ok": true, "selection": "sel_041GR3RZE4KV0BBH2S177Q36", "startLine": 12, "endLine": 15 }
```

| 項目 | 意味 |
|---|---|
| `selection` | `select`、または直前の `replace` が返したトークン。ファイルはここから決まる（`file` は不要） |
| `newText` | 置き換え後のテキスト。`""` は削除 |
| `why` | なぜこう変えるか |
| 出力の `selection` | **置き換え後の範囲**の新しいトークン。同じ箇所を続けて直すときは、`select` し直さずにこれを使える |

**`newText` の行の数え方**：`""` は0行（削除）。それ以外は `\n` で行に分け、末尾が `\n` なら最後の空要素は数えない（`"x\n"` は1行、`"\n"` は空行1つ）。返すトークンの範囲は、この数え方による。

**行番号は AI が計算しなくてよい。** 別の場所の編集で行がずれても、srwr が補正する。範囲と重なる編集があったときだけ、`selection_stale` になる。

扱えないファイル：LF 以外の改行（CRLF）を含むファイルと、バイナリ。`unsupported_file` になる。

## why

- `select` と `replace` の**両方で必須**。空白だけも不可（`invalid_input`）
- `select` は「なぜここを見るか」、`replace` は「なぜこう変えるか」。「何をしているか」の言い換えではなく、理由を1文で書く
- **ユーザーとの会話と同じ言語**で書く（ツールの説明文と入力スキーマで求めている）。人が読むためのもの

## エラー

MCP の応答では `isError: true` になり、本文は次の JSON。

```json
{"ok": false, "error": {"code": "selection_stale", "message": "…", "actual": ["…"]}}
```

| コード | 意味 | AI の取るべき行動 |
|---|---|---|
| `invalid_selection` | トークンの形式が不正、または書き換えられた。別のテープ（別のセッション）で発行されたものも、これになる。30分空いて新しいセッションになったときも同じ | `select` し直す |
| `selection_stale` | トークンを発行したあとに、その範囲と重なる編集があった | `select` し直す |
| `selection_mismatch` | 行番号を補正しても、範囲の内容が `select` したときと違う（srwr の外で変更された疑い） | 内容を確認して `select` し直す |
| `file_not_found` | 対象のファイルがない、または通常のファイルでない（ディレクトリなど） | — |
| `invalid_range` | 行番号がファイルの範囲外、または作業場の外のパス | 行数を確認して `select` し直す |
| `ignored_file` | 記録しないファイルに `select`・`replace` した。 | srwr では扱えない。ユーザーに頼む |
| `invalid_input` | 必須の入力がない、型が違う、`why` が空。`file` が空か NUL を含む、`selection` が空白だけ、`newText` に CR がある | 入力を直す |
| `unsupported_file` | CRLF やバイナリ | — |
| `internal_error` | I/O エラーなど | — |

エラーには、現在の内容（`actual`）が付くことがある。中身はコードごとに決まっている。

| コード | `actual` |
|---|---|
| `selection_mismatch` | 補正後の範囲の現在の内容（行の配列） |
| `selection_stale` | 重なった編集の直前まで補正した範囲の、現在の内容（ファイルの範囲内に収める） |
| `invalid_range` | `{"lineCount": 行数}`。作業場の外のパスのときはなし |
| ほか | なし |

## 処理の順序

`replace` を受け取ると、srwr は次の順で処理する。

1. トークンの復号と検証（失敗は `invalid_selection`）
2. ファイルの特定
3. srwr の外で起きたファイルの変更を検知する（あれば [`external`](tape_ja.md#external) として記録）
4. 行番号の補正（重なる編集があれば `selection_stale`）
5. 内容の照合（一致しなければ `selection_mismatch`）
6. **先に実ファイルを書き、そのあとテープに追記する**

検知するファイルをトークンから知るので、復号が先になる。偽のトークンでは、外部変更を記録しない。`select` は、パス・記録しないファイル・ファイルの種類の検査（`invalid_range`・`ignored_file`・`file_not_found`・`unsupported_file`）→ 検知 → 範囲の検査（`invalid_range`）→ 記録の順。

途中で落ちても、次に srwr が触れたとき、実ファイルとの食い違いが `external` として見える。

## 関連

- 記録されるもの：[tape.md](tape_ja.md)
- コマンドラインと導入：[cli.md](cli_ja.md)
