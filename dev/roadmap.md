# 開発の段階（作り直し）

**読者**：srwr を開発する AI と人間。

srwr は、前のリポジトリで一度作った（段階 P0〜P5 と、UI の作り直し P10 まで）。このリポジトリでは、**コードを持ち込まずに一から作り直す。** 持ち込んだのは、確定した文書と正解データだけ。

| 持ち込んだもの | 場所 | 扱い |
|---|---|---|
| 仕様（利用者・開発者向け） | `docs/` | 正本。作るものはここに書いてある |
| 確定したデザイン、UI の振る舞い、確認の方針、前の決まりと落とし穴 | `handoff/`（git に入れない） | 読むだけ。デザインは**変えない・足さない**（`handoff/README.md`） |
| 固定テープ・テープの読み取りの正解 | `extension/test/fixtures/` | 書き換えない |
| コマの列の正解（22本） | `extension/test/golden/` | 書き換えない。食い違えば実装を直す |

## 進め方

- **この順に進める。** 段階ごとに、完了条件を**実際に動かして**確かめてから次へ
- 段階を始めるとき、`mtqg t add` で todo を立てる。中のタスクも、着手する前に立てる
- 【UIゲート】の段階は、**実装の前に**、ポイント資料とサンプル画像で了承をもらう（`.claude/rules/working-with-human.md` 4章）
- 「人間の確認」は、**1コマンドで済む形にしてから**頼む（同 3章）。その自動化を作ることも、その段階の作業に含む
- 迷ったら、小さく作って動かす。先回りして作らない

## 前のリポジトリとの対応

| 今 | 前 | 中身 |
|---|---|---|
| R0 | — | 土台 |
| R1〜R3 | P0〜P2・P5 | テープ・範囲トークン・`srwr mcp`・セッション・表示サーバー |
| R4 | P3・P10 | VSCode クライアント |
| R5 | P4・P10 | Vim クライアント |
| R6 | （新規） | UI の確認の一本化（`qsoku ui-check`） |
| R7〜R11 | P6〜P9・P11 | hook、記録しないファイル、`init`・`tapes`、`vcs`、配布 |
| R12 | — | 片付け |

---

## R0：土台

- [ ] devcontainer で `qsoku check` が通る（Go はプレースホルダーだけ。拡張・Vim は「skipped」）
- [ ] `internal/docs`：`docs/` の文書の相対リンクと、`docs/…` のパスが切れていないことを確かめるテスト。これを足したら `internal/doc.go` を消す
- [ ] 最初のコミット（push は人間）

**完了条件**：`qsoku check` が通る。CI が通る（push のあと）。
**人間の確認**：なし。

## R1：テープと範囲トークン

参照：`docs/reference/tape.md`・`docs/design/token.md`・`.claude/rules/tape.md`

- [ ] `internal/tape`：イベントの型、追記、読み込み（古い形式・壊れた行・改行で終わらない最後の行の保留を含む）、テープから作る状態（ファイルごとの内容、最後の `seq`、`replace` の列）
- [ ] `internal/token`：LEB128、Crockford Base32（読み替えの許容）、HMAC（テープID 全体を含む）

**完了条件**：`extension/test/fixtures/` のテープ4本（`basic`・`external-text`・`phase0-bash`・`real-playground`）を読んだ結果が、`*.expected.json`（最後のファイルの内容と、コマの種類の並び）と一致する。トークンの往復・改ざんの検出・読み替えのテストが通る。
**人間の確認**：なし。

## R2：`srwr mcp`（select / replace）とセッション

参照：`docs/reference/mcp.md`・`docs/reference/tape.md`（セッション）・`docs/examples/select-replace.md`・`.claude/rules/go-code.md`

- [ ] `internal/jsonrpc`（改行区切り。`mcp` と表示サーバーで共有する）
- [ ] `internal/core`：範囲の検証、行番号の補正、内容の照合、`external` の検知
- [ ] `internal/session`：`.srwr/active`、セッションの区切り（30分）、`.srwr/lock`（flock。OS ごとにビルドタグ）、テープからの読み足し、鍵の作成（書きかけを読ませない）
- [ ] `internal/tools`・`internal/mcp`・`internal/cli`（`mcp`）・`cmd/srwr`
- [ ] `internal/docs` に、`docs/examples/select-replace.md` の応答が本物と一致することの確認を足す

**完了条件**：①`srwr mcp` を2つ起動し、片方のトークンをもう片方の `replace` で使える、②間が空くと新しいセッションになる（テストでは短く）、③別のセッションのトークンは `invalid_selection`、④2つ同時に起動しても鍵の作成で失敗しない。`qsoku race` と `qsoku cross` が通る。
**人間の確認**：なし。

## R3：表示サーバー（`srwr view-server`）

参照：`docs/reference/protocol.md`・`docs/examples/protocol-session.md`・`handoff/ui/UI.md`（2〜4章）

