# 設定の一覧

*[English](settings.md) | **日本語***

**読者**：srwr を使う人。利用者が設定できるもの（と、設定できないもの）の一覧。

srwr の設定は少ない。**srwr に、見た目（色、幅、ラベルなど）の設定はない。**（Vim の色だけは、ハイライトグループで上書きできる。下を見る）設定するのは、主に `srwr` のバイナリの場所と、srwr が見せる言語。ほかに、記録しないファイル（`.srwrignore`）と、Edit・Write を禁止するか（厳格モードか緩いモードか。[cli.md](cli_ja.md)）がある。

## 1. 見る側の設定

### srwr のバイナリの場所

エディタは、`srwr` のバイナリを起動してテープを読む。`srwr` が PATH にあれば、設定はいらない。見つからないときは、場所を指定する。

| 設定 | 場所 | 既定 | 意味 |
|---|---|---|---|
| **`srwr.path`** | VSCode の設定（設定画面、または `settings.json`） | `srwr` | `srwr` のバイナリの場所。既定の `srwr` は、PATH から探す。見つからないときは、絶対パスを指定する |
| **`g:srwr_path`** | Vim の変数（`vimrc` に書く） | `srwr` | 同上。`let g:srwr_path = '/path/to/srwr'` |
| `SRWR_PATH` | 環境変数 | なし | VSCode の `srwr.path` が既定のままのときだけ使う、`srwr` の場所（拡張を開発用に起動するときの指定） |
| `SRWR_VIM` | 環境変数 | PATH の `vim` | `srwr view` が起動する Vim |

- VSCode の `srwr.path` は、**拡張が持つ唯一の設定**。見つからないときは、入れ方と「設定を開く」ボタンつきで案内する。バージョンが合わないときは、どちらかを更新するよう案内する
- `srwr view` で起動した Vim では、`g:srwr_path` は自動で、その `srwr` 自身に設定される
- あなたの `vimrc` は、`srwr view` でも読み込まれる。`g:srwr_…` の設定は `vimrc` に書ける

### 言語

srwr が出す文言は**英語が既定**で、日本語にも切り替えられる。

| どこ | 日本語にする方法 |
|---|---|
| `srwr` の端末の出力（使い方、`srwr init`、`srwr tapes`、`srwr trace`、`srwr view`、エラー） | 環境変数 `SRWR_LANG=ja` |
| Vim（`srwr view`、srwr-view.vim） | Vim を起動する前に、環境変数 `SRWR_LANG=ja` |
| VSCode（srwr-view） | srwr の設定はない。拡張は VSCode の表示言語に従う。日本語の言語パックを入れると、日本語で出る |

- `SRWR_LANG` は、値が `ja` で始まれば（`ja`、`ja_JP.UTF-8`）日本語、それ以外と未設定は英語。`LANG` などは見ない
- AI が読むもの（MCP のツールの説明とエラー文）、hook の注記と案内（AI のために標準出力へ書くもの）、hook 自身のフラグの使い方、表示サーバーのエラー文、古い Vim で Vim スクリプトが出す「動かせない」案内（`SRWR_LANG` を見る前に出す）は、英語だけ

### 時刻

- テープの時刻（`header.startedAt` と各イベントの `ts`）は **UTC** で書く（`2026-10-03T08:12:10.000Z`）。テープIDの日時も UTC。古い版が `+09:00` などのオフセットで書いたテープも、そのまま読める
- `srwr trace` は、その機械の時間帯の `HH:MM` だけを出す。`--json` は UTC の時刻（`2026-10-03T08:12:10.000Z`）
- 人に見せる時刻（`srwr tapes`、Vim の一覧、VSCode のテープ選び）は、**その機械の時間帯**で見せる。別の時間帯で見たいときは、環境変数 `TZ` を指定する（例：`TZ=America/Los_Angeles srwr tapes`）
- `srwr tapes` は、英語で `Oct 03 17:12`、日本語で `10/03 17:12`。Vim の一覧は `2026-10-03 17:12`、VSCode のテープ選びは `2026-10-03 17:12:10`（秒つき）で、どちらの言語でも同じ

### Vim の色の上書き

色は、Vim のハイライトグループで定義してあり、`vimrc` で上書きできる（`highlight default` で定義しているので、あなたの定義が優先される）。何も設定しなくても、背景が暗い・明るいに合わせて、決まった色になる。

| グループ | 使うところ |
|---|---|
| `SrwrWhySelect`・`SrwrWhyReplace` | look・edit（new）の理由の行 |
| `SrwrSelect`・`SrwrReplace` | look・edit（new）の範囲、差分の前（左）・後（右） |
| `SrwrCurrent` | 操作一覧の今のコマ |
| `SrwrDotSelect`・`SrwrDotReplace`・`SrwrDotExternal` | 操作一覧の丸（青・橙・紫） |
| `SrwrWhyFailure`・`SrwrDotFailure` | failure のコマの理由の行と丸（赤） |
| `SrwrDim` | 使えないボタン（戻る・進む） |

