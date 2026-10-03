# baseline（Vim の画面の基準）

**`ja/`**（日本語の画面）は、前の実装で承認した画像から、一度だけ作った（作る道具は R14 で消した）。**`en/`**（英語の画面。既定）は、R10.5 のゲート（`dev/review/R10.5/`）で文言を承認し、`qsoku ui-check` の画面を `qsoku ui-accept` で取ったもの。`vim/screen_test.go` が、疑似端末で本物の Vim を動かして取った画面と比べる。

- 比べるのは、全部のセルの**文字と背景**と、srwr が決める色（理由の行・範囲・今のコマの行の上の文字、丸、ステータス行）の**前景**。Vim の構文の色は版で変わるので比べない
- **書き換えない。** 食い違えば実装を直す。直せない違いは、理由を書いて人間の了承をもらってから更新する
- `hl_dark.json`・`hl_light.json`：色の実測値（前の実装の実測値の写し）。`test_hl.vim` が比べる

## 画像そのままでない所（理由）

| 基準 | 何を直したか | 理由 |
|---|---|---|
| `live-basic` の 4〜6コマ目 | ステータス行の右側を、「（閉じる：…）」なしで描き直した | R5 のUIゲートの決定2。画像は、「（閉じる：…）」のせいで「L：LIVE に戻る」が切れている |
| `live-external` の 2コマ目（外部変更） | 一番下のステータス行を比べない（`skip`） | 同じ決定2。右のウィンドウは幅が狭く、画像は切れた古い表示 |
| 全部の一覧の行 | 丸を番号の前に移した（`● 番号 種類 ファイル:範囲`） | 人間の決定：VSCode の一覧と同じ並びにする。画像は「番号 丸 …」（`DotFirst`。最初の5セルを並べ替えるだけ） |
| `replay-no-why` の 3〜7コマ目 | 範囲が窓いっぱい（47行）に塗られている | 範囲が窓より長いコマ。画像は、塗り終わる前に取られたらしく、塗りが無い。範囲は塗る（`docs/reference/vim.md` 3） |

- 2026-10-03 `qsoku ui-accept`（版 d552e26）：replay-long-why dark、replay-long-why light、replay-long-why-narrow dark、vscode long_why を、人間が OK を付けた画面に差し替えた

- 2026-10-03 `qsoku ui-accept`（版 576b4c8-dirty）：replay-why-basic en dark、replay-why-basic en light、replay-external en dark、replay-external en light、replay-no-why en dark、replay-no-why en light、replay-long-why en dark、replay-long-why en light、replay-long-why-narrow en dark、live-basic en dark、live-basic en light、live-external en dark、live-external en light、vscode all_basic en、vscode all_ext en、vscode all_nowhy en、vscode long_why en、vscode live_basic en、vscode live_ext en、vscode long_why ja を、人間が OK を付けた画面に差し替えた