- [ ] `internal/timeline`：コマの列、各コマの時点の文書、差分のコマ、最後の差分
- [ ] `internal/viewserver`：メソッド、エラー、ライブ（テープの大きさのポーリング、200ミリ秒）、通知は返事のあとに始める
- [ ] 安全：テープに書かれたパスを信用しない（作業場の外・シンボリックリンクは「存在しない」）。`tapeId` は裸の名前だけ
- [ ] `internal/docs` に、`docs/examples/protocol-session.md` の確認を足す

**完了条件**：`extension/test/golden/` の22本すべてで、コマの列が一致する。要求と応答、ライブの通知、失敗のテストが通る。
**人間の確認**：なし。

## R4：VSCode クライアント（srwr-view）【UIゲート】

参照：`docs/reference/vscode.md`・`handoff/design/`（`DESIGN.md`・`images/vscode/`・`CODE.md`）・`handoff/ui/`（`UI.md`・`CODE.md`）・`.claude/rules/extension.md`

- [ ] **ゲート**：`dev/review/R4/`（作る画面の一覧と、`handoff/design/images/vscode/` の該当の画像）で了承をもらう
- [ ] `extension/`：`server.ts`（表示サーバーとの通信）と、表示の部品（`present`・`replay`・`live`・`controls`・`sidebar`・`extension`）
- [ ] テスト：偽の `vscode`、偽のサーバー（golden から答える）、本物の `bin/srwr` との通し（`server.test.ts`）、`wiring.test.ts`
- [ ] **画面の取得**：偽の `vscode` の上で全コマの見せる内容（文書・装飾・一覧・バー）を JSON に取り、`handoff/checklist/captured/`（前の実装の記録）と比べる。最初の基準として取り込む
- [ ] **`qsoku ui-open vscode <テープ>`**：固定テープの作業場を一時ディレクトリに作り、拡張を入れた VSCode で開くまでを1コマンドに。`.vscode/launch.json`（F5）も置く

**完了条件**：`qsoku ext` が通る。取得した全コマが基準と一致する（違いは理由を書いて、了承をもらってから基準を更新）。
**人間の確認**：`qsoku ui-open vscode why-basic` などで、`handoff/checklist/CHECKLIST.md` 4-2 の 1〜4 を見る。

## R5：Vim クライアント（srwr-view.vim）【UIゲート】

参照：`docs/reference/vim.md`・`handoff/design/images/vim/`・`handoff/ui/UI.md`・`.claude/rules/vim.md`

- [ ] **ゲート**：`dev/review/R5/`。確定した画面に加え、持ち越しの課題（下の表）の直し方を、推奨案つきの静止画で決めてもらう
- [ ] `vim/`（Vim9 script）、`vim/embed.go`、`srwr view`（埋め込み、キャッシュへの書き出し、機能の確認。最も古い Vim は 9.0.0784）
- [ ] テスト：画面なしの Vim。持ち越しの不具合2件を回帰テストにする
- [ ] **画面の取得**：疑似端末で本物の Vim を動かし、コマごとの画面を `term_scrape()` で取る。色は `handoff/checklist/captured/hl_*.json` と比べる。画面の基準は、ゲートで了承した画像から作る
- [ ] **`qsoku ui-open vim <テープ>`**

**完了条件**：`qsoku vim-test` が通る（最新の Vim と 9.0.0784）。取得した画面が基準と一致する。
**人間の確認**：`qsoku ui-open vim why-basic` などで、4-2 の 1〜4 を見る。

## R6：UI の確認の一本化【UIゲート（確認ページ）】

参照：`handoff/checklist/CHECKLIST.md`（2〜4章）

- [x] **ゲート**：確認ページ（基準と今を左右に並べる HTML）の見た目の案。簡素に
- [x] `qsoku ui-check`：ビルド → 作業場 → その場で作るテープ（長い理由など。本物の `srwr mcp` で）→ 両方の画面の取得 → 自動の検証（CHECKLIST 2章の「自動」の列すべて）→ 基準との比較 → 確認ページ → 結果の保存。途中で止まらない
- [x] `qsoku ui-live`（`--watch` で、人間が流し込みを見ながら体感できる）
- [x] 確認の仕組みそのもののテスト（わざと UI を壊して、落ちることを確かめる）
- [x] 実物の VSCode の画面を自動で取れるか調べて決める（CHECKLIST 3-4。`mtqg q add`）

**完了条件**：変更が無いとき「違いなし」と出る。わざと壊すと、どの項目か分かる文言で落ちる。
**人間の確認**：`qsoku ui-check` を一度動かし、確認ページを見る。

## R7：hook を同じテープに

参照：`docs/reference/cli.md`（`srwr hook`）・`docs/reference/tape.md`（`source`・`tool`）

- [x] `srwr hook`：Read・Bash・Grep・Edit・Write の記録。`mcp` と同じセッションに書く
- [x] 調べる過程のコマが多いときの見せ方を変える必要が出たら【UIゲート】（`docs/design/limitations.md` の未定）

