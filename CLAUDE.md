# CLAUDE.md

このファイルは、このプロジェクトで作業する際、Claude Codeが常に踏まえるべき前提を示す。

## このプロジェクトについて

**srwr** は、AIエージェントに、理由（`why`）を添えて srwr の道具（`look` / `edit` / `new`）でファイルを編集させ、その操作を**テープ**に記録し、エディタ（VSCode と Vim）で**コマ送りで再生**する道具。`why`（理由）と選択範囲のコードを一緒に見せて、人間がAIの作業を理解できるようにする。

- 部品：Go の単一バイナリ `srwr`（`mcp`・`hook`・`view-server`・`view`・`init`・`tapes`・`trace`・`completion`）、VSCode 拡張 **srwr-view**（TypeScript）、Vim スクリプト **srwr-view.vim**（Vim9 script。バイナリに埋め込む）
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

## 守ること（設計の芯）

実装の細部より優先する。これを崩す変更は、実装する前に必ず相談すること。

- **外部依存ゼロを保つ。** トークン・テープ・MCP・表示サーバーは標準ライブラリで書く（`.claude/rules/go-code.md`）
- **先に実ファイルを書き、そのあとテープに追記する。** テープへの書き込みは、すべて `internal/session` を通す（`.claude/rules/go-code.md`・`tape.md`）
- **エディタはテープを直接読まない。** エディタとの境界は表示サーバーのプロトコル（`.claude/rules/tape.md`）
- **`docs/` が正本。** 確定した画面の仕様と、正解データ・画面の基準は変えない（上の「進め方の要点」、`.claude/rules/documentation.md`）

## ドキュメントの言語

- **`CLAUDE.md`・`.claude/rules/`・`dev/`・mtqg の記録は日本語のみ**
- **`docs/`・README は英語が基準**で、日本語版 `*_ja.md` と2本立て
- **コード・コメント・コミットメッセージは英語**。利用者に見せる文言は英語が既定で、日本語に切り替えられる
- 詳しい書き方は `.claude/rules/documentation.md`

## 参照すべきファイル

作業を始める前に、以下を確認すること。

- **`.claude/rules/bcuz-usage.md`** — bcuzの使い方。**Bashで実行するコマンドは、必ず`bcuz "理由" コマンド…`の形で打つ**（フックが、そうでないものを断る）。理由は「なぜ」を書く。**コマンドを打つ前に読む**
- **`.claude/rules/srwr-usage.md`** — srwrの使い方（**緩いモード**）。**ファイルの変更は`look`／`edit`／`new`（理由`why`つき）で行う。`Edit`・`Write`は禁止されていないが、srwrの道具が使えないときだけ使う。作業のまとまりごとに`session`で題つきのテープを始め、コミットの前にテープを閉じる。ファイルを変える前に読む**
- **`.claude/rules/mtqg-usage.md`** — mtqgの使い方（記録者、種類の使い分け、こまめに記録する）
- **`.claude/rules/qsoku-usage.md`** — qsokuの使い方（`qsokufile`の名前の一覧、呼び方、書式、`//`、シェル連携）。**ビルド・テスト・lintは`go`などを直接打たず、ここの名前で実行する**
- **`.claude/rules/crube-usage.md`** — crubeの使い方（MCPの`crube`・`crube-ja`とCLI、場所の探し方、英語と日本語の説明の保ち方）。**場所を探す前、ファイルを足す・動かす・消す前に読む**
- **`.claude/rules/testing.md`** — テスト方針（開発環境の前提、動作確認の粒度、期待値の決め方、壊して確かめる（Skill`mutation-check`）、Trivy、ShellCheck、`-race`、Bashサンドボックス・CIの落とし穴）
- **`.claude/rules/directory-structure.md`** — ディレクトリ構成と、ファイルの置き場所の判断基準。**新しいファイルを追加するときに読む**
- **`.claude/rules/datetime.md`** — 日時の扱い（データは UTC で持ち、表示だけ利用者のタイムゾーンに直す。瞬間で比べる、時計を差し替える）。**日時を保存・表示・比較するコードを書くときに読む**
- **`.claude/rules/working-with-human.md`** — 人間とのやり取り（頼み方の3種類、小さなつまずきの頼み方、確認の自動化、UIゲート）。**人間に何かを頼む前に読む**
- **`.claude/rules/naming.md`** — 命名規則（製品名、用語の日本語と英語の対応、JSON・Go・TypeScript・Vim script の名前）
- **`.claude/rules/distribution.md`** — 配布方法（goreleaserでの配布物の作り方とリリースの流れ、`go install`、cgo なし、バージョン、`.vsix`、Vim スクリプトの埋め込み）
- **`.claude/rules/documentation.md`** — ドキュメントの書き方（文書の役割、仕様と実装がずれたとき、言語、例の値は道具で求める、`docs/design/`に書く経緯の線引き）
- **`.claude/rules/go-code.md`** — Go の srwr 固有の決まり（外部依存ゼロ、書き込みの順序、ロック、記録しないファイル）
- **`.claude/rules/tape.md`** — テープ・範囲トークン・表示サーバーのプロトコルの互換性、正解データとの答え合わせ。**形式やプロトコルに触れるときに読む**
- **`.claude/rules/extension.md`** — VSCode 拡張の決まりと落とし穴。**`extension/` を触るときに読む**
- **`.claude/rules/vim.md`** — Vim クライアントの決まりと落とし穴（Vim 9.0.0784 以上、Vim9 script）。**`vim/` を触るときに読む**

