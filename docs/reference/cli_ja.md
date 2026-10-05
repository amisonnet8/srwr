# コマンドライン（srwr）

*[English](cli.md) | **日本語***

**読者**：srwr を使う人。

`srwr` は Go の単一バイナリ。サブコマンドで役割が分かれる。

| コマンド | 使う人 | 役割 |
|---|---|---|
| `srwr mcp` | AI（MCP クライアント） | MCP サーバー（stdio）。[`look` / `edit` / `replace` / `new`](mcp_ja.md) を提供する |
| `srwr hook` | Claude Code の hook | Read・Bash・Grep・Edit を、`srwr mcp` と同じ[テープ](tape_ja.md)に記録する |
| `srwr view-server` | エディタ（VSCode 拡張・Vim スクリプト） | 表示サーバー。人は直接使わない（[protocol.md](protocol_ja.md)） |
| `srwr view [テープ]` | 人 | Vim で再生する（[vim.md](vim_ja.md)） |
| `srwr init` | 人 | 作業場を srwr 用に準備する |
| `srwr tapes` | 人 | テープの一覧・整理 |

`srwr --version` でバージョン、`srwr --help` で使い方が出る。

srwr が出す文言は英語が既定。日本語にするには、環境変数 `SRWR_LANG=ja`（[settings.md](settings_ja.md)）。

## 導入

```
go install github.com/amisonnet8/srwr/cmd/srwr@latest
```

ビルド済みのバイナリは、Linux・macOS・Windows（amd64・arm64）向けを GitHub Releases で配る。ファイル名は `srwr_<版>_<os>_<arch>.tar.gz`（Windows は `.zip`）で、中に `srwr`（`srwr.exe`）・`LICENSE`・`README.md` が入る。`checksums.txt` に全ファイルの SHA-256 が並ぶ（`sha256sum -c checksums.txt`）。cgo は使わないので、どの OS でも単一のバイナリで動く。

見る道具は、好みで選ぶ。

| 見る場所 | 入れるもの |
|---|---|
| VSCode | 拡張 srwr-view（[vscode.md](vscode_ja.md)）。拡張は `srwr` を含まないので、先に `srwr` を入れる（上） |
| Vim | なし。`srwr view` が、バイナリに埋め込んだ Vim スクリプトで起動する（[vim.md](vim_ja.md)） |

## 作業場のディレクトリ

srwr を使うディレクトリを**作業場**と呼ぶ。srwr は作業場の中に次を作る。

```
<作業場>/
├── .srwr/
│   ├── key                 HMAC の鍵（32バイト、0600）。共有しない
│   ├── lock                書き込みの排他用（flock）
│   ├── active              今のセッションのテープID
│   └── tapes/
│       ├── <id>.tape.jsonl   テープ（書き込み中のもの）
│       └── <id>.tape.jsonl.gz   終わったセッションのテープ（圧縮）
├── .srwrignore             記録しないファイルの指定（任意）
├── .mcp.json               srwr mcp の登録（srwr init が書く）
└── .claude/settings.json   hook の登録、Edit/Write の禁止（srwr init が書く）
```

## srwr mcp

AI に使わせる MCP サーバー。AI エージェント（MCP クライアント）が起動する。作業場は `--root <作業場>`（省略時はカレントディレクトリ）。ツールは `look`・`edit`・`replace`・`new` の4つ（[mcp.md](mcp_ja.md)）。同じ作業場で複数起動してもよい。同じセッション（同じテープ）に書き、片方が発行した範囲トークンをもう片方で使える（[tape.md](tape_ja.md)）。

## srwr hook

```
srwr hook [--root <作業場>]
```

Claude Code の hook から呼ばれ、AI が持つ既存のツールの操作を、同じテープに記録する。調べる過程（どこを読んだか）も、テープに載せるため。標準入力から hook の JSON を読む。動かした例は [hook.md](../examples/hook_ja.md)。

