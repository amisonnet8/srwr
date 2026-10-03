# VSCode 拡張（srwr-view）

## UI は確定している

- **見た目と振る舞いは確定している**（`docs/reference/vscode.md`と、基準 `extension/test/baseline/`）。色（select は青 `#0b61a4`、replace は橙 `#b45f06`、差分のコマの丸は紫）、理由の行、差分（左右2つのエディタ、前＝青・後＝橙で変わった行だけ）、コマ送りだけ、平らな操作一覧、下のバー（戻る・進む・位置）、ライブ（録画と同じ画面、「LIVE に戻る（新着 N）」）を、勝手に変えない
- **足さない。** 自動再生、ジャンプラベル、削除・挿入の線、親子の一覧、見た目の設定、実ファイルを開く機能は、評価者がやめると決めた
- 細かい値（色・文言・位置）は `docs/reference/vscode.md` と、今の実装・基準に合わせる
- 新しい画面が要るときだけ、UIゲート（`.claude/rules/working-with-human.md` 4章）
- Vim（`docs/reference/vim.md`）は同じ見え方を目指す。VSCode 側の見た目を、Vim に合わせて変えない

## 表示サーバーとの関係

- テープの読み込みとコマの列の組み立ては Go の表示サーバー（`srwr view-server`）がする（`docs/reference/protocol.md`）。拡張は描くだけ
- 表示の部品は `present`（色）・`replay`（録画。仮想ドキュメント、差分の左右2つのエディタ）・`live`（録画の画面を包んで、追いかけと新着を数える）・`controls`（下のバー）・`sidebar`（操作一覧）・`extension`。表示サーバーと話す部品は `server.ts`（`vscode` を import しない）
- 拡張が使うコマのフィールドは `index`・`kind`・`file`・`range`・`why`・`before`・`after`・`deleted` だけ（`docs/reference/protocol.md`）
- 拡張は `srwr` のバイナリを起動して話す。場所は設定 `srwr.path`（開発では環境変数 `SRWR_PATH`）。見つからないとき・バージョンが合わないときは、分かる文言で案内する

## 落とし穴（前のリポジトリで踏んだ）

- **行番号を出すか消すかは、コマごとに毎回、明示する。** 消した設定は、同じ場所で次に開くエディタに引き継がれる（差分の左の行番号が消えた）
- ライブで続けて届いたコマは、「見せると決めたコマ」を同期的に持って追いかける。画面の更新を待って決めると、追いかけが止まる
- 標準の diff 画面は、色を替えられないので使わない。左右2つのエディタに、自前の色を付ける
- テープごとに作業場を分けない。拡張は開いた作業場の `.srwr/tapes/` しか一覧に出さないので、取り違える。固定テープは `extension/test/fixtures/ui-check/` の1つの作業場に3本

- **コマの移動は1つずつ順に処理し、古い移動は飛ばす**（`ReplaySession.goto`）。移動のたびに世代番号を増やすだけだと、古い移動が、新しい移動の後始末のあとで右のエディタを開いてしまう。右のタブを閉じるかは、フラグでなく、毎回タブを調べて決める（R4 で見つけた。`view.test.ts` の「quick successive steps」）
- 画面の更新を待つテストは、`sleep` でなく、`activate` が返す `settled()`（ライブの描画が追いついたら解決する）か、`until(条件)` で待つ

## 言語と時刻（R10.5）

- 文言は英語が既定で、`vscode.env.language` が `ja` で始まれば日本語。`src/lang.ts` の `pick(en, ja)` を、**呼ぶときに**評価する（モジュールの定数に `pick` の結果を置くと、言語が決まる前に評価される。`sidebar.ts` の種類の名前で踏んだ）。`activate` の最初で `setJapanese` を呼ぶ
- `package.json` の文言は `%キー%` で、`package.nls.json`（英語）と `package.nls.ja.json`。`wiring.test.ts` が、キーの過不足を確かめる。`.vscodeignore` に `!package.nls*.json`
- 表示サーバーのエラー文は英語で開発者向け。人に見せるもの（`tape_not_found`・`tape_unreadable`）は `extension.ts` の `shownMessage` が自分の言語で言い直す
- テープ選びの時刻は `src/times.ts` の `localStamp`（その機械の時間帯）。テストの `setup.ts` は `TZ=Asia/Tokyo` に固定する（固定テープは `+09:00`）。時間帯の振る舞いは `english.test.ts` が `process.env.TZ` を替えて確かめる
- 偽の `vscode` の言語は `state.language`。`reset()` が環境変数 `SRWR_TEST_LANG`（既定 `en`）で決める。`view.test.ts`・`live.test.ts` は日本語の文言を確かめるので `ja` にし、英語は `english.test.ts`
- 画面の基準は `test/baseline/{ja,en}/`。`capture.test.ts` が両方を比べる
- **実物の VSCode で英語を見せるには、人が VSCode の表示言語を English にする**（Ctrl+Shift+P →「Configure Display Language」）。devcontainer の `code` は、人の VSCode に窓を開かせるだけで、`--locale en`・`--user-data-dir` は効かない（R10.5 の確認で、2回試して分かった）。確認ページの最初の手順に書く

## 構成

- TypeScript（`strict: true`）。ビルドは `tsc` のみ。バンドラーは使わない
- **ランタイム依存は持たない**（`dependencies` は空）。表示サーバーとの通信も、`child_process` と改行区切りの JSON で自前に書く
- `devDependencies` は `typescript`・`@types/vscode`・`@types/node` 程度
- 拡張はテープも実ファイルも書き込まない。鍵（`.srwr/key`）も読まない
- `tsconfig.json` の `rootDir` は `.`、`include` は `src` と `test`。出力は `out/src/`・`out/test/`。`package.json` の `main` は `./out/src/extension.js`
- `package.json` のコマンド・ビュー・設定と、コードが参照する名前は、`wiring.test.ts` で照合する

## テストと確認

- `qsoku ext-build`（コンパイルだけ）、`qsoku ext`（`bin/srwr` をビルドしてから、コンパイル＋テスト）。`extension/` の `.ts` を編集すると、`.claude/hooks/build.sh` が `qsoku ext-build` を自動で呼ぶ
- `npm test` は `node:test`。偽の `vscode`（`test/fakevscode.ts`）、偽のサーバー（`test/fakeserver.ts`。golden から答える）、本物の `bin/srwr` との通し（`test/server.test.ts`）
- **画面の取得**：偽の `vscode` の上で、全コマの見せる内容（文書・装飾・一覧・バー）を JSON に取り、基準と比べる（R4。`ja/` の最初の基準は前の実装が取った記録の写し）
- **偽の `vscode` の画面の取得**（`test/capture.ts`、基準は `test/baseline/`。`qsoku ui-check` は `node --require ./out/test/setup.js out/test/capture.js <出力先> [<追加の作業場>]` で取り、Go（`internal/uicheck/shots.go`）で比べる）：装飾の並びは、「最初に塗られた順」（前の実装の取り方と同じ）。本物の `bin/srwr` を動かすので、先に `qsoku bin`（`qsoku ext` は先に作る）。基準を更新するときは、理由を書いて人間の了承をもらう
- 人間に見てもらうのは、実物の VSCode でしか分からないことだけ。`qsoku ui-open vscode <テープ>` の1コマンドで開けるようにしてから頼む
- 拡張を F5 で起動する設定は、ルートの `.vscode/launch.json`。`outFiles` は `extension/out/src/**/*.js`