これらのルールファイルは、実装中に得た細かい気づき・教訓をClaude Code自身が育てていくものである。新しく気づいたルール・踏んだ落とし穴があれば、該当するファイルに追記すること。どのファイルにも当てはまらない新しい種類の気づきであれば、新しいルールファイルを作ってよい（作ったらこの一覧にも足すこと）。

## 開発の進め方

- **Bashのコマンドは、すべて`bcuz "理由" コマンド…`で実行する**（`bcuz "lintの確認" qsoku lint`）。理由は「何をするか」でなく「なぜするか」。`bcuz`で始まらない・`&&`や`;`でつなぐコマンドは、`.claude/settings.json`の`PreToolUse`フック（`.claude/hooks/require-bcuz.sh`）が実行前に断る。**`git commit`だけは`bcuz`を通さない**（通すとコミットの後に記録が増えるため。理由はコミットメッセージに書く）。実行の記録は`.bcuz/history.jsonl`に残り（コミットする）、`bcuz -H`で見られる。**トークンなどの秘密を、コマンドの引数・`env`の指定・理由に書かない**（記録に残り、コミットされる）（`.claude/rules/bcuz-usage.md`）
- **ファイルの変更は、srwrの道具（`look`で見て`edit`、新規は`new`）で行い、すべてに理由（`why`）を添える。** このリポジトリは緩いモードで、組み込みの`Edit`・`Write`は禁止されていないが、srwrの道具が使えないとき（道具の不具合など）だけ使う。Bashでの書き換え（`sed -i`・リダイレクト）もしない
- **作業のまとまり（todo・bug・人間の依頼・作業の種類の変わり目）ごとに、最初に`session`で題つきのテープを始める。** 1つの作業＝1本のテープ、コミットも同じ区切り
- **コミットの前に、テープを閉じる**（`srwr tapes check`のあと`srwr tapes new`）。コミットまでの手順はSkill`commit-with-tape`。書き込み中の生のテープは`.gitignore`で対象外で、閉じて圧縮した`.srwr/tapes/*.tape.jsonl.gz`だけがgitに入る（`.claude/rules/srwr-usage.md`）
- **このリポジトリの中の場所は、まずcrubeで引く**（MCPの`find`・`tree`・`show`、または`crube find …`）。ファイル・ディレクトリを足す・動かす・消すときは、同じ変更の中で英語と日本語の説明を直し、コミットの前に両方の言語で`crube status --require all`を通す（`.claude/rules/crube-usage.md`）
- 実装後は`.claude/rules/testing.md`に従い、ビルド確認に加えて実際の動作確認（`bcuz "…" qsoku check`、必要に応じて`qsoku race`・`qsoku trivy`・`qsoku shellcheck`）を行うこと。判断に迷ったら実行する側に倒すこと
- 依存ライブラリは、標準ライブラリ → `golang.org/x` → 何かの公式のもの → デファクトの順で選ぶ（依存を足すこと自体が人間に聞く判断。`.claude/rules/working-with-human.md`）
- 依存ライブラリを足したら、Trivyでライセンス・脆弱性を確認すること（`qsoku trivy`）。srwr は外部依存ゼロが方針なので、足すこと自体が重要な判断（人間に聞く）
- 人間に確認を頼むときは、`.claude/rules/working-with-human.md` 3章の形で、1コマンドと見るところだけを渡すこと

## gitの扱い（Claude Codeの作業として）

- **pushは人間が行う。** Claude Codeはコミットまでで止めること。`git push`は`.claude/settings.json`で拒否（deny）されている
- 作業の区切りでこまめにコミットしてよい（消してしまったときに戻せるように）
- `git reset --hard`・`git clean`は、コミットしていない作業を消すので、使う前に必ず理由を説明すること
- **コミットの前に`git config user.email`／`user.name`を確かめる。** 正しい値は`65074207+amisonnet8@users.noreply.github.com`／`amisonnet8`（GitHubのプライバシー用noreplyアドレス）。違う値なら、`.git/config`の`[user]`（ローカルの上書き）を疑う。ローカルに設定が無いと、コンテナのglobalの設定（noreplyでないことがある）が使われる

## 権限・自動化について

`.claude/settings.json`（人間が管理する）により、次の設定になっている。

