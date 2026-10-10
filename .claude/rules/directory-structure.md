# ディレクトリ構成

新しいファイルを追加する際は、どこに置くかをここで確かめる。**構成を変えたら、この文書も同じ変更の中で直す。**

下の図は、段階（`dev/roadmap.md`）を進めた後の姿。`(R2)` などの印は、その段階で作るもの。まだ無いディレクトリは、その段階で作る。

```
.
├── CLAUDE.md               ← プロジェクトルール（参照先の案内）
├── LICENSE                 （MIT）
├── README.md / README_ja.md ← 利用者向けの入口（英語が基準）
├── qsokufile               ← ビルド・テストの近道（qsoku）
├── go.mod / go.sum         （module github.com/amisonnet8/srwr）
├── .gitattributes          （`* text=auto eol=lf`）
├── .gitignore
├── .golangci.yaml          ← lintの設定
├── trivy.yaml              ← 脆弱性・ライセンス検査の設定
├── .mcp.json               ← mtqgのMCPサーバーの配線（srwr の開発用。srwr 自身は登録しない）
├── cmd/srwr/               ← 単一バイナリ（R2）。main.go は引数を internal/cli に渡すだけ
├── internal/
│   ├── docs/               ← docs/ の検査だけのテスト（リンク切れ、英語版と日本語版の対、examples/ の本物との照合）（R0・R2・R3・R10.5）
│   ├── tape/               ← テープの読み書き（イベントの型、追記、読み込み、テープから作る状態、閉じたテープの圧縮と、生・`.gz` の両方を開く入口 `store.go`）（R1）
│   ├── token/              ← 範囲トークン（R1）
│   ├── jsonrpc/            ← 改行区切りの JSON-RPC（mcp と viewserver が共有）（R2）
│   ├── core/               ← look / edit / new の本体、行番号補正、external の検知（R2）
│   ├── session/            ← 今のセッションの決定・ロック（flock）・テープの読み足し・鍵（R2）
│   ├── tools/              ← MCP のツール定義（説明文・入力スキーマ）（R2）
│   ├── mcp/                ← MCP サーバー（R2）
│   ├── trace/              ← `srwr trace`：差分のテキストを読み、追加行を作ったテープの操作（edit・new と why）に結びつける。git は動かさず、テープも書かない（v0.1.15）
│   ├── timeline/           ← コマの列・文書の状態・差分のコマ・最後の差分（R3）
│   ├── viewserver/         ← 表示サーバー（メソッド、ライブの見張り）（R3）
│   ├── hook/               ← `srwr hook`：hook の JSON と Bash の読み取り・Grep の出力を読んで `core.HookRequest` を作る（R7）
│   ├── ignore/             ← 記録しないファイル（R8）
│   ├── vcs/                ← header の vcs：git の HEAD と未コミットの有無（R10）
│   ├── lang/               ← 画面や端末に出す文言の言語（`SRWR_LANG`）。英語が既定（R10.5）
│   ├── setup/              ← `srwr init`：`.mcp.json`・`.claude/settings.json`・`.gitignore` への追記、順序を保つ JSON、バックアップ（R9）
│   ├── uicheck/            ← 画面を比べる道具（端末の画面・承認した画像 SVG を同じ形にし、セルごとに比べる）（R5）
│   └── cli/                ← サブコマンド（R2 から順に）
├── extension/              ← srwr-view（VSCode 拡張、TypeScript）（R4）
│   ├── src/                ← server（通信）・timeline・lines・present・replay・live・controls・sidebar・config・extension
│   ├── test/
│   │   ├── fixtures/       ← 実物のテープ。ui-check/ は固定テープ3本の作業場。**書き換えない**（最初からある）
│   │   ├── golden/         ← コマの列の正解（22本）。**書き換えない**（最初からある）
│   │   └── baseline/       ← 画面の基準（`ja/`・`en/` それぞれに、録画3本・ライブ2本・長い理由。`capture.test.ts` が比べる）（R4・R10.5）
│   ├── media/srwr.svg      ← 左端のアイコン
│   ├── media/readme/       ← 拡張の README の画像（PNG。`qsoku readme-media` が作る）
│   ├── README.md / README_ja.md ← 拡張の README（Marketplace に出るのは `README.md`）
│   ├── package.json
│   └── tsconfig.json
├── vim/                    ← srwr-view.vim（Vim9 script）（R5）
│   ├── plugin/srwr.vim     ← コマンドの定義だけ（vim9script の前に、足りない機能の検査）
│   ├── autoload/srwr/      ← 本体（server・timeline・hl・buf・paint・sidebar・list・replay・diff・live・ui・config）
│   ├── test/               ← 画面なしの Vim で動かすテスト（test_*.vim）、helpers.vim
│   │   ├── screen/capture.vim ← 疑似端末で本物の Vim の画面を取る（外側の Vim）
│   │   └── baseline/       ← 画面の基準 `ja/`・`en/`（`ja/` は承認した画像から作った JSON）と、hl_*.json（色の実測値）
│   ├── screen_test.go      ← 画面を取って基準と比べる（`SRWR_SCREEN_TEST=1` のときだけ。`qsoku vim-test` が設定する）
│   └── embed.go            ← plugin/・autoload/ を srwr に埋め込む（go:embed）
├── tools/
│   ├── dist/               ← 配る物を作る：`qsoku dist`（6つのバイナリ・圧縮・`checksums.txt`・`.vsix`、自分で検査）と `publish-check`（R13）
│   ├── usage/              ← `qsoku usage`：Claude Code の会話ファイル（.jsonl）から、srwr の道具の引数の形・失敗・ほかの道具への逃げを数える
│   ├── ui-check/           ← UI の確認の自動化：`run`（qsoku ui-check）・`live`・`accept`・`open`、自動の検証の項目の表（checks.go）、長い理由のテープ（maketape.go）、確認ページ（page.go）（R4〜R6）
│   └── vim-oldest.sh       ← 最も古い Vim（9.0.0784）をビルドして `vim-test` を動かす（R5）
├── docs/images/            ← README の画像（バナーは手書き、デモ・VSCode の絵は `qsoku readme-media` で作る。コミットする）
├── docs/                   ← 外部向けの文書（正本）。英語が `名前.md`、日本語版が `名前_ja.md`。reference/・design/・examples/（`look-edit.md`・`workflow.md`（init から片付けまで一回りする例。`internal/docs` の `TestWorkflowExample` が、コマンドの出力を本物と照合する）・`protocol-session.md`・`hook.md`。`internal/docs` が本物と照合する。`internal/docs/testdata/demo-en/`（英語・UTC のテープ）と `demo/`（日本語・古い `+09:00` のテープ）が protocol-session の題材）
├── dev/                    ← 開発のうちうち。roadmap.md、publish.md（公開の手順）、review/<段階>/（UIゲートの資料。終わった段階は `<段階>.zip`）
├── .vscode/launch.json     ← 拡張を F5 で起動するためだけに置く（R4）
├── .devcontainer/          ← 開発環境（devcontainer.json、postCreate.sh）
├── .claude/                ← rules/（ルール）、hooks/、settings.json（人間が管理する）
├── .github/workflows/      ← CI
└── .mtqg/                  ← mtqgの記録
```