- 作業場は `--root`。省略すると環境変数 `CLAUDE_PROJECT_DIR`、それも無ければカレントディレクトリ
- 記録するのは `PostToolUse`（道具を使い終えたあと）の JSON だけ。使う項目は `hook_event_name`・`tool_name`・`tool_input`・`tool_response`・`cwd`。相対パスは `cwd` からの相対として読み、作業場の外のパスは記録しない
- **AI の作業を止めない。** 読めない JSON、無いファイル、扱えないファイル（バイナリ・CRLF）があっても、標準エラー出力に理由を出して終了コード 0 で終わる（終了コード 2 は Claude Code が道具の実行を止めるため、使わない）
- 登録は `.claude/settings.json`（`srwr init` が書く）：

```json
{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Read|Bash|Grep|Edit", "hooks": [{ "type": "command", "command": "srwr hook" }] }
    ]
  }
}
```

| Claude Code の操作 | テープへの記録 |
|---|---|
| Read | `look`（`offset`・`limit` なしは全体） |
| Bash の読み取り（`cat`・`nl`・`head`・`tail`・`sed -n 'A,Bp'`・`grep -n`） | `look` |
| Grep（内容モード） | `look`（連続する行は1つにまとめる） |
| Edit | `edit`（範囲は置換位置を含む行全体。`replace_all` は一致ごとに1件） |
| Write・MultiEdit・NotebookEdit | 記録しない（次に srwr が触れたとき、`external` として見える） |

- Bash のあとは（読み取りでなくても）、テープが内容を持つ全ファイルを読み直し、違えば `external`（`detectedBy` は `hook`）を記録する
- Bash のあと、git の作業ツリーでは、**新しいファイル**（git が未追跡か、インデックスに追加したばかり（`git add -N` を含む）と数え、無視せず、`.srwrignore` にもないもの）を、`created: true` の `external` として記録する。作ったものがテープに出る。CRLF のファイル、バイナリ、256 KiB を超えるファイルは対象外。1回のコマンドで記録する新しいファイルは 50 まで。超えた分は記録せず、hook が知らせる。git の作業ツリーの外では、新しいファイルは見つけられない
- Edit は、編集前の内容（テープが持つもの。無ければ `tool_response.originalFile`）に `old_string` → `new_string` を当てて、今のファイルと一致すれば `edit`。一致しなければ（ほかの変更も混ざっているなど）`external` として記録する
- 一度の呼び出しで記録する `look` は100件まで（広い範囲の検索で、テープが膨らまないように）
- パイプは先頭のコマンドだけを見る。`$( )`・書き込みのリダイレクト・`sed -i`・`tail -f`・`-n` なしの `grep` は記録しない
- hook が記録したコマは、`why` が `null` で表示される（`why` の行が出ない）。範囲トークンは持たない（`selection`・`from` は `null`）。`srwr mcp` が先に発行したトークンは、hook の `edit` のあとも、行番号が補正されて使える
- **1つのファイルの読み取りを記録したあと**（Read、または Bash の `cat`・`nl`・`head`・`tail`・`sed -n`。複数ファイルの検索や `grep -n` では出さない）、hook は AI に、`look`（`search`、または `startLine` と `endLine`）を使うよう1行で伝える。同じ行が `edit` に渡せる範囲トークンつきで返るため。`PostToolUse` の hook の JSON を標準出力に書き（`{"hookSpecificOutput": {"hookEventName": "PostToolUse", "additionalContext": "…"}}`）、終了コードは 0。読み取りを記録しなかったときは何も書かない

## srwr view-server

エディタが子プロセスとして起動する表示サーバー。テープを読んで、コマ送りに必要なデータを組み立てる。

```
srwr view-server --root <作業場>
```

人が直接使うものではない。エディタの対応を作る人は [protocol.md](protocol_ja.md) を読む。

## srwr view

`srwr view [テープ]` は、バイナリに埋め込んだ Vim スクリプトを使って、プラグインなしの Vim でテープを再生する。テープを省くと一覧から選ぶ。テープは、そのIDか、`.tape.jsonl`・`.tape.jsonl.gz` のファイルのパス。`--live` でライブ。作業場以外から実行するときは `--root <作業場>`。Vim 9.0.0784 以上が要る。使い方は [vim.md](vim_ja.md)。

## srwr init

```
srwr init [--lenient] [--root <作業場>]
```

作業場を srwr 用に準備する。何回動かしても同じ結果になり、変えるものがなければ何も書き換えない。

