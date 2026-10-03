# R9 UIゲート：`srwr init` と `srwr tapes` の出力

## ① この段階で作るもの
- `srwr init`：作業場を srwr 用に準備する（`.srwr/`・`.mcp.json`・`.claude/settings.json`・`.gitignore`）
- `srwr tapes`：テープの一覧・`new`・`prune`・`path`
- 画面（VSCode・Vim）は変わらない。変わるのは、端末に出る文言だけ

## ② 出力の見え方（画像）
- `init-empty`：空の作業場
- `init-existing`：すでに設定がある作業場（追記とバックアップ）
- `init-again`：2回目（何も変えない）
- `init-lenient`：緩いモードへの切り替え
- `init-broken`：設定が壊れているとき（何も変えずに止まる。終了コード 1）
- `tapes-list`：一覧（今のセッションに印）
- `tapes-ops`：`new`・`prune`・`path`

## ③ 従う確定デザイン
端末の出力は新しいので、確定済みの画像はない。色は VSCode／Vim と同じ系統（追記は青、作成は緑、変更なしは灰、失敗は赤）。

## ④ 新しく決めること（推奨案つき）
1. Claude Code が聞かずに使えるよう、`settings.json` に `enabledMcpjsonServers` と、`select`・`replace` の許可も足す（推奨）
2. `srwr init --lenient` / `srwr init` で、厳格 ⇄ 緩いを切り替える。srwr が足した禁止だけを付け外しする（推奨）
3. `.gitignore` に `.srwr/init-backup/` も足す。テープは無視しない（共有のため）（推奨）
4. `prune` は確認を聞かずに消し、消したテープを1行ずつ出す。今のセッションは消さない（推奨）

## ⑤ 了承してほしいこと
上の画像の文言と、④の4つ。
