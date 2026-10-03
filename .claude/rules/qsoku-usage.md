# qsokuの使い方

ビルド・テスト・lintなどの決まった作業を、リポジトリ直下の`qsokufile`に**名前を付けて**置き、`qsoku <名前>`で呼ぶツール。Makefileの代わり。手元（devcontainer）とCIで、同じ名前・同じコマンドで動かすために使う。**`qsokufile`に定義した作業は、`go`・`golangci-lint`などを直接打たず、`qsoku <名前>`で実行する。**

## このプロジェクトの名前

| 名前 | 内容 | いつ使うか |
|---|---|---|
| `qsoku build` | `go build ./...` | 編集後のビルド確認（Goファイルの編集後は、`.claude/hooks/build.sh`が自動で呼ぶ） |
| `qsoku fmt` | `golangci-lint fmt` | 整形を書き直したいとき |
| `qsoku vet` | `go vet ./...` | |
| `qsoku lint` | `golangci-lint run` | |
| `qsoku unit` | `go test ./...` | |
| `qsoku go-check` | `vet`・`lint`・`unit`をまとめて実行 | Go だけ確かめたいとき（CI の check ジョブ） |
| `qsoku cross` | `GOOS=windows`・`darwin` で `vet` と `lint` | OS に依存するコードを触ったとき（`check` に含む） |
| `qsoku ext-deps` | `extension/`で`npm ci`（lockが無ければ`npm install`） | 拡張の依存を入れ直すとき |
| `qsoku ext-build` | 拡張のコンパイル | 拡張の編集後（`.claude/hooks/build.sh`が自動で呼ぶ） |
| `qsoku ext` | `bin/srwr` をビルドしてから、拡張のコンパイルとテスト | 拡張を確かめたいとき（CI の extension ジョブ） |
| `qsoku ui-open vscode <テープ>` | 固定テープの作業場を一時ディレクトリに作り、`.vsix` を入れた VSCode で開く。テープは `why-basic`・`external`・`no-why`・`long-why`（長い理由。本物の `srwr mcp` でその場で作る）・`live`（ライブは3秒ごとに追記） | **人間に VSCode の見た目を見てもらう前**（`.claude/rules/working-with-human.md` 3章）。`code` コマンドが要る |
| `qsoku ui-open vim <テープ>` | 同じ作業場を作り（`-light` を付けると白い背景）、手順を番号つきで出して、Enter のあと `bin/srwr view` で Vim を開く（`live` は5秒後から3秒ごとに追記） | **人間に Vim の見た目を見てもらう前**。人間の端末で動かす |
| `qsoku ui-check` | **UI の確認を1コマンドで**。自動の検証（CHECKLIST 2章の14項目。名前の付いた既存のテストを走らせる）、Vim と拡張の画面の取得、基準との比較、長い理由のテープ（本物の `srwr mcp` でその場で作る）、確認ページ `ui-check-result/latest/index.html` と `result.json` の保存。数十秒。違い・失敗があれば終了コード 1 | **人間に UI を見てもらう前**。人間に渡すのはこの1コマンドと、できたページだけ |
| `qsoku ui-live` | `ui-check` のライブだけ（Vim の live-basic・live-external、拡張の live_basic・live_ext）。`qsoku ui-live --watch vim`（`vscode`）は、人間が流し込みを見て体感するために、ライブを開く（`ui-open … live` と同じ） | ライブに関わる変更 |
| `qsoku ui-accept` | 最後の `ui-check` の結果に、人間の判断を記録する。引数なしが OK（違う画面・新しい画面が次の基準になり、基準の README に日付が付く）。`qsoku ui-accept ng <ひとこと>` は NG（基準はそのまま）。自動の検証が落ちている結果は OK にできない | 人間がチャットで OK／NG を答えたあと、AI が動かす |
| `qsoku hook-try` | R7 の確認。Claude Code が働く作業場（バグが2つの小さな Go のプロジェクト、`.mcp.json` に `srwr mcp`、`.claude/settings.json` に hook と許可）を作り、VSCode で開く。AI の作業のあと、記録されたもの（hook の select・replace、mcp の select・replace の件数）を ○ × で出し、Enter で Vim が開いてそのテープを再生する | **人間に hook の動きを見てもらう前**。人間の端末で動かす。AI は Claude Code を動かさない |
| `qsoku vim-test` | `bin/srwr` を作り、`vim/test/test_*.vim`を画面なしのVimで実行。続けて、疑似端末で本物のVimの画面を取り、基準と比べる（`vim/screen_test.go`） | Vimスクリプトを触ったとき（CI の vim ジョブ） |
| `qsoku vim-oldest` | Vim 9.0.0784 をソースからビルドして（初回だけ）、それで `vim-test` | Vim に関わる変更の区切り（CI の vim-oldest ジョブ）。`SRWR_VIM_OLDEST_DIR` でビルド先を替えられる |
| `qsoku check` | `go-check`・`cross`・`ext`・`vim-test`をまとめて実行 | **作業の区切りで必ず通す**（`.claude/rules/testing.md`） |
| `qsoku bin` | `go build -o bin/srwr ./cmd/srwr` | 手元で`srwr`を動かして確かめるとき（R2 以降） |
| `qsoku race` | `CGO_ENABLED=1 go test -race` | 並行性・排他に関わる変更 |
| `qsoku trivy` | 既知の脆弱性・ライセンスの検査 | 依存を足したとき |
| `qsoku shellcheck` | 追跡中の`*.sh`・`*.bash`をShellCheckにかける | シェルスクリプトを足したとき |