| 項目 | 内容 |
|---|---|
| `.srwr/` | ディレクトリと鍵を作る |
| `.mcp.json` | `srwr mcp` を登録する（既存のサーバーは残して追記。すでに `srwr` があれば触らない） |
| `.claude/settings.json` | hook の登録、Claude Code が聞かずに使えるようにする設定（`enabledMcpjsonServers` と、`look`・`edit`・`replace`・`new` の許可）、厳格モードなら Edit/Write の禁止（既存の内容は残して追記） |
| `.gitignore` | `.srwr/key`・`.srwr/lock`・`.srwr/active`・`.srwr/init-backup/` を足す（git の管理下の場合）。テープ（`.srwr/tapes/`）は共有できるよう無視しない |

- **srwr を新しいツールのある版に入れ替えたら、作業場ごとに `srwr init` をもう一度実行する。** 新しい名前の許可を `permissions.allow` に足し、なくなったツール（`select`・`sub`。`look`・`replace` になった）の許可を外す。しないと、Claude Code が毎回確認を出すか、ツールを使わない
- 既存のファイルを壊さない。キーの順や、ほかの設定・サーバー・hook はそのまま残す。書式は2字下げの JSON に整える
- 書き換える前の内容は `.srwr/init-backup/<日時>/` に残す（変えるファイルがあるときだけ）
- JSON として読めないファイルがあるとき（またはオブジェクト・配列の形が違うとき）は、何も書き換えずに終了コード 1 で止まる
- `srwr` が PATH にないときは、最後に注意を出す（`.mcp.json` と hook は `srwr` を呼ぶ）
- 対象のエージェントは **Claude Code のみ**（MCP 自体は他のエージェントでも使えるが、Edit/Write の禁止と hook は Claude Code の設定に依存する）

出力の例（空の作業場）：

```
作業場：/home/me/project

  作った   .srwr/                鍵 .srwr/key を作りました
  作った   .mcp.json             srwr mcp を登録しました
  作った   .claude/settings.json hook を登録し、Edit・Write などを禁止しました（厳格モード）
  作った   .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

準備できました。Claude Code を開き直すと、look / edit / replace / new が使えます。
緩いモード（Edit・Write を禁止しない）にするときは、srwr init --lenient。
```

### 厳格モードと緩いモード

| モード | 内容 |
|---|---|
| **厳格モード**（既定） | Claude Code の Edit・Write・MultiEdit・NotebookEdit を禁止する。AI がファイルを変える手段は `look` / `edit` だけになり、テープには必ず `why` が残る |
| **緩いモード**（`srwr init --lenient`） | Edit・Write を禁止しない。hook が Edit を `edit`（`why` は `null`）として記録する。Write は記録せず、`external` として見える |

- 厳格 ⇄ 緩いは、`srwr init` と `srwr init --lenient` で切り替えられる。緩いモードにすると、`permissions.deny` から上の4つ（`Edit`・`Write`・`MultiEdit`・`NotebookEdit`）を外す。それ以外の禁止は残す（利用者が自分で足した `Edit` の禁止も、同じ名前なので外れる）
- 厳格モードでも塞げないものがある。Bash 経由の編集（`sed -i`、リダイレクトなど）は、`external` として検知して見せる

## srwr tapes

```
srwr tapes [new | prune (--keep N | --older-than 30d) | path <テープID> | check [<テープID>]] [--root <作業場>]
```

