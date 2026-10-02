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
| `README.md` | 利用者向けの入口 | 使い方が変わったとき |
| `extension/README.md` | 拡張の利用者向けの説明（対応する表示サーバーのバージョンを含む） | 拡張の振る舞いが変わったとき |
| `CLAUDE.md`・`.claude/rules/` | 開発のルール | 気づき・落とし穴が見つかったとき（`CLAUDE.md` 参照） |

- `docs/` は、リポジトリに残る外部向けの文書。**`handoff/` や `dev/` を `docs/` から参照しない**（`handoff/` は最後に消す）
- 進捗・決定・質問は文書ではなく **mtqg** に記録する（`.claude/rules/mtqg-usage.md`）

## 仕様と実装がずれたとき

- **`docs/` が正本。** 作り直しなので、実装を `docs/` に合わせる
- `docs/` 自体に誤りや矛盾を見つけたら、黙って実装に合わせない。重要な判断として、推奨案つきで聞く（`.claude/rules/working-with-human.md` 1章）
- `docs/` と `handoff/`（前の実装から起こした資料）が食い違うときは、`handoff/design/`・`handoff/ui/`（評価者が確認した見た目と振る舞い）を正とし、`docs/` を直したうえで報告する
- 仕様を直すと決まったら、**同じ変更の中で**文書とコードを直す

## 書き方

- 文書は**日本語**で書く
- コード中のコメントは英語（Go・TypeScript・Vim script・シェルとも）
- UI の文言は日本語。VSCode と Vim で同じ文言にする。`docs/reference/vscode.md`・`vim.md`（文言の一覧は `handoff/design/DESIGN.md` 10章）に従う
- 文は短く、1文1つのこと。結論を先に書く
- 用語は `.claude/rules/naming.md` の表に揃える
- 表と箇条書きを使ってよい。ただし、理由は文で書く
- **文書の相対リンク・`docs/…` のパス・`examples/` の応答は、`internal/docs` のテスト（`qsoku unit`）が確かめる。** 文書を動かしたら、リンクを直す