`extension/package.json`・`vim/test/`が無いあいだは、`ext`・`ext-build`・`vim-test`は「skipped」と出して成功する。確認の自動化は、`ui-open vscode` が R4、`ui-open vim` が R5、`ui-check`・`ui-live`・`ui-accept` が R6 で入った。

一覧は、今ある`qsokufile`の内容そのもの。`qsoku .list`（名前とコマンド）か`qsoku .names`（名前だけ）で確かめる。**引数なしで`qsoku`とだけ打っても、使い方と定義済みの名前が出る。**

## 呼び方

```
qsoku <名前> [引数...]
```

- 引数は、項目のコマンドの中で`"$@"`・`$1`として受けられる。**このプロジェクトの`qsokufile`の項目は、どれも引数を受けていない**（書いていない引数は捨てられる）。受けたいときは、項目を`unit: (cd //; go test "$@")`のように書き換える。そうすれば`qsoku unit -run TestFoo ./internal/...`が使える
- **qsoku自身のオプションは無い。** `--help`・`--version`は**名前として調べられる**ので、「定義されていない」というエラーになる。使い方は`qsoku .help`、バージョンは`qsoku .version`
- コマンドは、どのOSでも、また呼び出し元がどのシェルでも、常に`sh`で実行される
- 標準入出力・標準エラー出力はそのまま素通し。終了コードは、コマンドのものがそのまま返る

### 終了コード

| コード | 意味 |
|---|---|
| 0 | 成功 |
| 1 | qsokuが頼まれたことをできなかった（`qsokufile`が無い・解析できない、`sh`を起動できない、など） |
| 2 | コマンドラインの誤り（**定義されていない名前**、管理用コマンドの引数の数の誤り） |
| それ以外 | 項目のコマンドが実行された後は、そのコマンドの終了コード（qsokuが差し替えることは無い） |

## 管理用コマンド

`.`で始まる。`qsokufile`の名前は`.`で始められないので、名前とぶつからない。

| コマンド | 内容 |
|---|---|
| `qsoku .list` | 定義されている名前とコマンドを、ファイルの順に出す |
| `qsoku .names` | 名前だけを出す（補完が使う。決して失敗しない） |
| `qsoku .add <名前> <コマンド>` | 項目を足す。**すでにある名前なら、その行をその場で置き換える**（コメントや並び順はそのまま） |
| `qsoku .rm <名前>` | 項目を消す。無い名前はエラー |
| `qsoku .edit` | `$EDITOR`で`qsokufile`を開く |
| `qsoku .where` | `qsokufile`のあるディレクトリを出す |
| `qsoku .init` | カレントディレクトリに空の`qsokufile`を作る |
| `qsoku .version` | qsokuのバージョン |
| `qsoku .shell <shell>` | シェル連携のコードを出す（下記） |
| `qsoku .help` | 使い方と、定義済みの名前 |

