# look / edit / replace / new のゲート（画面の名前）

画像は `images/`（dark・light）、まとめて見るなら `index.html`（ブラウザで開くだけ）。

## 1. この段階で作るもの
- AI のツールを `select`/`replace`/`sub`/`new` から **`look`/`edit`/`replace`/`new`** にする
  - `look`：見る（今の select）
  - `edit`：直す。look のトークンでも、`file`＋範囲＋`expect` でも呼べる（1手）
  - `replace`：同じ文字列を複数箇所で置き換える（今の sub。2か所以上のときだけ）
  - `new`：新しいファイルを作る（今のまま）
- テープ（`v:2`）・表示サーバーの約束（protocolVersion 2）・画面の名前を、この4つにそろえる。古いテープ（`v:1`）も読める

## 2. 画面で変わるところ
- 操作一覧の名前：`select`→`look`、`replace`→`edit`、`sub`→`replace`。`new` はそのまま
- 色・形・丸・差分の見せ方は今のまま（look は青、edit・replace・new は橙）
- Read・Bash などの hook の記録も `look` と出る
- 種類の ON/OFF は4つ：`look`／`edit`（edit・replace・new をまとめる）／`external`／`failure`
- 失敗のコマ：`✖ edit failed (content_not_found)` のように、ツール名が変わる
- Vim のキー（推奨）：**`tl`＝look、`te`＝edit、`tx`＝external、`tf`＝failure**。今の `te`（external）は `tx` に移る

## 3. 従う確定デザイン
今の画面（`extension/test/baseline/`・`vim/test/baseline/`）の色・並び・位置はそのまま。変わるのは名前の文字と Vim のキーだけ。

## 4. 新しく決めること（推奨が先頭）
1. 種類の ON/OFF の分け方：**4つ（look／edit／external／failure）（推奨）**／今の4つの名前を読み替えるだけ（select／replace／sub／new を別々に切る5つ）
2. Vim のキー：**`tl`・`te`・`tx`・`tf`（推奨）**／今のキー（`ts`・`tr`・`te`）のまま、意味だけ読み替える
3. 日本語の画面の名前：**ツール名と同じ英語（look・edit・replace・new）（推奨）**／日本語に訳す

## 5. 了承してほしいこと
画面の名前と ON/OFF の分け方、Vim のキーの変更。
