# 一回りの例：`srwr init` から片付けまで

*[English](workflow.md) | **日本語***

**読者**：srwr を使い始める人。小さなプロジェクトを1つ使って、準備、AI に作業させる、何をしたか見る、テープに出ない変更がないか確かめる、セッションを新しくする、片付ける、までを通す。各コマンドの決まりは [cli.md](../reference/cli_ja.md)。

ここにあるコマンドは、すべて実際に動かしたもの。実行ごとに変わるのは、テープIDの最後の4文字、大きさ、作業場の場所だけ。

## 題材

git の作業ツリーにある小さな Go のプロジェクト。`main.go` と `README.md` がある。AI が作業するのは `main.go`：

```go
package main

import "fmt"

func main() {
	fmt.Println(greet(""))
}

func greet(name string) string {
	return "hello " + name
}
```

## 1. 準備（1回だけ）

プロジェクトで `srwr init` を実行する。Claude Code に srwr を登録し、Edit と Write を禁止して、すべての変更が `look` と `edit` を通るようにする。書くものは [cli.md](../reference/cli_ja.md#srwr-init) にある。

```console
$ srwr init
作業場：/work

  作った   .srwr/                鍵 .srwr/key を作りました
  作った   .mcp.json             srwr mcp を登録しました
  作った   .claude/settings.json hook を登録し、Edit・Write などを禁止しました（厳格モード）
  作った   .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

準備できました。Claude Code を開き直すと、look / edit / replace / new が使えます。
緩いモード（Edit・Write を禁止しない）にするときは、srwr init --lenient。
```

新しい設定を読ませるために、Claude Code を開き直す。`srwr init` が書いたものはコミットする。テープ（`.srwr/tapes/`）は、共有できるように、無視しない。

## 2. AI に作業させる

いつもどおりに頼む。たとえば「`greet` は名前が空だと `hello ` と出る。直して」。AI は `look` で見て、`edit` で変える。どちらにも理由がつき、すべての呼び出しが `.srwr/tapes/` のテープに残る。AI 側のやり取りは [look-edit.md](look-edit_ja.md)。

`srwr tapes` で、プロジェクトのテープの一覧が出る：

```console
$ srwr tapes
  テープ               開始         最後の更新   イベント  ファイル  大きさ
  20261004-1200-7k7f  10/04 12:00  10/04 12:00  3         1          1.9 KB  ← 今のセッション

1 本（合計 1.9 KB）。再生は srwr view <テープ>、共有は srwr tapes path <テープ>。
```

## 3. 何をしたか見る

テープをエディタで開いて、理由を行の上に見ながら、コマ送りで読む。Vim は `srwr view`（[vim.md](../reference/vim_ja.md)）、VSCode は拡張の *Operations*（[vscode.md](../reference/vscode_ja.md)）。`srwr view --live` は、書き込み中のテープを追いかける。

## 4. テープに出ない変更がないか確かめる

srwr が記録するのは、`look`・`edit` を通ったものと、hook が見たもの。ほかの方法（手で直した、シェルのコマンドで変えた）で変わったファイルは、テープに出ないことがある。`srwr tapes check` は、テープと `git status` を比べる。ここでは README を手で直した：

```console
$ srwr tapes check
テープ 20261004-1200-7k7f（今のセッション）
作業ツリーで変わったが、テープにないファイル（1）：
  M    README.md
テープにあるファイル：1
```

警告があれば終了コード 1、なければ 0。読むだけで、ロックも取らず、何も書かない。

## 5. セッションを新しくする

1本のテープは、作業のひとまとまり。次のまとまりを始めるには、`srwr tapes new` を実行する：

```console
$ srwr tapes new
今のセッション 20261004-1200-7k7f を閉じました。次の書き込みから、新しいテープになります。
```

- 次の書き込みから新しいテープになる。30分、イベントがないときも、セッションは自然に終わる
- 終わったセッションのテープは**圧縮**されて、`<id>.tape.jsonl.gz` になる。テープを読むものは、今までどおり開ける
- 古いセッションで AI がもらった範囲トークンは使えなくなる。AI が作業の途中なら、`look` からやり直す
- エディタからは、新しいセッションを始められない。ここ（ターミナル）で行う

AI がまた作業すると、新しいセッションの、新しいテープができる：

```console
$ srwr tapes
  テープ               開始         最後の更新   イベント  ファイル  大きさ
  20261004-1300-p2q9  10/04 13:00  10/04 13:00  3         1          1.9 KB  ← 今のセッション
  20261004-1200-7k7f  10/04 12:00  10/04 12:00  3         1          1.0 KB

2 本（合計 2.9 KB）。再生は srwr view <テープ>、共有は srwr tapes path <テープ>。
```

`srwr tapes path` は、テープのある場所を出す。共有するときに渡すファイルは、これ。古いテープは、圧縮したファイルになっている：

```console
$ srwr tapes path 20261004-1200-7k7f
/work/.srwr/tapes/20261004-1200-7k7f.tape.jsonl.gz
```

## 6. 片付ける

自動の整理はしない。`srwr tapes prune` は、古いテープを、圧縮したものも含めて消す。今のセッションのテープは消さない：

```console
$ srwr tapes prune --keep 1
消しました  20261004-1200-7k7f  （1.0 KB）
1 本を消しました。残り 1 本（1.9 KB）。
```

## 関連

- コマンド：[cli.md](../reference/cli_ja.md)
- テープの中身と、閉じたテープの置き方：[tape.md](../reference/tape_ja.md)
- AI が使うツール：[mcp.md](../reference/mcp_ja.md)
