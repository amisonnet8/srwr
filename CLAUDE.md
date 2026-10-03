# CLAUDE.md

このファイルは、このプロジェクトで作業する際、Claude Codeが常に踏まえるべき前提を示す。

## このプロジェクトについて

**srwr** は、AIエージェントに2つのコマンド（`select` / `replace`）だけでファイルを編集させ、その操作を**テープ**に記録し、エディタ（VSCode と Vim）で**コマ送りで再生**する道具。`why`（理由）と選択範囲のコードを一緒に見せて、人間がAIの作業を理解できるようにする。

- 部品：Go の単一バイナリ `srwr`（`mcp`・`hook`・`view-server`・`view`・`init`・`tapes`）、VSCode 拡張 **srwr-view**（TypeScript）、Vim スクリプト **srwr-view.vim**（Vim9 script。バイナリに埋め込む）
- **このリポジトリは作り直し。** 前のリポジトリのコードは持ち込まず、確定した文書・デザイン・正解データを手がかりに、一から作る（`dev/roadmap.md`）

### 最初に読むもの

1. **`dev/roadmap.md`** — 段階（R0〜R14）。**この順に進める**。今どこかは `mtqg context`
2. **`docs/README.md` → `docs/design/overview.md`** — 何を作るか。正本は `docs/reference/`
3. **`docs/reference/vscode.md`・`vim.md`** — 確定した画面の仕様（見た目と振る舞い）。**変えない・足さない**。承認した画面は基準（`extension/test/baseline/`・`vim/test/baseline/`）がテストで守る
4. **`.claude/rules/working-with-human.md`** — 人間とのやり取り（下の3原則の詳細）

### 人間とのやり取りの3原則（必ず守る）

1. **小さなつまずき**（サンドボックスで書けない、など）は、原因と「してほしいこと」を5行以内で頼む。人間は「やる／やらない」だけを判断する。「どうしますか」と丸投げしない。些細な判断は聞かずに決めて記録する
2. **人間がする確認**は、準備・起動・取得・比較を自動にし、人間は見て OK／NG を付けるだけにする。1コマンドで、何度でも同じ手順。確認を頼む前に、その自動化を作る
3. **UI に関わる段階の始め**は、`dev/review/<段階>/` に箇条書きのポイント資料とデザインのサンプル画像を作り、了承をもらってから実装する

### 進め方の要点

- **動くものを小さく作る。** 段階ごとに完了条件を実際に動かして確かめる。先回りして作らない
- **正解データに合わせる。** `extension/test/golden/`・`extension/test/fixtures/`・画面の基準（`extension/test/baseline/`・`vim/test/baseline/`）は書き換えない。食い違えば実装を直す
- 進捗は mtqg の todo。**最初のセッションで、`dev/roadmap.md` の段階を `mtqg t add` で立てる**（段階ごとに1件。中のタスクは着手前に足す）
- 仕様と実装がずれたら `.claude/rules/documentation.md`

## 参照すべきファイル

作業を始める前に、以下を確認すること。

- **`.claude/rules/mtqg-usage.md`** — mtqgの使い方（記録者、種類の使い分け、こまめに記録する）
- **`.claude/rules/qsoku-usage.md`** — qsokuの使い方（`qsokufile`の名前の一覧、呼び方、書式、`//`、シェル連携）。**ビルド・テスト・lintは`go`などを直接打たず、ここの名前で実行する**
- **`.claude/rules/testing.md`** — テスト方針（開発環境の前提、動作確認の粒度、Trivy、ShellCheck、`-race`、Bashサンドボックス・CIの落とし穴）
- **`.claude/rules/directory-structure.md`** — ディレクトリ構成と、ファイルの置き場所の判断基準。**新しいファイルを追加するときに読む**
- **`.claude/rules/working-with-human.md`** — 人間とのやり取り（頼み方の3種類、小さなつまずきの頼み方、確認の自動化、UIゲート）。**人間に何かを頼む前に読む**
- **`.claude/rules/naming.md`** — 命名規則（製品名、用語の日本語と英語の対応、JSON・Go・TypeScript・Vim script の名前）
- **`.claude/rules/distribution.md`** — 配布方法（`go install`、GitHub Releases、cgo なし、バージョン、`.vsix`、Vim スクリプトの埋め込み）
- **`.claude/rules/documentation.md`** — ドキュメントの書き方（文書の役割、仕様と実装がずれたとき、言語）
- **`.claude/rules/go-code.md`** — Go の srwr 固有の決まり（外部依存ゼロ、書き込みの順序、ロック、記録しないファイル）
- **`.claude/rules/tape.md`** — テープ・範囲トークン・表示サーバーのプロトコルの互換性、正解データとの答え合わせ。**形式やプロトコルに触れるときに読む**
- **`.claude/rules/extension.md`** — VSCode 拡張の決まりと落とし穴。**`extension/` を触るときに読む**
- **`.claude/rules/vim.md`** — Vim クライアントの決まりと落とし穴（Vim 9.0.0784 以上、Vim9 script）。**`vim/` を触るときに読む**