| コマンド | 内容 |
|---|---|
| `srwr tapes` | 一覧（テープID、開始、最後の更新、イベント数、ファイル数、大きさ、今のセッションか）。新しい順（テープを始めた時刻の順）。時刻はその機械の時間帯（テープには UTC で持つ） |
| `srwr tapes new` | 今のセッションを閉じ、次の書き込みから新しいセッションにする。テープは[圧縮される](tape_ja.md#閉じたテープは圧縮する)（失敗したらテープはそのままで、警告を出す）。閉じる前に発行した範囲トークンは使えなくなる |
| `srwr tapes prune --keep N` / `--older-than 30d` | 古いテープを、圧縮したものも含めて消す。`--keep N` は新しい方から N 本を残す。`--older-than` は `30d`・`12h` の形。確認は聞かず、消したテープを1行ずつ出す。**今のセッションのテープは消さない** |
| `srwr tapes path <id>` | テープのパス（`.tape.jsonl` か `.tape.jsonl.gz`）を表示（共有するときに使う） |
| `srwr tapes check [<id>]` | git の作業ツリーで変わったのに、テープに出ないファイルを一覧にする。ID を省くと今のセッション（なければ一番新しいテープ）。読むだけ |

自動の整理はしない。消すのは利用者が明示的に行う。

### srwr tapes check

srwr が記録するのは、`look`・`edit`・hook が見たファイルだけ。AI がほかの方法（たとえばシェルのコマンド）で変えたファイルは、テープに出ないことがある。`check` は、テープと `git status` を比べて、それを知らせる。

- テープのどれかのイベントがそのファイルについてのものなら、そのファイルは**テープにある**
- 変わったのにテープにないファイルは**警告**。変わったファイルのうち[設定で記録しない](#記録しないファイル)ものは、別に数えて、警告にしない。`.srwr/` の中は除く
- テープを始めたとき、作業ツリーにすでに未コミットの変更があった（header の `dirty`）なら、警告の一部はテープより古いかもしれない、と添える
- 終了コードは、警告がなければ 0、あれば 1。git が要る。git の作業ツリーの外では、理由を出して 1 で終わる
- ロックは取らず、`.srwr/` も作らない

```
テープ 20261004-1530-ab12（今のセッション）
作業ツリーで変わったが、テープにないファイル（2）：
  M    docs/a.md
  ??   newfile.txt
変わったが、設定で記録しないファイル（1）：.env
テープにあるファイル：3
このテープを始めたとき、作業ツリーに未コミットの変更がありました。上のうち、テープより古いものがあるかもしれません。
```

出力の例（時間帯は `Asia/Tokyo`、`SRWR_LANG=ja`。テープIDの日時は UTC）：

```
  テープ              開始         最後の更新   イベント  ファイル  大きさ
  20261003-0812-k3f9  10/03 17:12  10/03 17:48  14        3          5.1 KB  ← 今のセッション
  20261002-0030-a8z1  10/02 09:30  10/02 11:05  62        7         21.4 KB

2 本（合計 26.5 KB）。再生は srwr view <テープ>、共有は srwr tapes path <テープ>。
```

## 記録しないファイル

テープはファイルの全文を持ち、共有するときはテープそのものを渡す。そのため、**秘密情報を含むファイルはテープに入れない**。

- **既定の対象**：`.env`、`.env.*`、`*.pem`、`*.key`、`id_rsa*`、`id_ed25519*`、`*.p12`、`*.pfx`、`.srwr/` の中
- **追加の指定**：作業場の `.srwrignore`（`.gitignore` と同じ書式）。`.gitignore` には従わない（生成物など、AI が扱ってよいファイルも含まれるため）

| 場面 | 振る舞い |
|---|---|
| `look` / `edit` | `ignored_file` エラー。テープには何も書かない |
| hook（Read・Bash・Grep・Edit） | 記録しない。`external` の検知の対象にもしない |
| 緩いモードで AI が Edit した | 記録しない |

細かい決まり：

- **既定の対象は、`.srwrignore` の `!` でも戻せない。** `!` が打ち消せるのは、`.srwrignore` 自身の指定だけ。ディレクトリが対象なら、その中のファイルも対象で、`!` で戻せない
- **大文字・小文字は区別しない。** macOS・Windows では `.ENV` で `.env` が開けるため、どの OS でも同じ判定にする
- **シンボリックリンクは、リンクの名前とリンク先の両方を調べる。** `notes.txt` が `.env` へのリンクなら、記録しない
- **`.srwrignore` は、作業場の直下のものだけを、呼ばれるたびに読む。** 書き足すとすぐ効く。すでにテープにあるファイルを書き足した場合も、以後は `look`・`edit` が `ignored_file` になり、`external` の検知も飛ばす
- **`.srwrignore` が読めないとき（権限がない、ディレクトリになっている）は、何も記録しない。** `look`・`edit` は `internal_error`、hook は標準エラー出力に一言だけ出す

### テープを共有する前に

- `.srwrignore` を見直す
- テープの中身を確認する（`srwr tapes` の一覧で、大きさとファイル数が分かる）
- 鍵（`.srwr/key`）は、再生に不要。共有しない
