# sub ゲート：複数ファイルの文字列置換 `sub` の画面

画面の確定デザイン（`docs/reference/vscode.md`・`vim.md`）は、ここに書いた所だけ変える。ブラウザで `index.html` を開くと、画像（dark と light）が見える。

## 作るもの
- 新しい MCP ツール **`sub`**：文字列を、複数ファイルで置き換える。当たりの数が合わなければ、何も変えない
- 画面：**ファイルごとに1コマ**。external と同じ左右2つの差分（左＝前・青、右＝後・橙、変わった行だけ塗る）
- 理由（`why`）は、タブとエディタの間の**橙の帯**。VSCode は、右の仮想ドキュメントの先頭に理由の行、左の先頭に同じ色の空の行を入れて、行を揃える（文字は右の中央）。Vim は、2つの窓の上に帯
- 一覧は `sub  a.go (2 hits)` の1行（橙の丸）
- 数が合わない失敗は `✖ sub failed (count_mismatch)`（赤）

## 変わるもの
コマに新しい種類 `sub`（差分のコマ＋理由）。プロトコル（`protocolVersion` を上げる）・拡張・Vim・基準。テープは、ファイルごとに `replace` を1つ（`tool: "sub"`）で、形式のフィールドは足さない。

## 画像（`images/`、dark と light）
`vscode-sub-diff`・`vscode-ops`・`vscode-failure-frame`・`vim-sub-diff`・`vim-failure-frame`

## 了承してほしいこと
この見え方でよいか（OK か、直すところ）
