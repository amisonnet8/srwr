# srwr-view

AI エージェントが `srwr` で行った操作（`select`・`replace`）を、**理由と一緒に、コマ送りで**見る VSCode 拡張です。

- 範囲の直前に、理由（`why`）を青（select）か橙（replace）の行で差し込みます
- srwr の外で起きた変更と、録画のあとのファイルの変更は、左右2つのエディタで差分を見せます
- ライブで、書かれていくテープを追えます。古いコマに戻ると「LIVE に戻る（新着 N）」が出ます
- 見るのは**コマ送りだけ**（自動再生はありません）。設定は `srwr.path` だけです
- テープも実ファイルも書き込みません

## 使い方

1. `srwr` を入れます：`go install github.com/amisonnet8/srwr/cmd/srwr@latest`
2. この拡張を入れます（`.vsix`）
3. 作業場を開き、左端の「srwr」→「操作一覧」から「テープを開く」か「ライブ視聴を開始」

`srwr` が PATH に無いときは、設定 `srwr.path` に場所を指定します。

## 対応する表示サーバー

`srwr view-server` の `protocolVersion` **1**。食い違うときは、更新を促す案内が出ます。