**完了条件**：一時的な作業場で Claude Code に作業させ、調べる過程と `select` / `replace` が1本のテープに並ぶ。`qsoku ui-check` が通る。
**人間の確認**：`qsoku ui-check` の確認ページ（違いがあるときだけ）。

## R8：記録しないファイル

参照：`docs/reference/cli.md`（記録しないファイル）

- [x] 既定の対象、`.srwrignore`、`ignored_file`。`select`・`replace`・hook・external の検知の4つの入口すべて

**完了条件**：4つの入口それぞれにテストがあり、`.env` がテープに残らない。
**人間の確認**：なし。

## R9：`srwr init` と `srwr tapes`【UIゲート（出力の例）】

参照：`docs/reference/cli.md`・`docs/reference/settings.md`

- [ ] **ゲート**：利用者が見る出力（書き換えた内容の案内、一覧の見え方）の例を、端末の画像で
- [ ] `init`（厳格・緩い、既存ファイルを壊さない追記、バックアップ）、`tapes`（一覧・new・prune・path）

**完了条件**：空の作業場と、既存の `.claude/settings.json`・`.mcp.json` がある作業場の両方で `srwr init` し、Claude Code がそのまま srwr を使える。
**人間の確認**：一時的な作業場を作って `srwr init` を見せるところまでを1コマンドに（`qsoku try-init` など）。

## R10：header の `vcs`

- [ ] git の管理下なら HEAD と未コミットの有無を記録

**完了条件**：git の作業場と、そうでない作業場の両方でテストが通る。
**人間の確認**：なし。

## R10.5：言語・時間対応【UIゲート】

R10 までは、確認者が日本語の方が得意なので、日本語で作る。ここで英語を既定にする（人間の決定）。

- [ ] **ゲート**：英語にした画面・出力の見え方（VSCode・Vim の文言、CLI の出力）
- [ ] 利用者向けの表示（CLI の出力・usage・エラー文・VSCode と Vim の画面の文言）を英語既定にし、日本語に切り替えられるようにする（切り替えの仕組みから決める。`SRWR_LANG=ja` など）
- [ ] **時間**：テープの時刻（`header.startedAt`・各イベントの `ts`）は標準時（UTC、`Z` つき）で書く。古いテープ（`+09:00` など）も読める。表示（一覧・操作一覧・`srwr tapes` など）は、タイムゾーンの設定に合わせる（既定はその機械の時間帯。`TZ` に従う）。書式は言語に合わせる
- [ ] テスト・画面の基準（`vim/test/baseline`・`extension/test/baseline`）・`docs/examples/` を英語に合わせる
- [ ] `docs/` を英語既定にし、日本語版を `<名前>_ja.md` にする。`.claude/rules/documentation.md`・`naming.md` を直す

**完了条件**：テープの時刻が UTC で書かれ、`TZ` を替えると表示の時刻が替わる。何も設定しない環境で表示がすべて英語、日本語に切り替えると今までの日本語と同じ。`qsoku check`・`qsoku ui-check` が通る。
**人間の確認**：英語の画面を1コマンドで見せる（`qsoku ui-check`）。

## R11：配布

参照：`.claude/rules/distribution.md`

- [ ] `srwr` のビルド（Linux・macOS・Windows）
- [ ] 拡張の `.vsix`。バイナリを同梱するかを決める（重要な判断。推奨案つきで聞く）
- [ ] `README.md`（導入、VSCode と Vim での見方、共有前の注意）

**完了条件**：手元で作った `.vsix` と `srwr` で、固定テープを VSCode と Vim の両方で開ける。
**人間の確認**：入れて開くまでを1コマンドに。

## R12：片付け

- [ ] `handoff/` の中で、まだリポジトリに取り込んでいない決まり・落とし穴が無いか確かめる
- [ ] `handoff/` を消す（了承をもらってから）
- [ ] `docs/` の最終確認（`internal/docs` のテスト）

---

## 前のリポジトリから持ち越す課題

| 内容 | 段階 | 元の記録 |
|---|---|---|
| Vim：範囲が画面の下端に近いと、理由の行が最終行に出て、範囲が画面の外に隠れる | R5（回帰テストにして直す） | mtqg `80e87f836a` |
| Vim のライブ：遅れているときのステータス行で、「LIVE に戻る（新着 N）」の左が切れる | R5（同上） | mtqg `6aca556b6e` |
| Vim で理由の行に行番号が付く件を、自前の番号（行頭の仮想テキスト）にするか（未回答） | R5 のゲートで決める | mtqg `104dc8c6d7` |
| 2つの `srwr mcp` が同時に鍵を作ると、書きかけを読んで起動に失敗した（前は直した） | R2 のテストで最初から押さえる | mtqg `e0168981fb` |
| Windows で `syscall.Flock` がビルドできない（前は CI で初めて見つかった） | R2 から `qsoku cross` で押さえる | `.claude/rules/testing.md` |

前の記録の全体は `handoff/reference/project-state/`（`mtqg-context.md`・`mtqg-journal.jsonl`）。このリポジトリの mtqg は新しく始める。
