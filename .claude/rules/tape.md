# テープ・範囲トークン・表示サーバーのプロトコルの互換性（テープは v1 まで互換なし）

srwr には、形式の約束が2つある。

| 約束 | 書く側 | 読む側 | 正本 |
|---|---|---|---|
| **テープ** | `srwr mcp`・`srwr hook`（Go） | `srwr view-server`（Go） | `docs/reference/tape.md` |
| **表示サーバーのプロトコル** | `srwr view-server`（Go） | VSCode 拡張・Vim スクリプト | `docs/reference/protocol.md` |

テープは Go の中だけの約束。エディタとの境界はプロトコル。**エディタはテープを直接読まない。**

## 形式を変えるとき

- 形式の変更は重要な判断（`.claude/rules/working-with-human.md`）。推奨案つきで聞いてから
- **先に `docs/reference/` の該当文書を直し、読む側と書く側を同じ変更の中で直す。** プロトコルなら、サーバー・VSCode・Vim の3つを一緒に直す
- 実装側で勝手にフィールドを足したり、名前を変えたりしない。名前の付け方は `.claude/rules/naming.md`
- **v1 までは、テープの形式の互換を保証しない**（q 7e8928bce7。実験の段階で、利用者がいない）。形式を変えるとき、古い版・古いテープとの互換は考えなくてよい。`v` も上げなくてよい。ただし、形式の変更は重要な判断なので、聞いてから行う。プロトコル（エディタとの約束）の `protocolVersion` は、拡張と表示サーバーが食い違うと利用者が困るので、意味を変えたら上げる
- **v0.1.15：`srwr trace --as-tape` が、ほかのテープの操作を切り出した派生テープを書く。** header の `author.kind` が `derived`（形式の項目は増えない）。読み手は今までどおり読める。`srwr trace` はこれを元とは数えない。
- **v0.1.13：`replace` ツールをなくした。** 版 2 の `replace` のイベントも `edit` として読む（`tape.Parse`。`hits` は捨てる。`failure` の `tool: replace` も `edit`）。golden の `replace`・`sub` は版 1 の名前で、もともと `edit` と読み替えている。
- **版 2（v0.1.5）**：イベントの名前を、ツールの名前に揃えた（`select`→`look`、`replace`→`edit`、`tool` が `sub`／`new` の `replace`→`replace`／`new`）。**版 1 のテープも読める**（約束。`docs/reference/tape.md`）。読み替えは `tape.Parse`（`internal/tape/read.go`）の**1か所**で、行ごとの `v` を見る（更新のあとに同じテープへ版 2 で書き足されるため）。fixture・golden・固定テープは版 1 のまま。golden を比べるテストは、`select`→`look`・`replace`→`edit`・`sub`→`replace` と読み替えて比べる。書き換えない
- **v1 からは、足すのはよい、意味を変えるのはだめ。** 古い読み手が無視しても壊れない項目なら `v` を上げずに足せる。既存の項目の意味を変える・消すときは上げる。古い形式のテープを読めることを、fixture・golden のテストで確かめる
- 今の読み取りの古い形（`source` の無い `select`・`replace` は `mcp`、`text` の無い `external` は「内容不明」の目印）は、v1 までは整理して消してよい。fixture・golden は書き換えない（消すときは、そのテストの扱いを聞く）

## テープの書き方

- すべてのイベントに `"v":1`
- テープは追記のみ。既存の行を書き換えたり削除したりしない。**例外：閉じたセッションのテープは、中身を変えずに gzip で圧縮して `<ID>.tape.jsonl.gz` にする**（`tape.Compress`。次のセッションを始める側が、ロックの中で行う。失敗してもそのまま残し、作業を止めない）。**テープを開くのは `tape.Find`・`ReadAll`・`ReadFile`・`IDs` を通す**（生と `.gz` の両方。両方あるときは生が正）。`os.ReadFile(tape.FileName(id))` を直接呼ばない。展開後の大きさに上限がある（`MaxTapeSize`）
- `seq` はテープ内で 1 から始まる連番で、欠番を作らない（`mcp` と `hook` が同じテープに書くので、`seq` の採番はロックの中で、テープから読み足した最後の値の次にする）
- `ts`・`header.startedAt` は RFC 3339 の **UTC**（`tape.FormatTS`。末尾は `Z`）、ミリ秒まで。テープIDの日時も UTC。**古い版が書いた `+09:00` などのオフセット付きのテープも読める**（`time.Parse(time.RFC3339, …)` は両方読む）。古い行は書き換えない
- テープの一覧は、ファイル名の順でなく、**テープを始めた時刻の順**（古い名前は手元の時間帯、新しい名前は UTC のため）
- 値のないフィールド（`why`・`selection`・`from` など）は省略せず `null`
- 1イベント1行を、1回の write で追記する
- **先に実ファイルを書き、そのあとテープに追記する。** 途中で落ちても、次に触れたとき `external` として検知される
- 読む側（表示サーバー）は、改行で終わっていない最後の行を、書き込み途中として次に読むまで保留する

## 範囲トークン

- 形式は `docs/design/token.md`：`sel_` ＋ Crockford Base32（version・LEB128 の seq/startLine/endLine・fileHash 4B・textHash 4B・HMAC 3B）
- HMAC の入力に**テープID 全体**（例 `20261001-1706-1795`）を含める。トークンは発行したテープ（セッション）の中でだけ有効
- 復号は、大文字小文字を区別せず、`O`→`0`、`I`・`L`→`1`、ハイフン無視
- 鍵は `.srwr/key`（32バイト、0600）。テープの再生に鍵は不要。表示サーバーもエディタも鍵を読まない

## 正解データとの答え合わせ

| 正解 | 何の答えか | 使う段階 |
|---|---|---|
| `extension/test/fixtures/*.expected.json` | テープを読んだ結果（最後のファイルの内容と、コマの種類の並び） | R1 |
| `extension/test/golden/*.json`（22本） | コマの列（変更前・変更後つき）、`files`、`contentAt`、最後の差分。境界ケースは本文ごと埋め込み（`golden/README.md`） | R3・R4 |
| `extension/test/baseline/ja/`・`vim/test/baseline/hl_*.json` | 前の実装が見せた内容（VSCode の全コマ・ライブ、Vim の色の実測値）の写し | R4・R5（最初の基準） |

- **正解は書き換えない。** 食い違えば実装を直す。正解が誤りだと考えるときは、重要な判断として聞く（UI の見え方が変わるため）
- golden の `jumpLabels` は、今の UI では使わない（ジャンプラベルはやめた）。比べなくてよい
- 手書きのテープを正解や fixture にしない（実物と形が違う）。足りない場面は、本物の `srwr mcp` でテープを作る
