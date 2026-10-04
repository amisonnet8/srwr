# sub ゲート：複数ファイルの文字列置換 `sub` の画面

画面の確定デザイン（`docs/reference/vscode.md`・`vim.md`）は、ここに書いた所だけ変える。ブラウザで `index.html` を開くと、画像（dark と light）と要点が1枚で見える。

## 作るもの
- 新しい MCP ツール **`sub`**：`files`・`old`・`new`・`count`・`why` を渡すと、文字列を置き換える。`count`（全部のファイルでの当たりの数）が合わなければ、何も変えない。文字列だけ（正規表現は無い）
- テープには、**当たった場所ごとに `replace` を1つ**、同じ `why` で書く。`replace` に `tool: "sub"` が付く
- 数が合わないときの失敗は、`failure`（`tool: "sub"`、コード `count_mismatch`）として書く

## 決めること（推奨案：案A）
1. **コマの見せ方**（人間の案 = 案C を推奨）
   - **案C（推奨）**：sub は**ファイルごとに1コマ**。external と同じ**左右2つの画面の差分**（左＝前・青、右＝後・橙、変わった行だけ塗る）。**ファイル名のタブと2つのエディタの間に、左右にまたがる理由の帯**（橙・白の太字）。左右の行が揃う。一覧は `sub  a.go (2 hits)` の1行。当たりが多くても、コマは増えない
   - 案A：当たりごとに、ふつうの replace のコマ。画面は変わらないが、当たりが多いとコマが多い
   - 案Cで変わるもの：コマに新しい種類 `sub`（差分のコマ＋理由）。プロトコル（`protocolVersion` を上げる）・拡張・Vim・基準。テープは、ファイルごとに `replace` を1つ（範囲＝最初の当たりから最後の当たりを含む行、`tool: "sub"`）で、形式のフィールドは足さない
2. **失敗の画面**：`✖ sub failed (count_mismatch)`。ほかは select・replace の失敗と同じ。メッセージに、見つけた数とファイルごとの数を言う
3. **`file`**：複数ファイルを渡す sub の失敗は、`file` を `(not shown)` にする（1つだけなら、そのファイル）

## 画像（`images/`、dark と light）
- `vscode-sub-diff-c`・`vscode-ops-c`：案C のコマ（左右の差分）と操作一覧
- `vscode-sub-frame-a`・`vscode-ops-a`：案A のコマと操作一覧
- `vscode-failure-frame`：数が合わなかった失敗
- `vim-sub-diff-c`・`vim-sub-frame`（案A）・`vim-failure-frame`：Vim のコマと失敗

## 了承してほしいこと
1. コマの見せ方（案C か 案A）
2. 失敗の画面（特に文言）
