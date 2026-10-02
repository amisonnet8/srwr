# テスト方針

## 開発環境の前提

開発者の手元環境は**Linux（devcontainer）のみ**。Windows・macOSでの動作確認は手元では行わず、GitHub Actionsのホステッドランナーに委ねる（`.github/workflows/ci.yml`の3OSマトリクス）。

## 実装後の動作確認

実装したら、ビルド確認に加えて実際の動作確認を行うこと。ロジック上正しそうに見えても、動かして初めて見つかる不具合はある。

- `qsoku check`（Go の vet・lint・単体テスト、他 OS 向けの型検査と lint（`cross`）、拡張のコンパイル・テスト、Vim のテスト。整形の差異は`lint`が報告する）は作業の区切りで必ず通す
- 並行性・排他制御に関わる変更は`qsoku race`も通す
- 依存を足したら`qsoku trivy`（既知の脆弱性・ライセンス）を通す
- シェルスクリプトを足したら`qsoku shellcheck`を通す（`git add`してから走らせる。追跡中のものだけが対象）

## srwr での動作確認

- **段階の完了条件**（`dev/roadmap.md`）を、実際に動かして確かめる。`srwr mcp`・`srwr hook`・`srwr view-server` は、`qsoku bin` で作った `bin/srwr` を `t.TempDir()` などの一時的な作業場で動かす
- **正解データ**：テープの読み取りは `extension/test/fixtures/*.expected.json`、コマの列は `extension/test/golden/`（`.claude/rules/tape.md`）。書き換えない
- **セッション・ロック・鍵**に関わる変更は `qsoku race` も通す
- **`internal/cli` の e2e テスト**は、本物の `srwr` を `go build` して、`srwr mcp` を複数のプロセスで動かす（`go test -short` では飛ばす）。セッションの30分は固定なので、間が空いた状態は、テープの日時を書き換えて作る（書き換えた日時を短くして、動いているサーバーに「テープが差し替わった」と分からせる）
- **拡張**：`qsoku ext`（`.claude/rules/extension.md`）。**Vim**：`qsoku vim-test`（`.claude/rules/vim.md`）
- **見た目**：自動で確かめられることはテストにし、人間には実物でしか分からないことだけを、1コマンドで見せる（`.claude/rules/working-with-human.md` 3章、`handoff/checklist/CHECKLIST.md`）。固定テープに無い場面は、本物の `srwr mcp` でテープを作るところまで自動にする
- **Claude Code での通しの確認**（R7・R9）は、この開発リポジトリではなく、一時的な作業場で行う。この開発リポジトリの `.claude/settings.json` を srwr 用に書き換えない

## 壊して確かめる（mutation-check）

通るだけのテストは、効いているとは限らない。実装を1か所ずつ壊して、テストが落ちるかを見る。手順・スクリプトは、mtqgのリポジトリ（https://github.com/amisonnet8/mtqg ）の`.claude/skills/mutation-check/`（`mutate.sh`が、変異を入れて、テストを走らせ、必ず元に戻す）を参考にする。生き残った変異は、握りつぶさずに読み、テストを足すか、同じ意味だと理由を書いて残す。

## Trivy：既知の脆弱性とライセンス

- `trivy fs`で、依存モジュールの既知の脆弱性（CVE）とライセンスを検査する（`qsoku trivy`）
- 何を失敗とみなすか（深刻度、禁止するライセンスの種類）は`trivy.yaml`で固定する
- `go get`で依存を足したら、必ずTrivyを通す
- Trivyは脆弱性データベースの取得に通信が要る（devcontainerのBashサンドボックスを使う場合は下記「Bashサンドボックスの落とし穴」参照）

## ShellCheck：シェルスクリプト

- シェルスクリプトは最小限にする。込み入った処理はGoで書く
- `qsoku shellcheck`は`git ls-files '*.sh' '*.bash'`で追跡中のものを列挙してShellCheckにかける
- シェルスクリプト中のコメントを、行頭が小文字の`# shellcheck`で始まる文にしてはいけない（ShellCheckがインラインディレクティブとして誤解釈し、SC1072/SC1073でパースエラーになる）。「ShellCheck」のように大文字を混ぜて書く

## `-race`の運用

- devcontainerは`CGO_ENABLED=0`。`-race`はcgoを要するため、`qsoku race`（`CGO_ENABLED=1 go test -race -count=1 ./...`）として`qsoku check`とは別にする
- CIでは`race`を別ジョブにし、`ubuntu-latest`・`macos-latest`に限る（`windows-latest`には標準でCコンパイラがない）

## Bashサンドボックスの落とし穴

> devcontainer内でClaude Codeが実行するBashコマンドは、既定でOSレベルのサンドボックス（Linux bubblewrap）にかかる。`.claude/settings.json`の`sandbox`でファイルシステムの書き込み先とネットワーク接続先を許可リストで絞っている。