これらのルールファイルは、実装中に得た細かい気づき・教訓をClaude Code自身が育てていくものである。新しく気づいたルール・踏んだ落とし穴があれば、該当するファイルに追記すること。どのファイルにも当てはまらない新しい種類の気づきであれば、新しいルールファイルを作ってよい（作ったらこの一覧にも足すこと）。

## 開発の進め方

- 実装後は`.claude/rules/testing.md`に従い、ビルド確認に加えて実際の動作確認（`qsoku check`、必要に応じて`qsoku race`・`qsoku trivy`・`qsoku shellcheck`）を行うこと。判断に迷ったら実行する側に倒すこと
- 依存ライブラリを足したら、Trivyでライセンス・脆弱性を確認すること（`qsoku trivy`）
- 人間に確認を頼むときは、`.claude/rules/working-with-human.md` 3章の形で、1コマンドと見るところだけを渡すこと

## gitの扱い（Claude Codeの作業として）

- **pushは人間が行う。** Claude Codeはコミットまでで止めること。`git push`は`.claude/settings.json`で拒否（deny）されている
- 作業の区切りでこまめにコミットしてよい（消してしまったときに戻せるように）
- `git reset --hard`・`git clean`は、コミットしていない作業を消すので、使う前に必ず理由を説明すること

## 権限・自動化について

`.claude/settings.json`（人間が管理する）により、次の設定になっている。

- 拒否（deny）：`git push`、`gh pr merge`、`gh release create`
- 実行前に確認（ask）：`git reset --hard`、`git clean`、`sudo`、`gh pr create`
- srwr 自身が利用者の作業場に書く「Edit/Write の禁止」（`srwr init`）は、**この開発リポジトリには入れない**（入れると開発できなくなる）
- **Bashコマンドは既定でOSレベルのサンドボックス（Linux bubblewrap）内で実行される。** ファイルシステムの書き込み先とネットワーク接続先は許可リストで絞られている。この範囲内なら`curl`・`wget`・`go get`などは確認なしに実行できる。リストに無い宛先・書き込み先は`sandbox_violations`として拒否される。そのときは、`.claude/rules/working-with-human.md` 2章の形（原因と、`.claude/settings.json` のどこに何を足すか）で**短く頼む**（許可を広げるかは人間が判断する）。落とし穴は`.claude/rules/testing.md`「Bashサンドボックスの落とし穴」参照

確認を求められた場合、無理に実行しようとしない。`.claude/settings.json`を変更したいときは、変更案を差分で示すにとどめること（人間が貼るだけで済む形にする）。

ビルドの自動フック（`PostToolUse`）が設定されている。`.go`・`go.mod`・`go.sum`を編集すると`.claude/hooks/build.sh`が`qsoku build`を、`extension/`の`.ts`・`package.json`・`tsconfig.json`を編集すると`qsoku ext-build`を実行し、失敗すると理由がClaude Codeに返る。複数ファイルにまたがる編集の途中は、まだ書いていないファイルを参照して一時的に失敗することがある。

## mtqgでの記録について

作業の中で生まれる「気づき・やること・質問と答え・不具合とそのやり取り・用語の合意・この先も効き続ける決まり事」は、コードにもコミットにも残らず、チャットの履歴やメモアプリに散って失われやすい。`mtqg`（`m`emo & rules・`t`odo・`q`a & bugs・`g`lossary）はそれを`.mtqg/journal.jsonl`に追記し、gitに乗せて共有する。使い方は`.claude/rules/mtqg-usage.md`。`.mtqg/`にはこのテンプレートの時点で決まり事（rule）を3件登録済み——`mtqg context`で確認できる。AIエージェントからは、フック（`.claude/settings.json`の`mtqg hook`）とMCPサーバー（`.mcp.json`の`mtqg mcp`）の両方で使える。

## ルール・スキルの提案

作業を進める中で、以下に気づいたら、都度こちらから提案すること（提案するだけで、勝手に作成・適用はしない。判断はこちらが行う）。

- **新しいルールにした方がよさそうな知見**: 同じ種類の判断や落とし穴に複数回遭遇した、既存の`.claude/rules/`のどれにも当てはまらない、といった場合。どのファイルに追記すべきか、あるいは新規ファイルが必要かも合わせて提案する
- **Skill化した方が効率的そうな作業**: 「同じ手順を3回目繰り返している」など、パターン化できそうな一連の作業に気づいた場合。まだ実装が薄い段階では無理に提案する必要はない。パターンが実際に繰り返されてから提案すること

提案は気づいたタイミングで随時行ってよく、まとめて報告するために貯めておく必要はない。

This repository records its development with mtqg: run `mtqg context` at the start of a session, and see `.mtqg/SCHEMA.md` for the data format.