- 拒否（deny）：`git push`、`gh pr merge`、`gh release create`
- **`bcuz`を通さないBashコマンドの拒否**：`PreToolUse`フック（`.claude/hooks/require-bcuz.sh`）が、`bcuz`で始まらないコマンド（`git commit`を除く。逆に`bcuz`を通した`git commit`を断る）と、後ろに別のコマンドをつなぐもの（`&&`・`||`・`;`・`&`・改行・単一引用符の外のコマンド置換）を、実行前に断る。直し方は断りのメッセージと`.claude/rules/bcuz-usage.md`に書いてある
- 実行前に確認（ask）：`git reset --hard`、`git clean`、`sudo`、`gh pr create`
- **Bashコマンドは既定でOSレベルのサンドボックス（Linux bubblewrap）内で実行される。** ファイルシステムの書き込み先とネットワーク接続先は許可リストで絞られている。この範囲内なら`curl`・`wget`・`go get`などは確認なしに実行できる。リストに無い宛先・書き込み先は`sandbox_violations`として拒否される。そのときは、`.claude/rules/working-with-human.md` 2章の形（原因と、`.claude/settings.json` のどこに何を足すか）で**短く頼む**（許可を広げるかは人間が判断する）。落とし穴は`.claude/rules/testing.md`「Bashサンドボックスの落とし穴」参照

確認を求められた場合、無理に実行しようとしない。`.claude/settings.json`を変更したいときは、変更案を差分で示すにとどめること（人間が貼るだけで済む形にする）。

**srwrの緩いモード**（2026-10-16、人間の決定）：`.mcp.json`にsrwrのMCPサーバー、`PostToolUse`に`srwr hook`（読み取りと`Edit`・外部変更の記録）があり、`permissions.allow`でsrwrの道具を許可している。厳格モードの`Edit`・`Write`の禁止は**入れていない**（使っているsrwrに不具合があっても、編集の手段を失わないため）。道具として動くのは公開済みの`srwr`（`@latest`）で、開発中の`bin/srwr`ではない。テープ（`.srwr/tapes/`）はgitで共有する（鍵`.srwr/key`は入れない）。引数なしの`srwr init`（厳格モードにする）は動かさない。詳細は`.claude/rules/srwr-usage.md`。

ビルドの自動フック（`PostToolUse`）が設定されている。`Edit`・`Write`かsrwrの`edit`・`new`で、`.go`・`go.mod`・`go.sum`を変えると`.claude/hooks/build.sh`が`qsoku build`を、`extension/`の`.ts`・`package.json`・`tsconfig.json`を編集すると`qsoku ext-build`を実行し、失敗すると理由がClaude Codeに返る。複数ファイルにまたがる編集の途中は、まだ書いていないファイルを参照して一時的に失敗することがある。

**crubeの配線**：`.mcp.json`に、crubeのMCPサーバーが2つある（`crube`は英語、`crube-ja`は日本語。`crube mcp --allow-write`）。srwrの`new`のあとには`.claude/hooks/crube-untracked.sh`が、説明の無いパスを言語ごとに知らせる（止めはしない）。詳細は`.claude/rules/crube-usage.md`。

## mtqgでの記録について

作業の中で生まれる「気づき・やること・質問と答え・不具合とそのやり取り・用語の合意・この先も効き続ける決まり事」は、コードにもコミットにも残らず、チャットの履歴やメモアプリに散って失われやすい。`mtqg`（`m`emo & rules・`t`odo・`q`a & bugs・`g`lossary）はそれを`.mtqg/journal.jsonl`に追記し、gitに乗せて共有する。使い方は`.claude/rules/mtqg-usage.md`。`.mtqg/`には決まり事（rule）を17件登録済み——`mtqg context`で確認できる。AIエージェントからは、フック（`.claude/settings.json`の`mtqg hook`）とMCPサーバー（`.mcp.json`の`mtqg mcp`）の両方で使える。

## ルール・スキルの提案

作業を進める中で、以下に気づいたら、都度こちらから提案すること（提案するだけで、勝手に作成・適用はしない。判断はこちらが行う）。

- **新しいルールにした方がよさそうな知見**: 同じ種類の判断や落とし穴に複数回遭遇した、既存の`.claude/rules/`のどれにも当てはまらない、といった場合。どのファイルに追記すべきか、あるいは新規ファイルが必要かも合わせて提案する
- **Skill化した方が効率的そうな作業**: 「同じ手順を3回目繰り返している」など、パターン化できそうな一連の作業に気づいた場合。まだ実装が薄い段階では無理に提案する必要はない。パターンが実際に繰り返されてから提案すること

提案は気づいたタイミングで随時行ってよく、まとめて報告するために貯めておく必要はない。

This repository records its development with mtqg: run `mtqg context` at the start of a session, and see `.mtqg/SCHEMA.md` for the data format.
