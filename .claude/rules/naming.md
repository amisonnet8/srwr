# 命名規則

## 製品名と部品名

| 対象 | 表記 | 備考 |
|---|---|---|
| 製品全体・Go のバイナリ | **srwr** | 常に小文字。文頭でも `srwr`。略語として意味を固定しない |
| サブコマンド | `srwr mcp`・`srwr hook`・`srwr view-server`・`srwr view`・`srwr init`・`srwr tapes`・`srwr trace` | `docs/reference/cli.md` |
| 表示サーバー | **表示サーバー**（view-server） | `srwr view-server`。エディタが起動する。人は直接使わない |
| VSCode 拡張 | **srwr-view** | 拡張ID・表示名とも `srwr-view` |
| Vim クライアント | **srwr-view.vim** | リポジトリの `vim/`。`srwr view` が埋め込みから起動する |
| MCP のツール名 | `look`・`edit`・`new`・`session` | 編集は前の3つだけ。`session` は題を付けて新しいテープを始める任意のツールで、ファイルに触れず、テープのイベントでなく header に書く。テープのイベント・表示サーバーのコマの種類・画面の名前は `look`・`edit`・`new`（`select`・`replace`・`sub` は古いテープの名前） |
| 拡張のコマンド・設定 | 接頭辞 `srwr.`（例：`srwr.openTape`、`srwr.path`） | コマンドパレットの表記は「srwr: 〜」 |
| Vim のコマンド・変数・ハイライト | コマンド `:Srwr…`（例：`:SrwrOpen`）、変数 `g:srwr_…`（snake_case）、ハイライト `Srwr…`（例：`SrwrWhyReplace`） | `docs/reference/vim.md` |

## 用語（日本語と英語の対応）

文書・コメント・UI の文言で、同じものを別の言葉で呼ばない。画面に出る英語は、この表の「英語」の列を使う（`external`・`final` など）。日本語の画面（`SRWR_LANG=ja`）は、「日本語」の列。

| 日本語 | 英語（コード・JSON） | 意味 |
|---|---|---|
| テープ | tape | 操作の記録。追記のみの JSONL。1セッション＝1本 |
| セッション | session | テープ1本分の作業のまとまり。`.srwr/active` が今のセッションを指す |
| 範囲トークン | selection token（フィールド名は `selection`・`from`） | `select` が返す `sel_…` の文字列 |
| コマ | frame | 再生の1ステップ。`look`・`edit`・`new`・`external`・`final` |
| コマの列 | frames / timeline | 表示サーバーが作り、エディタに渡すもの。UI との契約 |
| 文書の状態 | frame state | あるコマの時点のファイルの内容（差分のコマなら変更前・変更後） |
| 差分のコマ | diff frame | `external`・`final` のコマ。左右に並べた差分で見せる |
| 外部変更 | external change（イベントは `external`） | srwr の外で起きたファイルの変更 |
| 失敗 | failure（イベントは `failure`、コマも `failure`） | 失敗した `look`・`edit`・`new` の記録。コマにするのは、人が表示を ON にしたときだけ（赤） |
| 最後の差分 | final diff（コマは `final`） | テープの最後の内容と、今のファイルとの差分 |
| 理由 | why | `look`・`edit`・`new` に添える理由 |
| 作業場 | workspace | srwr を使うディレクトリ（`.srwr/` を持つ） |
| 厳格モード / 緩いモード | strict / lenient | Edit/Write を禁止するか（`docs/reference/cli.md`） |
| 記録しないファイル | ignored file（エラーは `ignored_file`） | `.srwrignore` と既定の対象 |
| ライブ / リプレイ（録画） | live / replay | 見るときの2つのモード |
| クライアント | client | 表示サーバーと話すエディタ側（VSCode 拡張・Vim スクリプト） |
| 固定テープ | ui-check | `extension/test/fixtures/ui-check/` の3本。確認の基準 |
| 正解のデータ | golden | `extension/test/golden/`。コマの列の答え |
| UIゲート | UI gate | UI に関わる段階の始めに、ポイント資料と画像で了承をもらうこと（`.claude/rules/working-with-human.md`） |

## JSON のフィールド名（テープ・MCP・表示サーバーのプロトコル）

- lowerCamelCase（例：`startLine`、`newText`、`fileShaBefore`）。`docs/reference/` に書かれた名前を変えない
- エラーコードは snake_case（例：`selection_stale`、`ignored_file`、`tape_not_found`）
- 表示サーバーのメソッド名は `名詞/動詞`（例：`tape/open`、`frame/state`、`live/frame`）
- 新しいフィールドやメソッドを足すときは、既存の名前の付け方に揃え、`docs/reference/` の該当文書に先に書く（`.claude/rules/tape.md`）

## Go のコード

- パッケージ名は短い小文字1語（`core`、`tape`、`token`、`tools`、`jsonrpc`、`mcp`、`hook`、`session`、`ignore`、`lang`、`setup`、`vcs`、`timeline`、`viewserver`、`cli`、`docs`）
- 上の用語表の英語をそのまま型名・関数名に使う（例：`Tape`、`Session`、`Selection`、`Frame`）
- サブコマンドの実装は `internal/cli` に置き、`cmd/srwr/main.go` は引数を渡すだけにする

## TypeScript のコード（拡張）

- ファイル名は小文字1語（`server.ts`、`present.ts`、`replay.ts`、`live.ts`、`controls.ts`、`sidebar.ts`、`extension.ts`）
- 用語表の英語を型名に使う（例：`Frame`）

## Vim script（`vim/`）

- Vim9 script。`autoload/srwr/<部品>.vim` に分け、ほかの部品からは `import autoload` で使う。公開する関数は `export def`、名前は大文字で始める
- コマンドやキーから呼ぶ関数も、`plugin/srwr.vim` から `import autoload` して呼ぶ
