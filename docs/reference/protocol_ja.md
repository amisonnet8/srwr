# 表示サーバーのプロトコル

*[English](protocol.md) | **日本語***

**読者**：srwr の表示を、新しいエディタ（IDE）に対応させたい人。VSCode と Vim は、この約束だけでテープを再生している。

**テープを読んで、コマ送りに必要なデータを組み立てるのは、表示サーバー（`srwr view-server`）の仕事。** エディタ側は、サーバーから受け取ったデータを描くだけでよい。**テープの形式を知らなくてよく、テープも実ファイルも読み書きしない。** 鍵（`.srwr/key`）も読まない。

| 表示サーバー（Go） | クライアント（エディタ） |
|---|---|
| テープの一覧と読み込み | サーバーの起動と終了 |
| **コマの列の組み立て** | **描画**（範囲の色、`why` の行、diff 画面、サイドバー、ステータス） |
| 各コマの時点の**文書の内容** | `why` の行を文書に差し込む |
| 差分のコマの変更前・変更後、最後の差分（今のファイルとの比較） | コマ送り、キー操作 |
| 設定（差分のコマを出すか）を受ける | 設定（差分のコマ）をサーバーに渡す（VSCode・Vim は、固定の値を送る） |
| ライブ：今のテープを見張り、追記されたコマを通知する | ライブ：通知を受けて描く |

## 流れ

クライアントが何をするか。

1. `srwr view-server --root <作業場>` を子プロセスとして起動する
2. `initialize` を送る（これより前の要求は `not_initialized`）
3. **リプレイ**：`tapes/list` で一覧を出す → 選ばれたら `tape/open`（コマの列が返る）→ コマを移るたびに `frame/state`（そのコマの文書の内容）→ 閉じるとき `tape/close`
4. **ライブ**：`live/start`（今のテープのここまでのコマが返り、見張りが始まる）→ 追記のたびにサーバーから `live/frame` が届く → やめるとき `live/stop`
5. 終わるとき `shutdown` を送る

1つのクライアントにつき、1つのサーバーのプロセスを起動する（共有のデーモンにはしない。共有の媒体はテープそのもの）。

## 通信

- **stdio で JSON-RPC 2.0**。**1メッセージ＝1行**（改行区切りの JSON）。LSP のようなヘッダーはない
- 標準エラー出力はログ。クライアントは表示しなくてよい
- 改行区切りにしたのは、Vim の標準機能（`job` と `nl` モードのチャンネル）でも、VSCode の子プロセスでも、ライブラリなしで扱えるため

## メソッド

引数は JSON のオブジェクト（なければ `{}`）。

| メソッド | 種類 | 引数 → 結果 |
|---|---|---|
| `initialize` | 要求 | `{client:"vscode"\|"vim", protocolVersion:3, options:{diffFrames}}` → `{serverVersion, protocolVersion:3}`。`options` の既定は `diffFrames` が `true`。知らない `options` は無視する |
| `tapes/list` | 要求 | `{}` → `{tapes:[TapeInfo…]}`。操作を1つ以上持つテープだけを、新しい順に（テープを始めた時刻の順。headerのないテープは最後の更新の時刻）。読めないテープは載せない |
| `tape/open` | 要求 | `{tapeId, withText?:false, kinds?}` → `{tapeId, frames:[Frame…], hidden?}`。コマの列。`diffFrames` が真なら、作業場の今のファイルと比べた**最後の差分**（`final`）を末尾に含む。同じ `tapeId` をもう一度開くと、読み直す |
| `frame/state` | 要求 | `{tapeId, index, file?}` → `{before, after, content}`。`index` のコマの変更前・変更後（`index` が −1 のときは両方 `""`）。`content` は `file`（省略時はそのコマのファイル）の、そのコマを終えた時点の内容。どのコマも触れていないファイルは `null` |
| `tape/close` | 要求 | `{tapeId}` → `{}` |
| `live/start` | 要求 | `{withText?:false, kinds?}` → `{tapeId:string\|null, frames:[Frame…], hidden?}`。今のテープ（更新時刻が最新のもの）の、**ここまでのコマ**（クライアントは表示せず、一覧に載せるだけ）。返事のあと、見張りが始まる |
| `live/hidden` | 通知（サーバー→クライアント） | `{tapeId, hidden}`。クライアントが頼んでいない種類のコマが届いたので、隠したコマの数が変わった |
| `live/frame` | 通知（サーバー→クライアント） | `{tapeId, frame:Frame}`。追記されたコマ。`tapeId` が今までと違う（別のテープに移った）ときは、クライアントは列を作り直す。そのテープのコマは先頭から送る |
| `live/stop` | 要求 | `{}` → `{}`。見張りをやめる |
| `shutdown` | 要求 | `{}` → `{}`。返事を書いたあと、サーバーは終了する |