- **Trivyの脆弱性DBの取得先は`ghcr.io`ではなく`mirror.gcr.io`（`mirror.gcr.io/aquasec/trivy-db:2`）。** 名前から`ghcr.io`（GitHub Container Registry）を許可すればよいと思い込むと、`qsoku trivy`が`sandbox_violations`で失敗する
- **Trivyの脆弱性DBは`~/.cache/trivy`に書き込む。** サンドボックスの`filesystem.allowWrite`にこれが無いと、ネットワークを許可してもダウンロード後の書き込みで`read-only file system`になる
- **`allowWrite`に書いたパスは、ディレクトリが既に存在しないと効かない。** 新しいコンテナには`~/.cache/golangci-lint`・`~/.cache/trivy`が無く、`~/.cache`自体は読み取り専用なので、`qsoku lint`・`qsoku check`は`failed to initialize build cache ... read-only file system`で**失敗**し、`qsoku trivy`もDBを保存できず終了コード1になる。`postCreate.sh`が`.claude/settings.json`の`allowWrite`を`jq`で読み、`~/`と絶対パス（`/go`など。`/dev`・`/proc`・`/sys`と相対パスは除く。作れなければ警告だけ）を`mkdir -p`して先に作る（`allowWrite`を変えても`postCreate.sh`は直さなくてよい）。**すでにあるコンテナを直すには、サンドボックスの外で該当のディレクトリを`mkdir -p`する**
- **Linuxでは`bubblewrap`（`bwrap`）と`socat`が無いと、`sandbox.enabled: true`でも黙って無効のままになる。** エラーも出ないので、サンドボックスが効いているつもりで効いていない状態になる。`postCreate.sh`が、無いものだけ`apt`で入れる（ベースイメージが持っていれば何もしない）
- **golangci-lintのキャッシュは`~/.cache/golangci-lint`に書き込む。** `filesystem.allowWrite`にこれが無いと、`qsoku lint`・`qsoku check`が`Failed to persist facts to cache ... read-only file system`の警告を出す。結果（`0 issues`）と終了コードには影響せず、キャッシュが効かなくなるだけ
- **`check.trivy.dev`への接続はTrivyのバージョン確認機能で、許可リストに無くても`qsoku trivy`の結果・終了コードには影響しない。** `sandbox_violations`として警告は出るが実害はないので、許可リストに足すかは任意
- **`git fetch`・`git pull`と`gh pr create`には`github.com`・`api.github.com`への許可が要る**
- **拡張の`npm ci`・`npm install`（`qsoku ext-deps`）には`registry.npmjs.org`への許可と、`~/.npm`への書き込みが要る**（どちらも`.claude/settings.json`に入れてある）
- **`srwr view` は `os.UserCacheDir()`（`~/.cache/srwr`）に書く。** サンドボックスでは `~/.cache` が読み取り専用なので、テストでは `XDG_CACHE_HOME` を一時ディレクトリに向ける
- **`.vscode/` はダミーのデバイスファイルで、サンドボックスの中からは作れない**（`mkdir` が「ファイルが存在します」）。`.vscode/launch.json` のように `.vscode/` に置くものは、内容を作業用ディレクトリに作り、`mkdir -p .vscode && cp …` を人間に頼む（`.claude/rules/working-with-human.md` 2章の形）
- **許可リストに無いものに当たったら、自分で回り道を探し続けない。** `.claude/rules/working-with-human.md` 2章の形で、何をどこに足すかを短く頼む
- **サンドボックスの書き込み保護が、作業ディレクトリ直下に`.bashrc`・`.gitconfig`などのダミーファイルを出現させることがある。** `git status`に大量の未追跡ファイルとして見えて驚くが、実害はない（未追跡のままなのでコミットには影響しない）。これはサンドボックス内のBashからだけ見える見かけ上のファイル（`/dev/null`のbind mount）で、サンドボックスの外の人間側には存在しない。`.gitignore`はルート直下のドットファイルを許可リスト方式にしてあり、`git add -A`で混ざらない

## GitHub Actions CIの落とし穴

- **`windows-latest`で`qsoku`を`go install`しても、内部で呼ぶ`sh`がそのままPATHに乗るとは限らない。** `qsokufile`のコマンドは常に`sh`（Git Bash付属）で実行される。CIの`run:`ステップは`shell: bash`を明示し、Git Bashが通ったPATHでqsokuを実行する
- **`/dev/stderr`等のUnix固有のパスはWindows（Git Bash）で壊れる。** `tee /dev/stderr`などを使わず、標準のリダイレクトだけで書く
- **Windowsのcheckoutで改行がCRLFになると、`gofmt -l`が全ファイルを未整形と誤検知する。** ルートの`.gitattributes`（`* text=auto eol=lf`）で防ぐ
- **`uses: owner/repo@TAG`はタグ名と厳密に一致しないと失敗する。** `v`の有無を見落としやすい。書く前に実際のタグ名を確かめる
- **Unix 専用の呼び出し（`syscall.Flock` など）は、Windows ではビルドできない。** OS ごとにビルドタグで分け、手元でも `qsoku cross`（`GOOS=windows`・`darwin` で `vet` と `lint`）を通す。手元の Linux だけでは見つからず、CI の Windows で初めて落ちる
- **手元の機械は JST で、CI は UTC。** 時刻を文字列で比べるテストは、手元でだけ通る。時刻は瞬間（`time.Time.Equal`）で比べる。時刻を扱う変更では、手元でも `TZ=UTC go test ./...` を流す（R3 で、`updatedAt` を `+09:00` の文字列で比べて、CI の3OSで落ちた）
- **Windowsには実行ビットの概念がない。** `os.Chmod`後に実行ビットを確かめるテストは、`runtime.GOOS != "windows"`でガードする
