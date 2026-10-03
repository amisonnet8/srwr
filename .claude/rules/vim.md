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
- **ユーザーコマンドの中からは、スクリプトの `import` が見えない**（`command! SrwrNext ui.Next()` は E121）。`plugin/srwr.vim` の中に `def Next()` を書き、`command! SrwrNext call <SID>Next()` で呼ぶ。さらに **`vim9script noclear`** にする（同じスクリプトをもう一度読むと、普通は `def` が消えて、コマンドだけが残る）
- **`'compatible'`（`vim -u NONE`）では行の継続（行頭の `\`）が使えない。** `plugin/srwr.vim` の最初の検査は1行で書く
- **autocmd の中は `Setup()` でなく `call Setup()`**（`autocmd OptionSet background Setup()` は何も起こさない）。`-Es` の Vim は `OptionSet` を送らないので、テストでは `doautocmd OptionSet background` を自分で送る
- **ファイルの種類のプラグインが、同じキーを割り当てる**（markdown の `ftplugin` は `]]`・`[[` を割り当てる）。`filetype` が変わるたび（バッファの名前を替えたあと）に、srwr のキーを割り当て直す。固定テープの `no-why` の TASK.md で見つかった
- **バッファの行を全部消すと「--バッファに行がありません--」が出る**（`deletebufline(buf, 1, '$')`）。先に新しい行で上書きし、余った行だけを消す
- `lines[0 : -1]` は「何も無い」でなく「全部」。先頭に差し込むときは、場合分けする
- **コマごとに、先頭（`topline: 1`）から表示してから、見えなければ動かす。** 前のコマの位置が残ると、同じコマが違う画面になる。動かすときは `zz` と同じ（理由の行を真ん中、ファイルの終わりより下は見せない）。承認した画像はこの動き
- `hlset()` の `default: true` は、設定のあるグループを替えない。自分が付けた値をおぼえておき（`applied`）、それと同じ間だけ `background` の変更で付け直す。**付けた値を、おぼえる前に利用者が替えていても、おぼえない**（おぼえると、次の `Setup()` で利用者の色を消す）
- **`WinResized` は 9.0.0784 に無い**（E216）。窓の大きさの変化は `VimResized,WinScrolled` で受ける。最も古い Vim で動かして初めて分かった
- **Vim の文言は言語で替わる**：タブ行の `[無名]`／`[No Name]`、diff の折りたたみの `行`／`lines`。画面の比較では、タブ行は比べず、折りたたみの文言は自前（`diff.FoldText`）にして、テストは `LC_ALL=C.UTF-8` で動かす
- 仮想テキストの本文は `prop_list()` で取れない。行番号の文字は、疑似端末の画面（`term_scrape()`）で確かめる
- Vim 9.0.0784 の環境は、`pkill` が効かないことがある。サーバーを止めるテストは `server.Pid()` を `kill` する

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
- 表示サーバーを使うテストは、`qsoku bin` で作った `bin/srwr` を起動する（`qsoku vim-test` が先に作る）
- **画面の取得と基準**（部品は `internal/uicheck/vimcap.go` の `CaptureVim`。`vim/screen_test.go` と `qsoku ui-check` が使う）：疑似端末（`script -qec`）の外側の Vim が、`term_start()` で内側の Vim（srwr-view.vim）を 140桁×50行の端末で動かし、`term_scrape()` でコマごとの画面を取る（`vim/test/screen/capture.vim`）。待ちは画面の条件（`sleep` で時間を待たない）。基準 `vim/test/baseline/*.json` は、承認した画像（`handoff/design/images/vim/`）から `go run ./tools/ui-check vim-baseline` で作ったもので、文字と背景は全部、前景は srwr が決める色（理由の行・範囲・丸・ステータス行）だけを比べる。Vim の構文の色は版で変わるので比べない。基準を替えるときは、理由を書いて人間の了承をもらう（`vim/test/baseline/README.md`）
- `term_scrape()` が返す文字は、全角が1つで幅 2。反転（`reverse`）は、色を入れ替えて見た目どおりにして記録する（ステータス行）
- 承認した画像の背景は、一番多い色が全体の下地になる。`Normal` の背景とは限らない（範囲が画面いっぱいのコマ）。`Normal` の背景は呼ぶ側が渡す
- **画面なしの Vim では、スクロール位置（`topline`）や見た目は確かめられない。** 疑似端末で本物の Vim を動かして `term_scrape()` で取る（R5 の画面の取得。例：`TERM=xterm script -qec "vim -Nu NONE -S 確認用.vim" /dev/null`）
- CI では ubuntu-latest の Vim と、最も古い Vim（`vim-oldest` ジョブ）で動かす
- **最も古い Vim（9.0.0784）での確認は `qsoku vim-oldest`**（`tools/vim-oldest.sh`）。手順は次のとおり：`git clone --filter=blob:none https://github.com/vim/vim.git` → タグ `v9.0.0784` を checkout → `./configure --with-features=huge --disable-gui --without-x --disable-nls --with-tlib=tinfo`（ncurses の `-dev` が無いときは、`libtinfo.so.6` への `libtinfo.so` のシンボリックリンクを作って `LDFLAGS=-L…` で渡す）→ `make` → `VIMRUNTIME=<clone>/runtime <clone>/src/vim …` でテストを1本ずつ。この手順も `qsoku` の項目にして、人間に手で打たせない
