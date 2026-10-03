# 配布方法

配布物は2つ（`srwr` のバイナリと、VSCode 拡張）。Vim スクリプトはバイナリに埋め込むので、別には配らない。`docs/reference/cli.md`（導入）・`docs/reference/vim.md`・`dev/roadmap.md` の R13。

## srwr（Go のバイナリ）

手順の全体（版、タグ、Release、Marketplace）は `dev/publish.md`。成果物は `qsoku dist` が `dist/` に作り、自分で検査する（6つの実行ファイルの形式と CPU、`.vsix` の中身、`checksums.txt`）。確認は `qsoku dist-try`（入れて開く）と `qsoku publish-check`（アップロード前）。

- **`go install github.com/amisonnet8/srwr/cmd/srwr@latest`** で入れられるようにする（最初の配布方法）
- GitHub Releases に、Linux・macOS・Windows（amd64・arm64）のビルド済みバイナリを置く。**リリースの作成は人間が行う**（`gh release create` は deny）
- **cgo は使わない**（`CGO_ENABLED=0`）。どの OS でも単一のバイナリで動くようにする。ロック（flock）など OS に依存する処理は、ビルドタグで OS ごとに分け、cgo なしで書く
- 外部依存を足さない（`.claude/rules/go-code.md`）
- **バージョン**：`srwr --version` は `runtime/debug.ReadBuildInfo()` のモジュールのバージョンを出す（`go install …@vX.Y.Z` で入れたときにタグが入る）。取れないとき（`go test` のバイナリなど）は `(devel)`。Go 1.24 以降は、リポジトリで `go build` したバイナリにも `v0.0.0-<日時>-<コミット>+dirty` の形のバージョンが入る
- テープの `header.tool.version` にも、同じバージョンを書く
- **Release は人間が作る**（タグを push し、GitHub で Publish）。公開されると `.github/workflows/release.yml` が `qsoku dist` を動かし、6つの圧縮ファイル（`srwr_<版>_<os>_<arch>.tar.gz`、Windows は `.zip`）・`checksums.txt`・`.vsix` を付ける。**タグのコミットからビルドしないと、`srwr --version` が `v0.0.0-…` になる**（workflow は `fetch-depth: 0`）。CI の `dist` ジョブも同じコマンドを毎回動かす
- ビルド済みバイナリはリポジトリにコミットしない（`/bin/`・`/dist/` は `.gitignore` 済み）

## srwr-view（VSCode 拡張）

- `.vsix` は `qsoku dist` が作る（`npx @vscode/vsce package --no-dependencies --baseContentUrl … --baseImagesUrl …`）。**拡張は `extension/` にあるので、README の相対パスの画像・リンクが正しい URL になるよう、この2つを渡す**（渡さないと `…/raw/HEAD/media/…` になって画像が出ない）。`@vscode/vsce` は devDependencies に入れず、使うときに `npx` で呼ぶ（依存を増やさない）
- VSCode Marketplace・Open VSX への公開は、人間が判断して行う。**コマンドでは公開せず、`.vsix` を Web サイトからアップロードする**（`dev/publish.md`。PAT・`vsce publish`・Actions は使わない）。公開前に Marketplace で `srwr-view` の名前が空いているかを手で確かめる
- 拡張は `srwr` のバイナリ（表示サーバー）を必要とする。場所は設定 `srwr.path`。**`.vsix` にバイナリを同梱しない**（R11 で決定。`srwr` は `go install` か GitHub Releases で別に入れる。理由は `docs/design/decisions.md`）。見つからないときは、拡張が入れ方を案内する
- 拡張の README（`extension/README.md`）は Marketplace に出る。**画像は PNG にする**（vsce は SVG を受けない。`qsoku readme-media` が作る）。相対パスの画像は vsce が公開時にリポジトリの URL に直す（R13 の `vsce package` で確かめる）
- 拡張のバージョンは `extension/package.json` の `version`。srwr のバイナリとは別に上げてよい。ただし、**対応する表示サーバーの `protocolVersion`** を拡張の README に書く

## srwr-view.vim（Vim スクリプト）

- `vim/plugin/`・`vim/autoload/` を `go:embed` で `srwr` に埋め込む。`srwr view` が `os.UserCacheDir()/srwr/vim/<バージョン>/` に書き出して使う。バージョンごとにディレクトリを分けるので、古い版と混ざらない
- 普段から Vim を使う人向けに、リポジトリの `vim/` をプラグインとして入れる方法を README に書く（このときも `srwr` のバイナリは要る）

## 互換性

- テープの形式を変えたら、古いテープを新しい表示サーバーで読めることを確かめる（`.claude/rules/tape.md`）
- 表示サーバーと拡張のバージョンが合わないときは、`initialize` の `protocolVersion` で分かり、利用者に分かる文言を出す。Vim スクリプトはバイナリに埋め込まれているので、食い違わない