名前の `Select`・`Replace` は、色（青・橙）の古い名前。利用者の上書きが効き続けるよう、変えていない。値は [vim.md](vim_ja.md)。VSCode の色は、設定できない（テーマで決まる部分を除く。[vscode.md](vscode_ja.md)）。

## 2. コマンドの引数

| コマンド | 引数 | 意味 |
|---|---|---|
| `srwr mcp` | `--root <作業場>` | 作業場のディレクトリ。省略時はカレントディレクトリ |
| `srwr view-server` | `--root <作業場>` | 同上（エディタが起動する） |
| `srwr view [テープ]` | `--root <作業場>`、`--live` | 作業場（省略時はカレント）、ライブ視聴（テープとは一緒に使えない）。テープはテープIDか、`.tape.jsonl`・`.tape.jsonl.gz` のパス。省略すると一覧 |
| `srwr hook` | `--root <作業場>` | 標準入力から、Claude Code の hook の JSON を読む。作業場は、省略すると `CLAUDE_PROJECT_DIR`、それも無ければカレントディレクトリ |
| `srwr init` | `--lenient`、`--root <作業場>` | 作業場を準備する。`--lenient` は Edit・Write を禁止しない |
| `srwr tapes` | `new`・`prune --keep N`／`--older-than 30d`・`path <テープID>`・`check [<テープID>]`、`--root <作業場>` | テープの一覧・整理 |
| `srwr trace` | `--root <作業場>`、`--tape <テープID>`、`--mark`、`--json`、`--as-tape`、`[<ファイル>]` | 差分の追加行を作ったテープの操作を引く。差分はファイル、なければ標準入力 |
| `srwr` | `--version`（`version`）、`--help`（`-h`・`help`） | バージョン、使い方 |

## 3. 作業場のファイル

| ファイル | 内容 | 設定の仕方 |
|---|---|---|
| `.srwr/` | 鍵・ロック・今のセッション・テープ、`init-backup/`（`srwr init` が書き換える前の内容）。**鍵（`.srwr/key`）は共有しない** | srwr が作る |
| `.mcp.json` | AI エージェントに `srwr mcp` を登録する | `srwr init` が書く（手で書いてもよい） |
| `.claude/settings.json` | Claude Code の hook の登録、Edit・Write・MultiEdit・NotebookEdit の禁止（厳格モード） | `srwr init` が書く（手で書いてもよい） |
| `.srwrignore` | 記録しないファイルの指定（`.gitignore` と同じ書式） | 手で書く |
| `.gitignore` | `srwr init` が `.srwr/key`・`.srwr/lock`・`.srwr/active`・`.srwr/init-backup/` を足す（git の管理下のとき） | `srwr init` が書く |

`.mcp.json` の例（Claude Code）：

```json
{ "mcpServers": { "srwr": { "command": "srwr", "args": ["mcp"] } } }
```

`srwr init`（[cli.md](cli_ja.md)）が `.claude/settings.json` に書く内容（厳格モード）：hook の登録、Claude Code が聞かずに使えるようにする設定、組み込みの編集ツールの禁止（理由は [decisions.md](../design/decisions_ja.md)）。緩いモード（`--lenient`）では、`deny` を書かない。

```json
{
  "hooks": { "PostToolUse": [ { "matcher": "Read|Bash|Grep|Edit", "hooks": [ { "type": "command", "command": "srwr hook" } ] } ] },
  "enabledMcpjsonServers": ["srwr"],
  "permissions": {
    "allow": ["mcp__srwr__look", "mcp__srwr__edit", "mcp__srwr__new", "mcp__srwr__session"],
    "deny": ["Edit", "Write", "MultiEdit", "NotebookEdit"]
  }
}
```

## 4. 設定できないもの

次は固定で、設定できない。

| 項目 | 値 |
|---|---|
| セッションを区切る時間 | 最後のイベントから30分（AI の `session` ツールと `srwr tapes new` は、すぐに新しいセッションにする） |
| ライブの見張りの間隔 | 200ミリ秒 |
| 理由の折り返し幅 | VSCode は表示の幅100、Vim はウィンドウの幅 |
| 色・ラベル・差分の見せ方など、見た目 | 固定（Vim の色の上書きを除く） |
| 自動再生・速度 | なし（コマ送りだけ） |
| テープの保存期間 | 自動で消さない |
| hook の1回の呼び出しが記録するもの | `look` は100件まで、新しいファイルは1回のコマンドで50件まで・1件256 KiB まで |
| 既定で記録しないファイル | `.env`・鍵など（[cli.md](cli_ja.md)）。`.srwrignore` で足せるが、外せない |
| `srwr trace` が、それだけでは信じない短い行 | 12文字未満 |