**`tapeId`**：テープのファイル名から `.tape.jsonl` を除いたもの（例：`20260929-0237-1359`）。使える文字は `0-9 A-Z a-z - _ .` だけで、`.` で始まってはいけない。それ以外（`/` など）は `invalid_params`。

**TapeInfo**：`{tapeId, startedAt, updatedAt, ops, files}`。`startedAt` は header の値（header がなければ `""`）、`updatedAt` はテープのファイルの最終更新時刻（UTC の RFC 3339、ミリ秒まで、末尾は `Z`）、`ops` は `look`・`edit`・`external` の数、`files` は触れたファイル（初めて触れた順）。古い版が書いたテープの `startedAt` には `+09:00` のようなオフセットが付いていることがある（同じ瞬間）。クライアントは、これらの時刻をその機械の時間帯で見せる。

## コマ（Frame）

**契約は「コマの列」と「各コマの時点の文書の内容」。** クライアントは、これだけを見て描く。

| 種類（`kind`） | 元になるもの | 持つ情報（要点） |
|---|---|---|
| `look` | テープの `look`（`source` が `mcp` でも `hook` でも） | ファイル、範囲、`why`（`null` のことがある）、`seq`、系譜（`selection`） |
| `edit` | テープの `edit` | ファイル、変更前後の範囲とテキスト、`why`（`null` のことがある）、`seq`、系譜（`from`→`selection`） |
| `replace` | テープの `replace`：1ファイルの、全部の場所 | ファイル、範囲と変更前後のテキスト、`why`、`seq`、`hits`（場所の数）、`hunks`。`external` と同じ差分で見せ、`why` は変わった行のまとまりごとの直前の帯に出す |
| `new` | テープの `new`：新しいファイル全体 | ファイル、範囲（ファイル全体）と変更後のテキスト（変更前は空）、`why`、`seq`。`edit` と同じ1枚のエディタで、ファイルを橙で塗り、上に `why` を出す |
| `external` | テープの `external` | ファイル、変更前（直前の内容）と変更後（`text`）、削除されたか |
| `final` | テープの最後の内容と、今のファイルの比較 | ファイル、変更前（テープの最後）と変更後（今のファイル）、今は存在しないか |
| `failure` | テープの `failure`（AI がエラーを受け取った `look`・`edit`・`replace`・`new`） | `tool`・`code`・`message`・`why`。`file` は、分からないときと伏せるときは `""`。`range` は `look` に渡された範囲（ないときは `{start:0,end:-1}`）。変更前後の本文は空 |

Frame のフィールド：

