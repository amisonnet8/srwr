# baseline（画面の基準）

偽の `vscode` の上で本物の拡張と本物の `srwr` を動かして取った、コマごとの画面（文書・装飾・一覧・バー）の基準。`capture.test.ts` が、今の取得結果と比べる。

- `ja/`（日本語の画面）の最初の基準は、前の実装が取った記録の写し。`en/`（英語の画面。既定）は、R10.5 のゲートで文言を承認し、`qsoku ui-accept` で取ったもの。**書き換えない。** 食い違えば実装を直す
- 違いが正しい変更によるものなら、理由を書き、人間の了承をもらってから更新する（`.claude/rules/working-with-human.md`）
- `all_*.json`：録画（`why-basic`・`external`・`no-why`）の全コマ。`live_*.json`：ライブの各段階

- 2026-10-03 `qsoku ui-accept`（版 d552e26）：replay-long-why dark、replay-long-why light、replay-long-why-narrow dark、vscode long_why を、人間が OK を付けた画面に差し替えた

- 2026-10-03 `qsoku ui-accept`（版 b7e5e90-dirty）：vscode long_why を、人間が OK を付けた画面に差し替えた

- 2026-10-03 `qsoku ui-accept`（版 576b4c8-dirty）：replay-why-basic en dark、replay-why-basic en light、replay-external en dark、replay-external en light、replay-no-why en dark、replay-no-why en light、replay-long-why en dark、replay-long-why en light、replay-long-why-narrow en dark、live-basic en dark、live-basic en light、live-external en dark、live-external en light、vscode all_basic en、vscode all_ext en、vscode all_nowhy en、vscode long_why en、vscode live_basic en、vscode live_ext en、vscode long_why ja を、人間が OK を付けた画面に差し替えた

- 2026-10-04 `qsoku ui-accept`（版 v0.1.1-11-g418b6a2-dirty）：replay-failure en dark、replay-failure en light、replay-failure ja dark、replay-failure ja light、vscode with_failure en、vscode with_failure ja を、人間が OK を付けた画面に差し替えた