## `qsokufile`の書き方

```
# 行頭が#の行はコメント。
build: (cd //; go build ./...)
unit: (cd //; go test "$@")
check: qsoku vet && qsoku lint && qsoku unit
```

- **1行が1項目**：`名前: コマンド`。最初の`:`が区切り。**行の継続は無い**（1項目は必ず1行）
- コメントは**行頭が`#`の行だけ**。行の途中・末尾の`#`はqsokuには特別な意味が無く、`sh`がシェルのコメントとして読む
- 名前に使えるのは、英数字（ASCII）・`.`・`_`・`-`。**先頭を`.`にはできない**。大文字・小文字は区別される
- **同じ名前を2回定義するとエラー**（両方の行番号が出る）。何も実行されない
- 別の項目を呼ぶときは、`qsoku <名前>`と書く（`check`の例）
- **項目の中で`exit`を呼ばない。** qsokuが同じスクリプトの末尾に後始末の行を足すので、`exit`があるとそこで終わって届かない。成否は、項目の最後のコマンドの終了コードに任せる

### `//`：`qsokufile`のあるディレクトリ

`//`は、使われている`qsokufile`のあるディレクトリ（環境変数`QSOKU_ROOT`）に置き換わる。**どのディレクトリから`qsoku`を呼んでも、リポジトリ直下で動かすために**、項目は`(cd //; ...)`の形で書く。

- 置き換わるのは、**引用符の外で、単語の先頭**にあるとき（コマンドの先頭、または空白・`;`・`&`・`|`・`(`の直後）
- 置き換わらない：`http://example.com`（`:`の後）、`"//"`・`'//'`（引用符の中）、コメントの中
- **括弧で囲む**（`(cd //; ...)`）と、`cd`が子プロセスの中に閉じるので、呼び出し元のシェルの居場所は動かない。括弧なしの`cd //src && ...`は、呼び出し元のシェルもそこへ動く（シェル連携が居場所を持ち帰るため）
- 引用符の中で場所を使いたいときは、`$QSOKU_ROOT`を直接書く

### 探し方

`qsokufile`は、カレントディレクトリから**親へ順にたどって最初に見つかったもの**が使われる。gitリポジトリの境界では止まらない。サブディレクトリからでも同じ`qsokufile`が使える。見つからなければ、エラー（終了コード1）で`qsoku .init`を案内する。

## 名前を足す・変える

- 新しい作業は、`qsoku .add <名前> '<コマンド>'`で足すか、`qsokufile`を直接編集する。**足したら、上の「このプロジェクトの名前」の表と、`.claude/rules/testing.md`の該当箇所も同じ変更の中で直す**
- CIで走らせたい作業は、`.github/workflows/ci.yml`の`run: qsoku <名前>`に書く（手元とCIでコマンドが食い違わない）
- 例：e2eテストを足す

  ```
  test: (cd //; go test -tags e2e -count=1 ./e2e/...)
  ```

## シェル連携

`.devcontainer/postCreate.sh`が、`~/.bashrc`に次の1行を足す。

```
eval "$(qsoku .shell bash)"
```

これで、次の2つが入る。

- **`qsoku`というシェル関数**：項目が`cd`した居場所を、呼び出し元のシェルへ持ち帰る。関数の中は`command qsoku`で本物のバイナリを呼ぶ
- **TAB補完**：`qsoku `の後でTABを押すと、その場で`qsoku .names`を呼んで、今ある`qsokufile`の名前を出す。管理用コマンド（`.add`など）は、シェルのスクリプトに固定で入っている

## OSごとの注意

- **Windows**：`qsokufile`のコマンドは、Git for WindowsのMSYS `sh.exe`で動く。`Git\bin`（`C:\Program Files\Git\bin`）を`PATH`に足すこと。CIの`run:`は`shell: bash`を明示する（`.claude/rules/testing.md`「GitHub Actions CIの落とし穴」）
- **Git Bash（Windows）**：引数が`//`で始まると、MSYSが先に書き換えてしまい、qsokuに届くのは`/`1つになる。`MSYS_NO_PATHCONV=1`で止められる
