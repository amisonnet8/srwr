# 配布方法

配布物は2つ（`srwr` のバイナリと、VSCode 拡張）。Vim スクリプトはバイナリに埋め込むので、別には配らない。`docs/reference/cli.md`（導入）・`docs/reference/vim.md`・`dev/roadmap.md` の R13。

## 配布物の作り方（goreleaser）

`srwr`のバイナリと`.vsix`は、`.goreleaser.yaml`・`.github/workflows/release.yml`・`tools/licenses`で作る（テンプレートと同じ形。2026-10-22、全PJでリリースの仕組みを統一した。前の自前の`qsoku dist`の、6つのバイナリを作る部分はやめる）。

- **cgo を使わない**（`CGO_ENABLED=0`）。どの OS でも、ほかのライブラリなしに動く
- 配布物は linux・darwin・windows の amd64・arm64。linux・darwin は`.tar.gz`、windows は`.zip`。名前は`<名前>_<版>_<OS>_<アーキテクチャ>`（版は`v`なし）。`checksums.txt`を添える。アーカイブには、実行ファイル・`LICENSE`・`README.md`・`README_ja.md`・`licenses/`（依存のライセンス文。srwr は外部依存ゼロなので、Go の`LICENSE`だけ）を入れる
- **`.vsix`**：goreleaser の`before`の手順が`qsoku vsix`で`bin/vsix/`に作り、`release.extra_files`で同じ Release に添付し、`checksum.extra_files`で`checksums.txt`にも入れる（下の「srwr-view」）
- **版**：goreleaser が`-X main.version=<タグ>`（`v0.1.0`の形）を埋める。`main`パッケージに`var version string`を置き、空のとき（`go install`で入れたもの）は`runtime/debug.ReadBuildInfo`のモジュールの版を出すと、どちらでも同じ`v0.1.0`になる（crube の`cli.BuildVersion`）。取れないとき（`go test` のバイナリなど）は `(devel)`。テープの`header.tool.version`にも、同じ版を書く
- **依存のライセンス文**を配布物に同梱する。`tools/licenses`が、3つの OS で実行ファイルに組み込まれるモジュールを`go list -deps`で求め、そのライセンスのファイルと Go の標準ライブラリの`LICENSE`を`bin/licenses/<モジュールのパス>/`に写す（goreleaser の`before`の手順）。ライセンスのファイルが無いモジュールがあれば失敗し、リリースが止まる
  - 依存を足す・外すと、`tools/licenses`の`TestRun`の一覧が落ちる（書き換えたあと）。新しいモジュールのライセンスを確かめてから一覧を直す（Trivy の`qsoku trivy`も通す）

## リリースの流れ

ビルド済みのバイナリなどを GitHub Releases で配るときは、次の流れにする。どのプロジェクトも同じ。

1. **人間が**タグ（`v0.1.0`の形）を打って push する。人間がするのはこれだけ
2. タグで動く workflow（`.github/workflows/release.yml`）が、検査を通してから配布物を作り、Release に添付して**公開する**（本文は goreleaser が前のタグからのコミットで作る）

- **Claude Codeは、タグも Release も作らない。** `gh release create`は`.claude/settings.json`で拒否（deny）されている。Release を作るのは workflow の中だけ
- `release.yml`は、まず`ci.yml`をそのまま呼んで（`workflow_call`）普段の CI の検査をすべて通し、そのあと goreleaser（`.goreleaser.yaml`。`goreleaser-action`の`version: "~> v2"`で、v2 の最新）で配布物を作る。書き込みの権限（`contents: write`）は、Release を作るジョブだけに与える
- goreleaser はいったん**下書き**に添付し、すべて添付し終えてから、最後の手順（`gh release edit --draft=false`）で公開する。途中で落ちると下書きだけが残り、公開された Release に配布物が欠けることはない。同じタグでもう一度動かすと、その下書きを置き換える（`replace_existing_draft`）。落ちたときは、原因を直したコミットに新しいタグを打つか、同じコミットのままなら workflow を再実行する
- **公開済みの Release があるタグでは、何も変えずに失敗する**（最初の手順）。出し直すときは、新しいタグを打つ
- **srwr だけ、Release の本文を絞る**（`.goreleaser.yaml` の`changelog.filters`。共通の設定から外れる。2026-10-22、人間の決定 q bbd517c9）。題が`chore`・`build`・`ci`・`test`・`docs(dev)`で始まるコミットは本文に載らない。利用者に見せる変更は`feat`・`fix`・`docs`で、内部の変更はそれ以外の題で書く。共通の設定を取り込み直すときに、この filters を消さない
- **タグを打つ前に、手元で`qsoku snapshot`を動かして中身を見る**（`.vsix`も`bin/vsix/`にできる。入れて開く確認は`qsoku dist-try`）。タグなしで`dist/`に同じ配布物を作る（直近のタグが`v0.2.0`なら、配布物の版は`0.2.1-snapshot`、バイナリの版は`v0.2.0`。タグが1つも無ければ`0.0.1-snapshot`と`v0.0.0`）。goreleaser は`.devcontainer/postCreate.sh`が入れる。コンテナを作り直す前に要るときは、`go install github.com/goreleaser/goreleaser/v2@latest`（Bashサンドボックスの中では、`proxy.golang.org`と`storage.googleapis.com`への接続が要る）。`.goreleaser.yaml`を変えたら`qsoku snapshot`で確かめる。CI の`dist`ジョブも、同じ配布物を毎回作って確かめる
- ビルド済みのバイナリと`.vsix`は、リポジトリにコミットしない（`/bin/`・`/dist/`・`*.vsix`は`.gitignore`済み。`.claude/rules/directory-structure.md`「配布物」）
- 手順の全体（版、タグ、Release、Marketplace）は`dev/publish.md`

