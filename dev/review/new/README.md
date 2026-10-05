# new ゲート：why つきで新しいファイルを作る `new`

## ① 作るもの
- 新しい MCP ツール **new**（入力は `file`・`content`・`why`）。新しいファイルを、理由つきで作る
- 既にあるファイルにはエラー `file_exists`（何も変えない。`select` と `replace` を使わせる）
- 足りない親ディレクトリは作る。記録しないファイル（`.env` など）・作業場の外は作らない
- 新ツールの許可のため、既存の利用者は `srwr init` をもう一度実行する

## ② 画面の見え方
- new のコマ：**replace のコマと同じ1枚のエディタ**。ファイル全体が薄い橙、1行目の上に橙の理由の行
- 操作一覧：`new  config.go:1-5`（橙の丸）。表示の ON/OFF は replace と一緒（sub と同じ扱い）
- 失敗：`✖ new failed (file_exists)`（赤）。メッセージ・why・tool・file

## ③ 従う確定デザイン
`vscode-new-frame`・`vscode-ops`・`vscode-failure-frame`・`vim-new-frame`・`vim-failure-frame`（dark・light）。色と理由の行は、確定済みの replace と同じ。

## ④ 新しく決めること（推奨つき）
1. 新しいファイルのコマ：**1枚のエディタ（推奨）**／左が空の左右の差分（Bash で作った `created` の external と同じ）。推奨は前者：左が空で場所の無駄になる
2. 親ディレクトリ：**作る（推奨）**／作らずエラー
3. 返事：**`selection`・`startLine`・`endLine` だけ（推奨。中身は返さない）**。そのまま `replace` で続けて直せる

## ⑤ 了承してほしいこと
この見え方と、④の推奨で作ってよいか。
