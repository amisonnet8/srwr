# ディレクトリ構成

新しいファイルを追加する際は、どこに置くかをここで確かめる。**構成を変えたら、この文書も同じ変更の中で直す。**

下の図は、段階（`dev/roadmap.md`）を進めた後の姿。`(R2)` などの印は、その段階で作るもの。まだ無いディレクトリは、その段階で作る。

```
.
├── CLAUDE.md               ← プロジェクトルール（参照先の案内）
├── LICENSE                 （MIT）
├── README.md               ← 利用者向けの入口
├── qsokufile               ← ビルド・テストの近道（qsoku）
├── go.mod / go.sum         （module github.com/amisonnet8/srwr）
├── .gitattributes          （`* text=auto eol=lf`）
├── .gitignore
├── .golangci.yaml          ← lintの設定
├── trivy.yaml              ← 脆弱性・ライセンス検査の設定
├── .mcp.json               ← mtqgのMCPサーバーの配線（srwr の開発用。srwr 自身は登録しない）
├── cmd/srwr/               ← 単一バイナリ（R2）。main.go は引数を internal/cli に渡すだけ
├── internal/
│   ├── docs/               ← docs/ の検査だけのテスト（リンク切れ。R2・R3 で examples/ の照合を足す）（R0）
│   ├── tape/               ← テープの読み書き（イベントの型、追記、読み込み、テープから作る状態）（R1）
│   ├── token/              ← 範囲トークン（R1）
│   ├── jsonrpc/            ← 改行区切りの JSON-RPC（mcp と viewserver が共有）（R2）
│   ├── core/               ← select / replace の本体、行番号補正、external の検知（R2）
│   ├── session/            ← 今のセッションの決定・ロック（flock）・テープの読み足し・鍵（R2）
│   ├── tools/              ← MCP のツール定義（説明文・入力スキーマ）（R2）
│   ├── mcp/                ← MCP サーバー（R2）
│   ├── timeline/           ← コマの列・文書の状態・差分のコマ・最後の差分（R3）
│   ├── viewserver/         ← 表示サーバー（メソッド、ライブの見張り）（R3）
│   ├── hook/               ← Claude Code の hook の記録（R7）
│   ├── ignore/             ← 記録しないファイル（R8）
│   └── cli/                ← サブコマンド（R2 から順に）
├── extension/              ← srwr-view（VSCode 拡張、TypeScript）（R4）
│   ├── src/                ← server（通信）・present・replay・live・controls・sidebar・extension
│   ├── test/
│   │   ├── fixtures/       ← 実物のテープ。ui-check/ は固定テープ3本の作業場。**書き換えない**（最初からある）
│   │   └── golden/         ← コマの列の正解（22本）。**書き換えない**（最初からある）
│   ├── package.json
│   └── tsconfig.json
├── vim/                    ← srwr-view.vim（Vim9 script）（R5）
│   ├── plugin/srwr.vim     ← コマンドの定義だけ（vim9script の前に、足りない機能の検査）
│   ├── autoload/srwr/      ← 本体（server・replay・live・sidebar・diff・list など）
│   ├── test/               ← 画面なしの Vim で動かすテスト（test_*.vim）
│   └── embed.go            ← plugin/・autoload/ を srwr に埋め込む（go:embed）
├── tools/ui-check/         ← UI の確認の自動化（作業場づくり・画面の取得・比較・確認ページ）（R4〜R6）
├── docs/                   ← 外部向けの文書（正本）。reference/・design/・examples/
├── dev/                    ← 開発のうちうち。roadmap.md、review/<段階>/（UIゲートの資料）
├── handoff/                ← 引き継ぎ資料。**git に入れない（.gitignore）。読むだけ。R12 で消す**
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
- **`select` / `replace` の意味**（範囲の検証、行番号補正、内容の照合、external の検知）は `internal/core`。MCP にも hook にも依存しない
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
- 画面の取得・比較・確認ページの生成は `tools/ui-check/`（Go で書く。シェルスクリプトは最小限）。`qsoku ui-check`・`ui-live`・`ui-open` から呼ぶ
- 基準（前回 OK だった画面の記録）はリポジトリに入れる。結果（`ui-check-result/`）は入れない

## 各ファイル・ディレクトリの補足

- **`extension/test/fixtures/ui-check/go.mod`**（`module uicheck`）：**消さない。** 消すと、中の `.go` がルートのモジュールに入り、`qsoku build`・`vet`・`unit` が「found packages …」で失敗する
- **`extension/test/fixtures/`・`golden/`**: 最初から置いてある（`handoff/checklist/` からの写し）。書き換えない
- **`handoff/`**: `.gitignore` 済み。clone したときは無いので、前のリポジトリの `next-space/handoff/` を置く。中は書き換えない（例外は `.claude/rules/documentation.md`）
- **`extension/`・`vim/`**: `package.json`・`vim/test/` が無いあいだは、`qsoku ext`・`ext-build`・`vim-test` が「skipped」と出して成功する（`qsokufile`）
- **`.vscode/launch.json`**: **例外として置く。** VSCode の設定は原則 `devcontainer.json` にまとめるが、拡張を F5 で起動する設定は `devcontainer.json` に書けないため。`launch.json`（と、必要なら `tasks.json`）以外は置かない。`.gitignore` で `!/.vscode/` として戻してある
- **`go.sum`**: コミットする。srwr は外部依存ゼロなので、空か無いことがある
- **`extension/package-lock.json`**: コミットする（`npm ci` のため）
- **`.mtqg/`**: コミットされる。中のファイルは直接編集せず、すべて`mtqg`のコマンド経由で書く（`.claude/rules/mtqg-usage.md`）。`.mtqg/.local/`はコミットされない。前のリポジトリの記録は `handoff/reference/project-state/`
- **`.mcp.json`**: `mtqg mcp`（MCPサーバー）の配線。srwr の開発で srwr 自身を使うかは決めていない
- **`.srwr/`**: この開発リポジトリの直下では作らない（`.gitignore` の `/.*` で無視される）。`srwr` を動かすテストは `t.TempDir()` の中で行う
- **`.gitattributes`**: `* text=auto eol=lf`。Windowsランナーでの改行コード変換による誤検知を防ぐ。`.mtqg/`の中の`.gitattributes`は`mtqg init`が置くもので、別物
- **`.gitignore`のドットファイル**: ルート直下のドットファイル・ドットディレクトリは**許可リスト方式**（`/.*`を無視し、追跡するものだけ`!`で戻す。`.claude/`の中は`/.claude/*`を無視して`settings.json`・`rules/`・`skills/`・`hooks/`だけ戻す）。サンドボックスが作業ディレクトリ直下に出すダミーファイルを`git add -A`で混ぜないため。**追跡するドットファイルを新しく足すときは、`.gitignore`に`!`の行を足すこと**
- **配布物（ビルド済みバイナリ、`.vsix`）**: リポジトリにコミットしない。`.gitignore`には**ルート直下に限定して**書く（`/名前`）
- **シェルスクリプト**: 最小限にする。追跡中のものは`qsoku shellcheck`の対象になる
