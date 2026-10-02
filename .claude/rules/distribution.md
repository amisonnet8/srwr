# 配布方法

配布物は2つ（`srwr` のバイナリと、VSCode 拡張）。Vim スクリプトはバイナリに埋め込むので、別には配らない。`docs/reference/cli.md`（導入）・`docs/reference/vim.md`・`dev/roadmap.md` の R11。

## srwr（Go のバイナリ）

- **`go install github.com/amisonnet8/srwr/cmd/srwr@latest`** で入れられるようにする（最初の配布方法）
- GitHub Releases に、Linux・macOS・Windows（amd64・arm64）のビルド済みバイナリを置く。**リリースの作成は人間が行う**（`gh release create` は deny）
- **cgo は使わない**（`CGO_ENABLED=0`）。どの OS でも単一のバイナリで動くようにする。ロック（flock）など OS に依存する処理は、ビルドタグで OS ごとに分け、cgo なしで書く
- 外部依存を足さない（`.claude/rules/go-code.md`）
- **バージョン**：`srwr --version` は `runtime/debug.ReadBuildInfo()` のモジュールのバージョンを出す（`go install …@vX.Y.Z` で入れたときにタグが入る）。取れないとき（`go test` のバイナリなど）は `(devel)`。Go 1.24 以降は、リポジトリで `go build` したバイナリにも `v0.0.0-<日時>-<コミット>+dirty` の形のバージョンが入る
- テープの `header.tool.version` にも、同じバージョンを書く
- ビルド済みバイナリはリポジトリにコミットしない（`/bin/`・`/dist/` は `.gitignore` 済み）

## srwr-view（VSCode 拡張）

- `.vsix` を作って配る（`npx @vscode/vsce package`）。`@vscode/vsce` は devDependencies に入れず、使うときに `npx` で呼ぶ（依存を増やさない）
- VSCode Marketplace・Open VSX への公開は、人間が判断して行う。公開前に Marketplace で `srwr` の名前が空いているかを手で確かめる
- 拡張は `srwr` のバイナリ（表示サーバー）を必要とする。場所は設定 `srwr.path`。OS ごとの `.vsix` にバイナリを同梱するかは R11 で決める（`docs/design/limitations.md` の未定）
- 拡張のバージョンは `extension/package.json` の `version`。srwr のバイナリとは別に上げてよい。ただし、**対応する表示サーバーの `protocolVersion`** を拡張の README に書く

## srwr-view.vim（Vim スクリプト）

- `vim/plugin/`・`vim/autoload/` を `go:embed` で `srwr` に埋め込む。`srwr view` が `os.UserCacheDir()/srwr/vim/<バージョン>/` に書き出して使う。バージョンごとにディレクトリを分けるので、古い版と混ざらない
- 普段から Vim を使う人向けに、リポジトリの `vim/` をプラグインとして入れる方法を README に書く（このときも `srwr` のバイナリは要る）

## 互換性

- テープの形式を変えたら、古いテープを新しい表示サーバーで読めることを確かめる（`.claude/rules/tape.md`）
- 表示サーバーと拡張のバージョンが合わないときは、`initialize` の `protocolVersion` で分かり、利用者に分かる文言を出す。Vim スクリプトはバイナリに埋め込まれているので、食い違わない
