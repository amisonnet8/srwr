# R4 UIゲート：VSCode 拡張 srwr-view

**新しい画面はありません。** 確定済みのデザイン（`handoff/design/`）を、そのまま作ります。見た目は変えない・足さない。

## 1. 作るもの
- 拡張 srwr-view：左のパネル「操作一覧」、エディタの表示（理由の行・範囲）、差分の左右2つ、下のバー、テープ選択、ライブ、失敗の案内
- 確認の自動化：全コマの画面の取得と基準との比較、`qsoku ui-open vscode <テープ>`、F5 用の `.vscode/launch.json`

## 2. 画面と従う画像（すべて確定済み。`images/` に写した）
| 画面 | 画像 |
|---|---|
| select（青の理由の行＋薄い青） | `replay-select_*` |
| replace（橙の理由の行＋薄い橙） | `replay-replace_*` |
| 理由なし（範囲の色だけ・標準の行番号） | `replay-no-why_*` |
| 外部変更（左右の差分） | `diff-external_*` |
| 録画のあとの変更（左右の差分） | `diff-final_*` |
| 操作一覧（紫の丸） | `panel_ops_external_and_final_*` |
| 下のバー | `statusbar_middle_*`、`statusbar_live_behind_new1_*` |
| ライブ（追う／遅れる） | `live-following_*`、`live-behind_*` |
| ウェルカム／テープ選択 | `welcome_no_tape_*`、`quickpick_open_tape_*` |

## 3. 新しく決めること（推奨案つき）
1. **`qsoku ui-open vscode <テープ>` の開き方**
   - 推奨：`.vsix` を作って入れ、一時の作業場（固定テープの写し）を新しいウィンドウで開く。人間は「テープを開く」を押すだけ
   - 代案：作業場を用意するだけにして、人間が F5 で起動する
2. **基準の置き場所**：`extension/test/baseline/`（最初は前の実装が取った記録の写し）
3. **`initialize` の `options`**：`{diffFrames:true}` だけを送る（ジャンプラベルは R3 で消した）

## 4. 了承してほしいこと
- 上の画面を、確定デザインどおりに作ること
- 3 の 1〜3