## 配置の判断基準

### Go
- **`cmd/srwr/main.go` は薄く**：引数を `internal/cli` に渡して終了コードを返すだけ
- **テープの形式に関わるもの**（イベントの型、JSON のフィールド、追記の決まり）は `internal/tape` に集める。ほかのパッケージがテープの JSON を直接組み立てない
- **`look` / `edit` / `new` の意味**（範囲の検証、行番号補正、内容の照合、external の検知）は `internal/core`。MCP にも hook にも依存しない
- **見せ方の元になる計算**（コマの列、各コマの文書の状態、差分のコマ、最後の差分）は `internal/timeline`。テープを読むだけで、書かない
- **入口ごとの変換**（MCP、表示サーバー、hook の入力、コマンドライン）は、それぞれ `internal/mcp`・`internal/viewserver`・`internal/hook`・`internal/cli`。改行区切りの JSON-RPC の読み書きは `internal/jsonrpc` に1つだけ置く
- **セッションとロック**は `internal/session`。テープに書く入口（`mcp`・`hook`）は、すべてここを通る
- 迷ったら「テープの形式か」「編集の意味か」「見せ方の計算か」「入口の変換か」で分ける

### 拡張（VSCode）
- 表示の部品は、コマの列と文書の状態だけを見て描く。テープを直接読まない
- 表示サーバーと話す部品（`server.ts`）は `vscode` を import しない
- テストの fixture は、Go が書いた**実物のテープ**を使う。手で書いたテープを fixture にしない

