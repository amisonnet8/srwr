# R15 ゲート：コマの種類ごとの表示 ON/OFF と、failure のコマ

画面の確定デザイン（`docs/reference/vscode.md`・`vim.md`）は、ここに書いた所だけ変える。ブラウザで `index.html` を開くと、画像（dark と light）と要点が1枚で見える。

## 作るもの
- コマの種類 **select・replace・external・failure** を、表示 ON/OFF できる（VSCode・Vim）
- 失敗した呼び出し（213678face で記録済み）を **failure のコマ**（赤）として出せる
- デフォルトは select・replace・external が ON、**failure が OFF**

## 決めること（推奨案）
1. **サーバーが絞る**：`tape/open`・`live/start` に `kinds` を足す（省略は今と同じ）。絞った後の番号は 1 から振り直すので、一覧の番号 = 下のバーの位置が今のまま成り立つ。録画後の差分（final）は external に連れて ON/OFF
2. **状態は保存しない**：設定項目を足さない。VSCode は拡張が動いている間、Vim は起動している間だけ覚え、開き直すとデフォルトに戻る
3. **VSCode**：操作一覧の右上の漏斗ボタン → 4つにチェックが付いた選択肢（複数選択）。一覧の上に「Hiding: failure (2)」。コマンドパレットにも「srwr: Choose Frames to Show」
4. **Vim**：srwr のバッファの中だけ、`ts`・`tr`・`te`・`tf` で select・replace・external・failure を切り替え。`:SrwrToggle {select|replace|external|failure}`。ステータス行に「hidden: failure (2)」
5. **failure の色**：赤。理由の行の背景 `#c62828`（白の太字）、一覧の丸は VSCode `charts.red`・Vim dark `#f85149` / light `#d32f2f`。橙との見分けは、色に加えて記号 `✖` と文字 `failure`（日本語の画面は `失敗`）で付ける
6. **failure のコマの見せ方**：専用の説明の画面（赤い行に `✖ select failed (invalid_range)`、続けてエラー文、why、ツール、範囲）。失敗は開くファイルが無いことが多い（絶対パス・記録しないファイル・入力の誤りは file が null）ので、ファイルの中は見せない
7. **OFF の種類**：コマ送りにも一覧にも出ない。ライブの「新着 N」は、表示している種類だけを数える。全部 OFF のときは「表示するコマがありません」
8. **切り替えたときの位置**：今のコマに一番近いコマ（記録の順で）へ移る。ライブは、切り替えると最新を追う

## 画像（`images/`、dark と light）
- `vscode-ops-hidden`・`vscode-ops-shown`：操作一覧（failure を隠している／出している）
- `vscode-picker`：絞り込みの選択肢
- `vscode-failure-frame`：failure のコマの画面
- `vscode-statusbar-live`：ライブの下のバー（隠した種類は数えない）
- `vim-failure-frame`・`vim-hidden`：Vim の failure のコマと、隠しているときのステータス行

## 了承してほしいこと
1. 上の決めること8項目の推奨案
2. 画像の見た目（特に failure の赤、failure のコマの画面）