## srwr（Go のバイナリ）

- **`go install github.com/amisonnet8/srwr/cmd/srwr@latest`** でも入れられるようにする（最初の配布方法）
- ロック（flock）など OS に依存する処理は、ビルドタグで OS ごとに分け、cgo なしで書く
- 外部依存を足さない（`.claude/rules/go-code.md`）
- Go 1.24 以降は、リポジトリで `go build` したバイナリにも `v0.0.0-<日時>-<コミット>+dirty` の形の版が入る（`-X main.version`の無いビルド）

## 取り消せない操作の前に（公開・タグの push・Release・Marketplace へのアップロード）

**公開したものは戻せない。** Marketplace は、公開した版を書き換えられず、版を消してもその番号は二度と使えず、拡張を Remove すると名前（ID）が永久に予約される（`dev/publish.md`）。最初の公開（0.1.0）で、README のバッジが廃止されていて色が付かなかったが、公開の前に確かめていなかった。だから、操作の**前**に、機械で確かめられることを全部確かめる。

1. README と `.vsix` が使う**外部の URL を、実際に取りに行く**。画像が返り、バッジが壊れた文言（retired・invalid・error・unavailable・404・500 など）を言っていないこと。`qsoku publish-check` がやる。結果は `ui-check-result/publish-check/index.html` に、ブラウザが読むのと同じに並べるので、人間も目で見る
2. 外部のサービス（バッジなど）は、**まだ公開していない対象で試して済ませない**。公開済みの別の対象でも先に試す。足す前に、そのサービスが今も動いていることを確かめる
3. `.vsix` の README が、リポジトリの `extension/README.md` と同じであること（`publish-check` が見る）
4. 上げる `.vsix` は、**タグのコミットから CI が作って Release に付けたもの**。上げる前に、それを取り出して 1〜3 を通す
5. 結果を人間に見せ、人間が「上げる」と言ったときだけ、操作の手順を渡す。**公開後の確認を、確認の代わりにしない**
6. 直す必要が出ても、人間に聞かずに、版を上げたり、決めたことを変えたりしない

## srwr-view（VSCode 拡張）

- `.vsix` は `qsoku vsix` が `bin/vsix/` に作る（goreleaser の`before`の手順。`npx @vscode/vsce package --no-dependencies --baseContentUrl … --baseImagesUrl …`）。**拡張は `extension/` にあるので、README の相対パスの画像・リンクが正しい URL になるよう、この2つを渡す**（渡さないと `…/raw/HEAD/media/…` になって画像が出ない）。`@vscode/vsce` は devDependencies に入れず、使うときに `npx` で呼ぶ（依存を増やさない）
- **GitHub Release への添付までは自動**（上の「リリースの流れ」）。VSCode Marketplace・Open VSX への公開は、人間が判断して行う。**コマンドでは公開せず、`.vsix` を Web サイトからアップロードする**（`dev/publish.md`。PAT・`vsce publish`・Actions は使わない）。公開前に Marketplace で `srwr-view` の名前が空いているかを手で確かめる
- 拡張は `srwr` のバイナリ（表示サーバー）を必要とする。場所は設定 `srwr.path`。**`.vsix` にバイナリを同梱しない**（R11 で決定。`srwr` は `go install` か GitHub Releases で別に入れる。理由は `docs/design/decisions.md`）。見つからないときは、拡張が入れ方を案内する
- 拡張の README（`extension/README.md`）は Marketplace に出る。**画像は PNG にする**（vsce は SVG を受けない。`qsoku readme-media` が作る）。相対パスの画像は vsce が公開時にリポジトリの URL に直す（R13 の `vsce package` で確かめる）
- 拡張のバージョンは `extension/package.json` の `version`。**srwr の版（タグから `v` を除いたもの）と同じにする**（2026-10-22、人間の判断。Release に `srwr` と版の違う `.vsix` が並ぶと、どれが対か分からない）。拡張が変わらない版でも上げる。タグを打つ前に、`package.json` と `package-lock.json`（2か所）を次の版にしてコミットする。`release.yml` の最初のジョブが、タグと `package.json` の版が違えば何も作らずに止める。また、**対応する表示サーバーの `protocolVersion`** を拡張の README に書く

## srwr-view.vim（Vim スクリプト）

- `vim/plugin/`・`vim/autoload/` を `go:embed` で `srwr` に埋め込む。`srwr view` が `os.UserCacheDir()/srwr/vim/<バージョン>/` に書き出して使う。バージョンごとにディレクトリを分けるので、古い版と混ざらない
- 普段から Vim を使う人向けに、リポジトリの `vim/` をプラグインとして入れる方法を README に書く（このときも `srwr` のバイナリは要る）

## 互換性

- テープの形式を変えたら、古いテープを新しい表示サーバーで読めることを確かめる（`.claude/rules/tape.md`）
- 表示サーバーと拡張のバージョンが合わないときは、`initialize` の `protocolVersion` で分かり、利用者に分かる文言を出す。Vim スクリプトはバイナリに埋め込まれているので、食い違わない