### Vim
- `plugin/srwr.vim` はコマンドとキーの定義だけにし、処理は `autoload/srwr/` に置く
- 表示サーバーとの通信は `autoload/srwr/server.vim` に1つだけ置く
- `vim/embed.go` は Go のパッケージ（`go:embed` は親ディレクトリを参照できないので、埋め込む側を `vim/` に置く）。`internal/cli` の `view` がこれを使う

### 確認の自動化
- 画面の取得・比較・確認ページの生成は `tools/ui-check/`（Go で書く。シェルスクリプトは最小限）。`qsoku ui-check`・`ui-live`・`ui-open` から呼ぶ。今あるのは `run`（`qsoku ui-check`）・`live`（`ui-live`）・`accept`（`ui-accept`）・`hook-try`（`qsoku hook-try`。R7 の確認）・`init-try`（`qsoku init-try`。R9 の確認）・`readme`（`qsoku readme-media`。README の画像）・`dist-try`（`qsoku dist-try`。R13 の確認）・`open vscode|vim`（`ui-open`。作業場づくり、VSCode／Vim を開く、ライブの追記）。画面の取得は、Vim が `internal/uicheck` の `CaptureVim`（`vim/screen_test.go` も同じ部品を使う）、VSCode が `extension/test/capture.ts`（`node --require ./out/test/setup.js out/test/capture.js <出力先> [<追加の作業場>]`）。比較・描画は `internal/uicheck`（`CompareBaseline`・`CompareShots`・`Grid.SVG`）
- 基準（前回 OK だった画面の記録）はリポジトリに入れる。結果（`ui-check-result/`）は入れない

## 各ファイル・ディレクトリの補足

- **`extension/test/fixtures/ui-check/go.mod`**（`module uicheck`）：**消さない。** 消すと、中の `.go` がルートのモジュールに入り、`qsoku build`・`vet`・`unit` が「found packages …」で失敗する
- **`extension/test/fixtures/`・`golden/`**: 最初から置いてある（前の実装の資料からの写し）。書き換えない
- **`extension/`・`vim/`**: `package.json`・`vim/test/` が無いあいだは、`qsoku ext`・`ext-build`・`vim-test` が「skipped」と出して成功する（`qsokufile`）
- **`.vscode/launch.json`**: **例外として置く。** VSCode の設定は原則 `devcontainer.json` にまとめるが、拡張を F5 で起動する設定は `devcontainer.json` に書けないため。`launch.json`（と、必要なら `tasks.json`）以外は置かない。`.gitignore` で `!/.vscode/` として戻してある
- **`go.sum`**: コミットする。srwr は外部依存ゼロなので、空か無いことがある
- **`extension/package-lock.json`**: コミットする（`npm ci` のため）
- **`.mtqg/`**: コミットされる。中のファイルは直接編集せず、すべて`mtqg`のコマンド経由で書く（`.claude/rules/mtqg-usage.md`）。`.mtqg/.local/`はコミットされない。
- **`.mcp.json`**: `mtqg mcp`（MCPサーバー）の配線。srwr の開発で srwr 自身を使うかは決めていない
- **`.srwr/`**: この開発リポジトリの直下では作らない（`.gitignore` の `/.*` で無視される）。`srwr` を動かすテストは `t.TempDir()` の中で行う
- **`.gitattributes`**: `* text=auto eol=lf`。Windowsランナーでの改行コード変換による誤検知を防ぐ。`.mtqg/`の中の`.gitattributes`は`mtqg init`が置くもので、別物
- **`.gitignore`のドットファイル**: ルート直下のドットファイル・ドットディレクトリは**許可リスト方式**（`/.*`を無視し、追跡するものだけ`!`で戻す。`.claude/`の中は`/.claude/*`を無視して`settings.json`・`rules/`・`skills/`・`hooks/`だけ戻す）。サンドボックスが作業ディレクトリ直下に出すダミーファイルを`git add -A`で混ぜないため。**追跡するドットファイルを新しく足すときは、`.gitignore`に`!`の行を足すこと**
- **配布物（ビルド済みバイナリ、`.vsix`）**: リポジトリにコミットしない。`.gitignore`には**ルート直下に限定して**書く（`/名前`）
- **シェルスクリプト**: 最小限にする。追跡中のものは`qsoku shellcheck`の対象になる