| フィールド | 内容 |
|---|---|
| `index` | 0 から始まる、列の中の位置 |
| `kind` | `look`・`edit`・`replace`・`new`・`external`・`final`・`failure` |
| `seq`・`ts` | テープの `seq`、時刻（エポックミリ秒。読めなければ前のコマの値、最初は 0）。`final` は最後のコマの値 |
| `file` | 作業場からの相対パス（`/` 区切り） |
| `range` | `{start, end}`。変更後の側の範囲（`look`＝その範囲、`edit`・`replace`・`new`＝新しい範囲、`external`・`final`＝ファイル全体）。`end < start` は空範囲 |
| `oldRange` | `edit` だけ。変更前の側の範囲 |
| `why`・`selection`・`from` | 文字列または `null` |
| `parent` | 系譜の親（`from` が指すコマの `index`）、なければ `null` |
| `tool`・`code`・`message` | `failure` だけ。ツール（`look`・`edit`・`replace`・`new`）、エラーコード、エラー文（実際のパスは伏せてある。[tape_ja.md](tape_ja.md#failure)） |
| `hits` | `replace` だけ。ファイルの中で変えた場所の数（ほかのコマでは出さない） |
| `hunks` | `replace` だけ。変わった行のまとまりを上から順に：`[{beforeStart, beforeEnd, afterStart, afterEnd}]`（1始まり、端を含む。変更前・変更後のテキストの行）。まとまりは、変わった行が続く塊なので、間に変わらない行がある場所は別のまとまり。挿入は `beforeEnd = beforeStart - 1`、削除は `afterEnd = afterStart - 1`。クライアントは、まとまりごとの直前に `why` を出す。比べきれないほど違うときは、変更の全体を1つのまとまりにする（ほかのコマでは出さない） |
| `deleted` | 差分のコマだけ。変更後にファイルが存在しない（そのときだけ `true`。それ以外は出さない） |
| `before`・`after` | **`withText` が真のときだけ**。変更前・変更後の全文。ふだんは `frame/state` で取る（大きいテープで、全コマが全文を持たないため） |

- **送る種類（`kinds`）**：`tape/open` と `live/start` は `kinds`（`look`・`edit`・`external`・`failure` の配列）を受ける。サーバーは、その種類だけを送り、**0 から番号を振り直す**（`index` は送ったものの中の位置で、`frame/state` もその `index` を受ける）。送らなかった数は `hidden` で返す（`{"failure": 2}` のような形。送らなかったものがない種類は入れない。何も送らなかったものがないときは、`hidden` 自体を出さない）。`final` は `external` に、`replace` と `new` は `edit` に連れる。省略すると `["look","edit","external"]`：頼まない限り `failure` のコマは送らない。知らない名前は `invalid_params`。空の配列は何も送らない。表示する種類を替えるときは、クライアントが別の `kinds` でもう一度開く。`frame/state` は、送らなかったコマに左右されない：ファイルの内容は、送らなかったコマも含めた結果になる
- ライブのコマ（`live/start`・`live/frame`）に、最後の差分は含まれない（`external` はテープに書かれたものが出る）
- **最後の差分は、サーバーが決める。** クライアントは出すだけ
- テープの項目が増えても（`source`・`tool`・`vcs` など）、クライアントは使わなくてよい
- **VSCode・Vim が使うフィールド**は、`index`・`kind`・`file`・`range`・`why`・`before`・`after`・`deleted`、`replace` では `hits` と `hunks`、`failure` では `tool`・`code`・`message`、それに `seq`（表示する種類を替えたとき、今に一番近いコマを探し直すため）だけ。`ts`・`selection`・`from`・`parent`・`oldRange` は使わない。ライブでも本文（`before`・`after`）を使うので、`live/start` に `withText: true` を渡す

## サーバーの振る舞い

- 最後の差分で今のファイルを読むとき、作業場の外を指すパス（`..`・絶対パス・外を指すシンボリックリンク）は読まず、「存在しない」として扱う。共有されたテープに、任意のファイルを読まされないため
- ライブの見張りは、テープの大きさのポーリング（間隔は200ミリ秒）。改行で終わっていない最後の行は、次に読むまで保留する
- テープの場所は `<root>/.srwr/tapes/`
- エラーの文言は英語で、開発者向け。人にエラーを見せるクライアントは、人が出会うもの（`tape_not_found`・`tape_unreadable`）を、コードから自分の言葉で言い直す
- サーバーはテープを**読むだけ**。ロックを取らず、書かない

## バージョンとエラー

- `protocolVersion` は整数。`initialize` で食い違ったらサーバーはエラーを返す。クライアントは「srwr とエディタ側のバージョンが合っていない」と分かる文言を出す
- エラーは JSON-RPC のエラーで返す。`error.data.code` に srwr のエラーコードが入る

| `data.code` | JSON-RPC の `code` | 意味 |
|---|---|---|
| `protocol_mismatch` | −32000 | `initialize` の `protocolVersion` が合わない |
| `not_initialized` | −32000 | `initialize` の前に要求が来た |
| `tape_not_found` | −32000 | `tapeId` のテープがない（開いていない・存在しない） |
| `tape_unreadable` | −32000 | テープを読めない |
| `invalid_params` | −32602 | 引数の誤り（型、`index` の範囲、不正な `tapeId`、`initialize` に `protocolVersion` がない） |
| （なし） | −32601 | 知らないメソッド |

## 互換性

- 足すのはよい。**古いクライアントが無視しても壊れない項目は、`protocolVersion` を上げずに足す。** 既存の項目の意味を変える・消すときは上げる
- クライアントは、知らないフィールドを無視する

## 対応するエディタを作るとき

- 描き方の基準は [vscode.md](vscode_ja.md)。コマの種類ごとの見せ方（範囲の色、`why` の行、差分のコマ）を、同じ情報・同じ順で出す
- 動くやり取りの例は [動く例](../examples/look-edit_ja.md) にある
