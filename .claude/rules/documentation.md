# ドキュメントの書き方

## 文書の一覧と役割

| 文書 | 役割 | 変えるとき |
|---|---|---|
| `docs/reference/` | **正本**。使い方と守る約束：`cli.md`・`settings.md`・`mcp.md`・`tape.md`・`protocol.md`・`vscode.md`・`vim.md` | 仕様を変えるとき。先に文書、後で実装 |
| `docs/reference/vscode.md`・`vim.md` の画面の仕様 | VSCode と Vim の見た目と振る舞い（**確定。変えない・足さない**） | 新しい画面だけ、UIゲートで了承をもらってから足す |
| `docs/design/` | 開発に加わる人向け：`overview.md`・`decisions.md`（判断と理由）・`token.md`・`limitations.md`（制限・未定・追加の候補） | 方針や判断が変わったとき。未定が決まったら `limitations.md` から消して該当の文書へ |
| `docs/examples/` | 実際に動かして取ったやり取りの例。`internal/docs` のテストが本物との一致を確かめる（R2・R3） | 挙動を変えたとき（テストが落ちる） |
| `dev/` | 開発のうちうち：`roadmap.md`（段階）、`review/`（UIゲートの資料） | 段階が進んだとき |
| `handoff/`（git に入れない） | 引き継ぎ資料。確定したデザインと UI の振る舞い（`design/`・`ui/`）、確認の方針（`checklist/`）、前の決まり（`reference/`） | **変えない**。例外：UIゲートで決まった新しい画面だけ、`design/DESIGN.md`・`images/`・`ui/UI.md` に足す |
| `README.md`・`README_ja.md` | 利用者向けの入口（GitHub の最初の画面）。英語が基準、日本語版は `_ja`。冒頭の画像（`docs/images/`）は `qsoku readme-media` で作る | 使い方が変わったとき |
| `extension/README.md`・`extension/README_ja.md` | 拡張の利用者向けの説明（Marketplace に出る）。対応する `protocolVersion` を含む。画像は PNG（`extension/media/readme/`。vsce は README の SVG を受けない）。`README_ja.md` は Marketplace には出ない | 拡張の振る舞いが変わったとき |
| `CLAUDE.md`・`.claude/rules/` | 開発のルール | 気づき・落とし穴が見つかったとき（`CLAUDE.md` 参照） |

- `docs/` は、リポジトリに残る外部向けの文書。**`handoff/` や `dev/` を `docs/` から参照しない**（`handoff/` は最後に消す）
- 進捗・決定・質問は文書ではなく **mtqg** に記録する（`.claude/rules/mtqg-usage.md`）

## 仕様と実装がずれたとき

- **`docs/` が正本。** 作り直しなので、実装を `docs/` に合わせる
- `docs/` 自体に誤りや矛盾を見つけたら、黙って実装に合わせない。重要な判断として、推奨案つきで聞く（`.claude/rules/working-with-human.md` 1章）
- `docs/` と `handoff/`（前の実装から起こした資料）が食い違うときは、`handoff/design/`・`handoff/ui/`（評価者が確認した見た目と振る舞い）を正とし、`docs/` を直したうえで報告する
- 仕様を直すと決まったら、**同じ変更の中で**文書とコードを直す
- **例外：コードが固定データ（fixture・golden・承認した画面）と合っていて、`docs/` の書き方だけが誤っているときは、`docs/` を直す**（形式もプロトコルも変えない）。R11 で、`tape.md`（省略されるフィールド）・`protocol.md`（`tapeId` の規則）・`token.md`（トークンの長さ）・外部変更のあとのトークンの扱いを、実際に `srwr mcp` を動かして確かめて直した。文書の例（範囲トークンなど）は、本物と同じ形にする（`internal/docs/tokens_test.go`）

## 書き方

- **`docs/` の文書は英語が基準。** 日本語版は、同じ名前に `_ja` を付ける（`reference/cli.md` と `reference/cli_ja.md`）。2つは対で、どちらも先頭（3行目）に、言語の切り替えを置く（英語版は `*[日本語](名前_ja.md) | **English***`、日本語版は `*[English](名前.md) | **日本語***`）。**仕様を直すときは、同じ変更の中で両方を直す**（`internal/docs` のテストが、対になっていること・英語版に日本語が混ざっていないことを確かめる）
- **README は、先頭が中央寄せのブロック（画像・バッジ）なので、言語の切り替えは3行目でなく、先頭12行のどこかに置く**（英語版は `<a href="README_ja.md">日本語</a> | <b>English</b>`、日本語版は `English</a> | <b>日本語</b>`）。`internal/docs/readme_test.go` が、4つの README の対・切り替え・英語版の日本語の混入・画像とリンクの実在・目次の見出しを確かめる。**README には、本物と同じ形の範囲トークンを使う**（`tokens_test.go` は `docs/` だけを見る）
- 外部サービスのバッジと対話形式のガイドの URL は、テストで確かめない（通信が要る）。**shields.io の `visual-studio-marketplace` のバッジは廃止された（どの拡張でも「retired badge」と出る）ので、Marketplace のバッジは `flat.badgen.net/vs-marketplace/…` を使う**（vsce が受ける信頼済みの提供元）。評価のバッジは、評価が1件もないあいだ 500 を返すので、付けない（付けるのは評価がついてから）。README は拡張と一緒に公開されるので、公開後にバッジを足すと公開物とリポジトリが食い違う。**公開前の Marketplace のバッジは、見つからない表示のまま入れておく**（R12 の決定）
- `docs/examples/` の英語版は英語の出力、日本語版は日本語の入力（`why` など）で、サーバーが返す文言（MCP のエラーなど）は、どちらも英語。両方を `internal/docs` のテストが本物と照合する
- `dev/`・`.claude/rules/`・`CLAUDE.md`・mtqg は、開発のうちうちなので日本語のまま。人間とのやり取りも日本語
- コード中のコメントは英語（Go・TypeScript・Vim script・シェルとも）
- **利用者に見せる文言（CLI の出力・usage・エラー文・VSCode と Vim の画面）は、英語が既定で、日本語に切り替えられる**。VSCode と Vim で同じ文言にする。`docs/reference/vscode.md`・`vim.md`（文言の一覧は `handoff/design/DESIGN.md` 10章）に従う
  - Go：`internal/lang`（環境変数 `SRWR_LANG` が `ja` で始まれば日本語）の `Pick`・`Sprintf`。英語と日本語を、使う場所に並べて書く
  - VSCode：`vscode.env.language`（`src/lang.ts` の `pick`）。`package.json` の文言は `package.nls.json`（英語）と `package.nls.ja.json`
  - Vim：`$SRWR_LANG`（`autoload/srwr/lang.vim` の `Pick`）
  - **AI が読むもの（MCP のツールの説明・エラー文）、hook の注記、表示サーバーのエラー文は、英語だけ**。人に見せるエラー（テープが無い、など）は、クライアントがエラーコードから自分の言語の文を作る
  - 文言を足すときは、英語と日本語の両方を書く。テストは、英語（既定）と `SRWR_LANG=ja` の両方で確かめる
- **時刻**：テープは UTC（`…Z`）で持ち、人に見せるときにその機械の時間帯（`TZ`）に直す。古い `+09:00` のテープも読める
- 文は短く、1文1つのこと。結論を先に書く
- 用語は `.claude/rules/naming.md` の表に揃える
- 表と箇条書きを使ってよい。ただし、理由は文で書く
- **文書の相対リンク・`docs/…` のパス・`examples/` の応答は、`internal/docs` のテスト（`qsoku unit`）が確かめる。** 文書を動かしたら、リンクを直す
