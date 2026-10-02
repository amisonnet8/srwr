# 設定の一覧

**読者**：srwr を使う人。利用者が設定できるもの（と、設定できないもの）の一覧。

srwr の設定は少ない。**見た目（色、幅、ラベルなど）の設定はない。** 設定するのは、主に `srwr` のバイナリの場所。

## 1. 見る側の設定

### srwr のバイナリの場所

エディタは、`srwr` のバイナリを起動してテープを読む。`srwr` が PATH にあれば、設定はいらない。見つからないときは、場所を指定する。

| 設定 | 場所 | 既定 | 意味 |
|---|---|---|---|
| **`srwr.path`** | VSCode の設定（設定画面、または `settings.json`） | `srwr` | `srwr` のバイナリの場所。既定の `srwr` は、PATH から探す。見つからないときは、絶対パスを指定する |
| **`g:srwr_path`** | Vim の変数（`vimrc` に書く） | `srwr` | 同上。`let g:srwr_path = '/path/to/srwr'` |
| `SRWR_PATH` | 環境変数 | なし | VSCode の `srwr.path` が既定のままのときだけ使う、`srwr` の場所（拡張を開発用に起動するときの指定） |
| `SRWR_VIM` | 環境変数 | PATH の `vim` | `srwr view` が起動する Vim |

- VSCode の `srwr.path` は、**拡張が持つ唯一の設定**。見つからないとき・バージョンが合わないときは、入れ方と「設定を開く」ボタンつきで案内する
- `srwr view` で起動した Vim では、`g:srwr_path` は自動で、その `srwr` 自身に設定される
- あなたの `vimrc` は、`srwr view` でも読み込まれる。`g:srwr_…` の設定は `vimrc` に書ける

### Vim の色の上書き

色は、Vim のハイライトグループで定義してあり、`vimrc` で上書きできる（`highlight default` で定義しているので、あなたの定義が優先される）。何も設定しなくても、背景が暗い・明るいに合わせて、決まった色になる。

| グループ | 使うところ |
|---|---|
| `SrwrWhySelect`・`SrwrWhyReplace` | select・replace の理由の行 |
| `SrwrSelect`・`SrwrReplace` | select・replace の範囲、差分の前（左）・後（右） |
| `SrwrCurrent` | 操作一覧の今のコマ |
| `SrwrDotSelect`・`SrwrDotReplace`・`SrwrDotExternal` | 操作一覧の丸（青・橙・紫） |
| `SrwrDim` | 使えないボタン（戻る・進む） |

値は [vim.md](vim.md)。VSCode の色は、設定できない（テーマで決まる部分を除く。[vscode.md](vscode.md)）。

## 2. コマンドの引数

| コマンド | 引数 | 意味 |
|---|---|---|
| `srwr mcp` | `--root <作業場>` | 作業場のディレクトリ。省略時はカレントディレクトリ |
| `srwr view-server` | `--root <作業場>` | 同上（エディタが起動する） |
| `srwr view [テープ]` | `--root <作業場>`、`--live` | 作業場（省略時はカレント）、ライブ視聴。テープはテープIDか、`.tape.jsonl` のパス。省略すると一覧 |
| `srwr hook` | なし | 標準入力から、Claude Code の hook の JSON を読む |
| `srwr` | `--version`、`--help` | バージョン、使い方 |

## 3. 作業場のファイル

| ファイル | 内容 | 設定の仕方 |
|---|---|---|
| `.srwr/` | 鍵・ロック・今のセッション・テープ。**鍵（`.srwr/key`）は共有しない** | srwr が作る |
| `.mcp.json` | AI エージェントに `srwr mcp` を登録する | `srwr init` が書く（手で書いてもよい） |
| `.claude/settings.json` | Claude Code の hook の登録、Edit・Write・MultiEdit・NotebookEdit の禁止（厳格モード） | `srwr init` が書く（手で書いてもよい） |
| `.srwrignore` | 記録しないファイルの指定（`.gitignore` と同じ書式） | 手で書く |

`.mcp.json` の例（Claude Code）：

```json
{ "mcpServers": { "srwr": { "command": "srwr", "args": ["mcp"] } } }
```

厳格モードにするには、`.claude/settings.json` で、組み込みの編集ツールを禁止する（理由は [decisions.md](../design/decisions.md)）。

```json
{ "permissions": { "deny": ["Edit", "Write", "MultiEdit", "NotebookEdit"] } }
```

## 4. 設定できないもの

次は固定で、設定できない。

| 項目 | 値 |
|---|---|
| セッションを区切る時間 | 最後のイベントから30分 |
| ライブの見張りの間隔 | 200ミリ秒 |
| 理由の折り返し幅 | VSCode は表示の幅100、Vim はウィンドウの幅 |
| 色・ラベル・差分の見せ方など、見た目 | 固定（Vim の色の上書きを除く） |
| 自動再生・速度 | なし（コマ送りだけ） |
| テープの保存期間 | 自動で消さない |
