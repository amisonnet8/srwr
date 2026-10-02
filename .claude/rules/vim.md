# Vim クライアント（srwr-view.vim）

目的は「**AIエージェントを動かしているターミナルから、そのまま Vim で見られる**」こと（`docs/design/overview.md`・`docs/reference/vim.md`）。この目的に反することをしない。

## 対応範囲

- **Vim 9.0.0784 以上**（`+channel`・`+job`・`+textprop`・`+vim9script`）。仮想テキストを行の上に出す機能は 9.0.0438 で入り、その不具合の修正を含む 9.0.0784 を最も古い版とする。このパッチ番号は、`plugin/srwr.vim` と `srwr view`（`internal/cli`）の2か所に書く。変えるときは両方と文書を直す
- 古い Vim に合わせた分岐（代わりの表示など）は持たない
- 機能が足りない Vim（古い版、`vim-tiny` など）では、起動時に `has()`・`has('patch-…')` で確かめ、何が足りないかを伝えて終わる
- Neovim は対象にしない。Neovim だけの機能（extmark、`vim.*`、Lua）を使わない

## 書き方

- **Vim9 script で書く**（`vim9script`・`def`）。従来の Vim script と混ぜない
- **他のプラグインに頼らない。** 表示サーバーとは `job_start()` と `nl` モードのチャンネル、`json_encode()`・`json_decode()` で話す
- `plugin/srwr.vim` はコマンドとキーの定義だけ。処理は `autoload/srwr/` に置き、`import autoload` で使う。利用者が設定する変数は `g:srwr_path` だけ（見た目の設定はない）
- srwr のバッファは `buftype=nofile`・`nomodifiable`・`noswapfile`・`bufhidden=wipe`。利用者のファイルを書き換えない
- キーの割り当ては srwr のバッファの中だけ（`<buffer>`）。グローバルなキーを奪わない
- ハイライトグループは `highlight default` で定義する。値は `handoff/checklist/captured/hl_dark.json`・`hl_light.json`（前の実装の実測値）に合わせる
- ウィンドウやバッファを作ったら、閉じるときに必ず片付ける。コマを行き来して増え続けないようにする

## Vim9 script の落とし穴（前のリポジトリで踏んだ）

- **`plugin/srwr.vim` の `vim9script` の前に書けるのは、`if … | finish | endif` のブロックだけ。** `let` などを書くと E1039。足りない機能の案内は、このブロックの中の `echomsg` で出す
- `return` の後ろに `# コメント` を書かない（古い Vim は式の続きと読み、E1096）
- autoload の関数をラムダの戻り値の型に使うと型が分からず E1013。`(): bool => …` と型を書く。関数型の変数の既定値にラムダは使わず、`def` の関数を入れる
- `prop_list()` は仮想テキストの本文を返さない（`id` が負の項目で見分ける）。`prop_remove()` は行を省くと全行が対象
- `:file` でバッファの名前を替えると、古い名前が非表示のバッファで残る。`bwipeout` で片付ける
- `win_execute()` に渡す文字列は Vim9 として動く。`let` は E1126 になるので `legacy let`。関数参照の変数名は大文字で始める。`hlget()` は名前を1つずつ渡す
- ラベルや文言は、コマの `file`・`range` から作る。クライアントはテープの形式を知らない

## UI

- 見え方は `docs/reference/vim.md`・`handoff/design/images/vim/`。**VSCode と同じ情報を同じ色で見せる**ことが確定。足さない
- `why` の行は、読み取り専用のバッファに実際の行として差し込む（長ければ折り返して全文）
- **ライブは、録画と同じ画面。** ライブの部品は、ライブとして開くことと、サーバーの通知を渡すことだけ。追いかけの判断は、録画のコマ送りと同じ処理を通す
- 差分のコマの「変わった行」は `diff_hlID()` で調べ、自前の色を付ける。Vim 標準の `Diff*` の色は、差分のコマを出している間だけ消し、終わると戻す
- 持ち越しの不具合2件（`dev/roadmap.md` の表）は、R5 のゲートで直し方を決め、回帰テストにする

## 埋め込みと起動（`srwr view`）

- `vim/plugin/`・`vim/autoload/` は `vim/embed.go` で `srwr` のバイナリに埋め込む。ファイルを足したら埋め込みの対象に入っているかを確かめる
- `srwr view` は、埋め込んだファイルを `os.UserCacheDir()/srwr/vim/<バージョン>/` に書き出し、`runtimepath` に足して Vim を起動する。利用者の `vimrc` は読み込んだままにする
- 同じリポジトリの `vim/` は、普通のプラグインとしても入れられる形を保つ

## テスト

- `qsoku vim-test`：`vim/test/test_*.vim` を、画面なしの Vim で1つずつ動かす（`vim -Nu NONE -i NONE -Es -V1 -S <ファイル> -c 'cquit 2' </dev/null`）
- テストは、失敗したら `cquit`（終了コード 1）、成功したら `qall!` で終わる。`v:errors` を使い、最後に空でなければ失敗にする
- **スクリプトが途中でエラーになると、Ex モードで標準入力を待って固まる。** そのため `-c 'cquit 2' </dev/null` を付けて動かす（`qsoku vim-test` はそうしてある）。`-V1` でエラーの内容が標準エラー出力に出る
- 表示サーバーを使うテストは、`qsoku bin` で作った `bin/srwr` を起動する
- **画面なしの Vim では、スクロール位置（`topline`）や見た目は確かめられない。** 疑似端末で本物の Vim を動かして `term_scrape()` で取る（R5 の画面の取得。例：`TERM=xterm script -qec "vim -Nu NONE -S 確認用.vim" /dev/null`）
- CI では ubuntu-latest の Vim で動かす
- **最も古い Vim（9.0.0784）での確認**：`git clone --filter=blob:none https://github.com/vim/vim.git` → タグ `v9.0.0784` を checkout → `./configure --with-features=huge --disable-gui --without-x --disable-nls --with-tlib=tinfo`（ncurses の `-dev` が無いときは、`libtinfo.so.6` への `libtinfo.so` のシンボリックリンクを作って `LDFLAGS=-L…` で渡す）→ `make` → `VIMRUNTIME=<clone>/runtime <clone>/src/vim …` でテストを1本ずつ。この手順も `qsoku` の項目にして、人間に手で打たせない
