# コマンドライン（srwr）

**読者**：srwr を使う人。

`srwr` は Go の単一バイナリ。サブコマンドで役割が分かれる。

| コマンド | 使う人 | 役割 |
|---|---|---|
| `srwr mcp` | AI（MCP クライアント） | MCP サーバー（stdio）。[`select` / `replace`](mcp.md) を提供する |
| `srwr hook` | Claude Code の hook | Read・Bash・Grep・Edit を、`srwr mcp` と同じ[テープ](tape.md)に記録する |
| `srwr view-server` | エディタ（VSCode 拡張・Vim スクリプト） | 表示サーバー。人は直接使わない（[protocol.md](protocol.md)） |
| `srwr view [テープ]` | 人 | Vim で再生する（[vim.md](vim.md)） |
| `srwr init` | 人 | 作業場を srwr 用に準備する |
| `srwr tapes` | 人 | テープの一覧・整理 |

`srwr --version` でバージョン、`srwr --help` で使い方が出る。

## 導入

```
go install github.com/amisonnet8/srwr/cmd/srwr@latest
```

ビルド済みのバイナリは、Linux・macOS・Windows 向けを GitHub Releases で配る。cgo は使わないので、どの OS でも単一のバイナリで動く。

見る道具は、好みで選ぶ。

| 見る場所 | 入れるもの |
|---|---|
| VSCode | 拡張 srwr-view（[vscode.md](vscode.md)） |
| Vim | なし。`srwr view` が、バイナリに埋め込んだ Vim スクリプトで起動する（[vim.md](vim.md)） |

## 作業場のディレクトリ

srwr を使うディレクトリを**作業場**と呼ぶ。srwr は作業場の中に次を作る。

```
<作業場>/
├── .srwr/
│   ├── key                 HMAC の鍵（32バイト、0600）。共有しない
│   ├── lock                書き込みの排他用（flock）
│   ├── active              今のセッションのテープID
│   └── tapes/
│       └── <id>.tape.jsonl   テープ
├── .srwrignore             記録しないファイルの指定（任意）
├── .mcp.json               srwr mcp の登録（srwr init が書く）
└── .claude/settings.json   hook の登録、Edit/Write の禁止（srwr init が書く）
```

## srwr mcp

AI に使わせる MCP サーバー。AI エージェント（MCP クライアント）が起動する。作業場は `--root <作業場>`（省略時はカレントディレクトリ）。ツールは `select`・`replace` の2つ（[mcp.md](mcp.md)）。同じ作業場で複数起動してもよい。同じセッション（同じテープ）に書き、片方が発行した範囲トークンをもう片方で使える（[tape.md](tape.md)）。

## srwr hook

Claude Code の hook から呼ばれ、AI が持つ既存のツールの操作を、同じテープに記録する。調べる過程（どこを読んだか）も、テープに載せるため。標準入力から hook の JSON を読む。

| Claude Code の操作 | テープへの記録 |
|---|---|
| Read | `select`（`offset`・`limit` なしは全体） |
| Bash の読み取り（`cat`・`nl`・`head`・`tail`・`sed -n 'A,Bp'`・`grep -n`） | `select` |
| Grep（内容モード） | `select`（連続する行は1つにまとめる） |
| Edit | `replace`（範囲は置換位置を含む行全体。`replace_all` は一致ごとに1件） |
| Write・MultiEdit・NotebookEdit | 記録しない（次に srwr が触れたとき、`external` として見える。緩いモードでは `replace` として記録） |

- Bash のあとは、テープが内容を持つ全ファイルを読み直し、違えば `external` を記録する
- パイプは先頭のコマンドだけを見る。`$( )`・書き込みのリダイレクト・`sed -i`・`tail -f`・`-n` なしの `grep` は記録しない
- hook が記録したコマは、`why` が `null` で表示される（`why` の行が出ない）

## srwr view-server

エディタが子プロセスとして起動する表示サーバー。テープを読んで、コマ送りに必要なデータを組み立てる。

```
srwr view-server --root <作業場>
```

人が直接使うものではない。エディタの対応を作る人は [protocol.md](protocol.md) を読む。

## srwr view

`srwr view [テープ]` は、バイナリに埋め込んだ Vim スクリプトを使って、プラグインなしの Vim でテープを再生する。テープを省くと一覧から選ぶ。`--live` でライブ。作業場以外から実行するときは `--root <作業場>`。Vim 9.0.0784 以上が要る。使い方は [vim.md](vim.md)。

## srwr init

作業場を srwr 用に準備する。

| 項目 | 内容 |
|---|---|
| `.srwr/` | ディレクトリと鍵を作る |
| `.mcp.json` | `srwr mcp` を登録する（既存の内容は残して追記） |
| `.claude/settings.json` | hook の登録。厳格モードなら Edit/Write の禁止（既存の内容は残して追記） |
| `.gitignore` | `.srwr/key`・`.srwr/lock`・`.srwr/active` を足す（git の管理下の場合） |

- 既存のファイルを壊さない。書き換える前の内容は `.srwr/init-backup/` に残す
- 対象のエージェントは **Claude Code のみ**（MCP 自体は他のエージェントでも使えるが、Edit/Write の禁止と hook は Claude Code の設定に依存する）

### 厳格モードと緩いモード

| モード | 内容 |
|---|---|
| **厳格モード**（既定） | Claude Code の Edit・Write・MultiEdit・NotebookEdit を禁止する。AI がファイルを変える手段は `select` / `replace` だけになり、テープには必ず `why` が残る |
| **緩いモード**（`srwr init --lenient`） | Edit・Write を禁止しない。hook が `replace`（`why` は `null`）として記録する |

厳格モードでも塞げないものがある。Bash 経由の編集（`sed -i`、リダイレクトなど）は、`external` として検知して見せる。

## srwr tapes

| コマンド | 内容 |
|---|---|
| `srwr tapes` | 一覧（開始時刻、最後の更新、イベント数、ファイル数、大きさ、今のセッションか） |
| `srwr tapes new` | 今のセッションを閉じ、次の書き込みから新しいセッションにする |
| `srwr tapes prune --keep N` / `--older-than 30d` | 古いテープを消す。今のセッションは消さない |
| `srwr tapes path <id>` | テープのパスを表示（共有するときに使う） |

自動の整理はしない。消すのは利用者が明示的に行う。

## 記録しないファイル

テープはファイルの全文を持ち、共有するときはテープそのものを渡す。そのため、**秘密情報を含むファイルはテープに入れない**。

- **既定の対象**：`.env`、`.env.*`、`*.pem`、`*.key`、`id_rsa*`、`id_ed25519*`、`*.p12`、`*.pfx`、`.srwr/` の中
- **追加の指定**：作業場の `.srwrignore`（`.gitignore` と同じ書式）。`.gitignore` には従わない（生成物など、AI が扱ってよいファイルも含まれるため）

| 場面 | 振る舞い |
|---|---|
| `select` / `replace` | `ignored_file` エラー。テープには何も書かない |
| hook（Read・Bash・Grep・Edit） | 記録しない。`external` の検知の対象にもしない |
| 緩いモードで AI が Edit した | 記録しない |

### テープを共有する前に

- `.srwrignore` を見直す
- テープの中身を確認する（`srwr tapes` の一覧で、大きさとファイル数が分かる）
- 鍵（`.srwr/key`）は、再生に不要。共有しない
