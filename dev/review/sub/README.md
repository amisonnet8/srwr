# sub ゲート：複数ファイルの文字列置換 `sub` の画面

画面の確定デザイン（`docs/reference/vscode.md`・`vim.md`）は、ここに書いた所だけ変える。ブラウザで `index.html` を開くと、画像（dark と light）と要点が1枚で見える。

## 作るもの
- 新しい MCP ツール **`sub`**：`files`・`old`・`new`・`count`・`why` を渡すと、文字列を置き換える。`count`（全部のファイルでの当たりの数）が合わなければ、何も変えない。文字列だけ（正規表現は無い）
- テープには、**当たった場所ごとに `replace` を1つ**、同じ `why` で書く。`replace` に `tool: "sub"` が付く
- 数が合わないときの失敗は、`failure`（`tool: "sub"`、コード `count_mismatch`）として書く

## 決めること（推奨案：案A）
1. **コマの見せ方**
   - **案A（推奨）**：sub の1か所＝ふつうの replace のコマ。橙の理由の行、置き換えた行は薄い橙。一覧の種類も `replace`。同じ理由が続くので、一括だと読める。画面・プロトコル・拡張・Vim・基準を変えない
   - 案B：種類を `sub`、理由の行の頭に `[2/3]` を付ける。プロトコルにフィールドを足し、拡張・Vim・基準を直す（`protocolVersion` も上がる）
2. **失敗の画面**：`✖ sub failed (count_mismatch)`。ほかは select・replace の失敗と同じ。メッセージに、見つけた数とファイルごとの数を言う
3. **`file`**：複数ファイルを渡す sub の失敗は、`file` を `(not shown)` にする（1つだけなら、そのファイル）

## 画像（`images/`、dark と light）
- `vscode-sub-frame-a`・`vscode-sub-frame-b`：sub の2か所目のコマ（案A・案B）
- `vscode-ops-a`・`vscode-ops-b`：操作一覧（案A・案B）
- `vscode-failure-frame`：数が合わなかった失敗
- `vim-sub-frame`・`vim-failure-frame`：Vim の、案Aのコマと失敗

## 了承してほしいこと
1. コマの見せ方（案A か 案B）
2. 失敗の画面（特に文言）
