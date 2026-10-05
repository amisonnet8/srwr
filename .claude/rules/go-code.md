# Go のコード（srwr 固有の決まり）

一般的な lint・整形・テストは `.claude/rules/testing.md` と `.golangci.yaml` に任せる。ここには srwr 固有の決まりだけを書く。前のリポジトリで踏んだ落とし穴を含む。

## 言語と時刻（R10.5）

- 利用者に見せる文言は `internal/lang`（環境変数 `SRWR_LANG` が `ja` で始まれば日本語。呼ぶたびに読む）の `Pick(en, ja)`・`Sprintf(en, ja, …)` で、**使う場所に英語と日本語を並べて**書く。文言を足すときは、両方を書く
- **AI が読むもの**（`internal/tools` のツールの説明、`internal/core`・`internal/mcp` のエラー文）、**hook の注記**（`internal/hook`・`core/hook.go`）、**表示サーバーのエラー文**（`internal/viewserver`）は、英語だけ（`lang` を使わない）
- テスト：`internal/cli`・`internal/setup` は `TestMain` で `SRWR_LANG=ja` にして（今までの日本語の文言を確かめる）、英語は `lang_test.go`・`english_test.go` が `t.Setenv("SRWR_LANG", "")` で確かめる
- **時刻**：`tape.FormatTS` が UTC で書く。テープIDの日時（`session.newID`）も UTC。読む側は `time.Parse(time.RFC3339, …)` で両方（`Z`・オフセット）を読む。人に見せるのは `Local()`（`TZ` に従う）。テストは `time.Local` を替えて確かめる（`internal/cli/lang_test.go`）
- テープの一覧の並びは、ID でなく**始めた時刻**（`internal/cli/tapes.go`・`internal/viewserver/tapes.go`）

## 依存

- **外部依存ゼロを保つ。** 範囲トークン（varint・Crockford Base32・HMAC）、テープ（JSONL）、ハッシュ、MCP と表示サーバー（JSON-RPC over stdio）はすべて標準ライブラリで書く
- MCP は公式 SDK を使わず、自前で書く（`initialize`・`ping`・`tools/list`・`tools/call` だけ）
- 改行区切りの JSON-RPC の読み書きは `internal/jsonrpc` に1つだけ置き、`mcp` と `viewserver` が共有する
- ファイルの見張り（ライブ）も外部依存を足さず、テープの大きさのポーリングで行う
- 依存を足すのは重要な判断（`.claude/rules/working-with-human.md`）。足したら `qsoku trivy` を通す

## ファイルとテープへの書き込み

- 実ファイルの書き換えは、**同じディレクトリの一時ファイルに書いて rename** する（権限を保つ）
- **先に実ファイルを書き、そのあとテープに追記する**（`.claude/rules/tape.md`）
- テープへの書き込みは、`internal/session` を通す。ロック（`.srwr/lock`）を取り、今のセッションを決め（`.srwr/active`、最後のイベントから30分空いたら新しいセッション）、前回読んだ位置から後ろのテープを読み足してから、処理を呼ぶ。`mcp` も `hook` も同じ道を通る
- 処理の中で、プロセスのメモリに状態を持たない。状態（最後の seq、ファイルの内容、replace の列）は、テープから読み足したものだけから取る（別のプロセスが書いた分が抜けるため）
- **ファイルを共有して作るもの（鍵 `.srwr/key`、`.srwr/active`）は、一時ファイルに全部書いてから置く。** 前のリポジトリで、2つの `srwr mcp` が同時に起動して、書きかけの鍵を読んで起動に失敗した。鍵は `link`（先に置いた方が勝つ）、`active` は `rename`
- トークンの HMAC に入れるのは、テープID 全体。短いID ではない
- **同じプロセスの goroutine 同士は、flock のほかに `sync.Mutex` で守る。** race detector は flock による順序を知らないので、`Workspace` が持つ読み足しの状態を2つの goroutine が順に触ると、flock だけでは race と報告される
- Windows の flock は、標準ライブラリに無いので `syscall.NewLazyDLL("kernel32.dll")` の `LockFileEx`・`UnlockFileEx` を呼ぶ（外部依存も cgo も要らない）。手元では型検査（`qsoku cross`）までで、動くのは CI の Windows が初めて
- flock は OS ごとにビルドタグで分け（`//go:build unix` と `//go:build windows`）、cgo を使わない。**手元でも `qsoku cross` を通す**（前は Windows の CI で初めて落ちた）
- **閉じたセッションのテープは gzip で圧縮される**（`internal/tape/store.go`）。起点は `session` の `current()`（間隔を越えた古いテープ）と `EndSession()`（`srwr tapes new`）で、どちらもロックの中。**圧縮の失敗は作業を止めない**（`current()` は握りつぶし、`EndSession` は `*CompressError` で返して CLI が警告にする）。圧縮したテープに追記しない（`tapeCache.closed`）。`Compress` は、読み終えて閉じてから rename・remove する（Windows は開いたファイルを置き換えられない）
- 表示サーバー（`viewserver`・`timeline`）はテープを**読むだけ**。ロックを取らず、書き込み途中の最後の行は保留する
- 表示サーバーが「今のファイル」を読むのは、最後の差分のときだけ。**テープに書かれたパスは信用しない**（共有されたテープが任意のファイルを読ませないよう、作業場の外を指すパス・シンボリックリンクは「存在しない」として扱う）。`tapeId` も裸の名前だけを受ける
- サーバーからの通知（ライブ）を書く goroutine は、接続が終わるときに必ず止めて待つ。通知は、その要求への返事を書いたあとに始める
  - 「返事のあとに始める」は、結果に `After()` を持たせて `jsonrpc.Serve` に任せる（`jsonrpc.Afterward`）。ハンドラーの中で始めると、返事の組み立てが長いときに通知が先に出る。**テスト**は、長いテープ（2万コマ）で返事の組み立てを遅くし、書き込みを続けながら開始する。短いテープだと、壊しても落ちない

## hook（`srwr hook`）

- **AI の作業を止めない。** 失敗しても標準エラー出力に出して**終了コード 0**。終了コード 2 は Claude Code が道具の実行を止める。コマンドラインの誤りだけが 1（止めない失敗）
- 記録は `internal/core` の `Hook` に集める（`mcp` と同じ `Workspace.Do` の中。ロック・セッション・テープの読み足しが同じ）。`internal/hook` は JSON と Bash の読み取りを読んで `core.HookRequest` を作るだけ
- パスは `toRel` で作業場からの相対にし、`core.cleanPath`・`readTarget` を通す（外を指すパス・リンクは記録しない）。記録しないファイル（R8）の判定は `core.readTarget` の1か所（hook・`observeAll` も `look`・`edit`・`replace`・`new` もここを通る）
- **Edit は、ファイルを先に書いた後で呼ばれる（`PostToolUse`）。** 編集前の内容は、テープが持つもの、なければ `tool_response.originalFile`。`old_string` → `new_string` を当てて今のファイルと一致しなければ、`replace` を作らず `external` にする
- **Bash の後は、新しいファイルも探す**（`core.observeNew`）。`internal/vcs` の `NewFiles`（git の未追跡と、インデックスに追加したばかり＝`git add`・`git add -N` のファイル）から得て、**必ず `readTarget` を通す**（記録しない設定・バイナリ・CRLF・外を指すリンクを飛ばす）。1回に50件・1件256 KiB まで。git が使えなくても黙って何もしない。`created: true` の `external` として書く
- Bash の読み取りは、**先頭のコマンドだけ**を見る。`$( )`・書き込みのリダイレクト・`sed -i`・`tail -f` などは読み取りとして扱わない。Bash のあとは、読み取りでなくても `ObserveAll`
- 実際の Claude Code の形：Read・Edit・Bash の `tool_response` は過去の記録（`~/.claude/projects/*.jsonl` の `toolUseResult`）で確かめた。**Grep の形は確かめていない**ので、複数の書式を受け、実機で確かめる

## header の vcs

- `internal/vcs` の `Detect` が、テープを作るとき（`session` の `createTape`）に1回だけ、`git` を読み取りで動かす。`-c core.fsmonitor=false --no-optional-locks`、`GIT_OPTIONAL_LOCKS=0`、`LC_ALL=C`、`GIT_TERMINAL_PROMPT=0`、5秒の時間切れ。**失敗は黙って `null`**（AI の作業を止めない）
- `dirty` は `.srwr/` を数えない（`:(exclude).srwr`）。テストは `session.Options.VCS` で差し替えられる
- git が使えない環境でも、テストは `t.Skip` でなく、`null` になることを確かめる

## init・tapes（`srwr init`・`srwr tapes`）

- `internal/setup` は、ファイルを**全部読んで計画を立ててから**書く。JSON として読めない・形が違うファイルが1つでもあれば `*UserError` で止まり、何も書かない（鍵も作らない）
- JSON は `internal/setup/ordered.go` の順序を保つ値で読み書きする（`map` に落とさない。キーの順と数値の書式を保つ）。変えるものがなければ書き換えない（バックアップも作らない）
- 厳格 ⇄ 緩いの切り替えで外す禁止は、`Edit`・`Write`・`MultiEdit`・`NotebookEdit` の4つだけ
- `srwr tapes new`・`prune` はロック（`Workspace.Do`）の中で動かす。`prune` は今のセッションのテープを消さない。`srwr tapes`（一覧）と `path` はロックも `.srwr/` の作成もしない
- `srwr tapes check` は読むだけ（ロックなし、`.srwr/` を作らない）。`git` を動かすのは `internal/vcs` の `Changes` に集める（`Detect` と違い、失敗の理由を返す。人間が頼んだ検査なので）。テープにあるか・記録しないかの判定は、テープのイベントの `file` と `ignore.Matcher` で行う
- 端末の表示の桁は `cellWidth`（全角は2桁）で揃える。出力の文言は、UIゲートで了承した例と `init_tapes_test.go` が一致を確かめる

## テープを読む・行を数える

- **`json.Unmarshal` は `null` をエラーにせず、ゼロ値を入れる。** `"why":null` が空文字列として読めてしまうので、`internal/tape` の `has()` で欠落として扱う
- **`newText` は「行を `\n` でつないだもの」で、末尾の空行が分からなくなる**（`["a",""]` と `["a"]` は、`Lines` で読み直すと同じ）。行の数は `newStartLine`・`newEndLine` から取る（`tape.NewLines`）。`Splice` に `newText` を渡す前に、行に分けておく（`SpliceLines`）

## 失敗の記録（`failure`）

- `look`・`edit` が失敗したら、`core.recordFailure`（`internal/core/failure.go`）の**1か所**でテープに `failure` を書く。`Core.Look`・`Core.Edit` が、返す前に呼ぶ。MCP の層で弾く失敗（必須の入力がない、型が違う）は `Core.RecordInputFailure`
- **`look` の `expect` の中身も、テープに書かない**（`edit` の `newText` と同じ。ファイルの内容が入りうる）。`expect` だけで範囲を探す呼び出し（`Locate`）の失敗は、`startLine`・`endLine` を `null` にする
- **`edit` を `file` と `expect` で呼ぶ失敗**（`locateEdit`）：`file` は `look` と同じ伏せ方で書き、`startLine`・`endLine` は渡されたときだけ。`expect`・`newText` は書かない。範囲は、渡された行番号 → 最後の look（`tape.State.LastLook`。hook の Read も含む）からずらした行番号 → `expect` のただ1か所、の順。1 と 2 が別の場所なら `content_ambiguous`。挿入（空の範囲）は、`LastLook` が `LastChange` より後のときだけ受ける（`insert: "after"|"before"` は、決めた範囲 `a..b` を `insertAt` で空の範囲 `b+1..b`・`a..a-1` に替えて書く。`expect` を確かめるので、この制限を受けない。テープは空の範囲の `edit` のまま）
- **`look` の `search`**（`core.Search`、`internal/core/search.go`）：当たった行ごとに、その1行の `look` を同じ `why` でテープに書き（20件まで）、トークンを返す。0件は何も書かない。失敗は `look` と同じ1か所（`file` と `why` だけ。`search` の文字列は書かない）。MCP の層（`callSearch`）で、行番号・`expect` との併用を断る
- **`edit` の `edits`**（`core.Edits`、`internal/core/edits.go`）：まず `checkEdit`（項目ごとの入力の検査）、次に全項目の範囲を `resolve`（呼ぶ前の状態。ファイルごとに `loadTarget` で1回だけ読んで見張る）、重なりを調べ、ファイルごとに上から順にずれを足しながら中身を作り、**全ファイルを書いてから**テープに `edit` を項目ごとに書く（`why` は同じ）。失敗は `failure` が1つで、`edits[i]: ` を `message` の頭に付け、その項目の `file`／`selection`／行番号を今の伏せ方で書く（`editFailure`）。重なりは項目なし
- **`edit` の `old`・`new`**（`core.locateOld`、`content.go`）：`file` と一緒に、行の一部を直す。当たりは重なりも数え（`aa` は `aaa` に2か所）、渡した行 → 最後の look からずらした行 → ファイル全体の順で、最初に当たりのあるもので決め、1か所だけを受ける（0は `content_not_found` と空白違いの `nearMatches`、2以上は `content_ambiguous`）。範囲は当たりにかかる行を丸ごと（old が改行で終わり new が違うと次の行がつながる）。`editPlan.put` が置く行で、テープは行全体の `edit`（`from` は `null`）。`old`・`new` はテープに書かない。`replace` の `use_edit` の `actual.edit` は `{file, old, new}`（重なる当たりでは付けない）。MCP の層は `editInputOf` に項目の検査を集める（`edits` の項目も同じ）
- **`replace` の失敗**（`core.Replace`。版 0.1.4 までの `sub`）も同じ1か所で書く。`count` が 1 で文字列が1か所だけのとき（`use_edit`。`useEdit`）は何も変えず、`actual` に場所と、そのまま `edit` に渡せる呼び出しを付ける（`message` は場所だけで、ファイルの中身を入れない。テープに載るため）。結果は場所ごとの `hits`（`hitsOf`。同じ行は1件、1ファイル20件まで）で、トークンは返さず、テープの `replace` の `selection` は `null`。`old`・`new` はテープに書かない（`newText` と同じ）。`file` は、ファイルを1つだけ渡したときだけ書く。複数のときは、パスの誤りの文も決まった文に替える（どれかのパスが実際のパスを含みうるため）。`replace` は、全部のファイルを読んで数えて計画を立ててから書く（数が違えば何も書かない。`count_mismatch`）。変えたファイルごとに `replace` を1つ（`hits`）
- **`new` の失敗**（`core.New`）も同じ1か所。`content` は書かない。`file` は渡したパスを、今の伏せ（絶対パス・外・`ignored_file`）に通して書く。ファイルは、同じディレクトリの一時ファイルに書いて `os.Link` で置く（先にできていたら `file_exists`。鍵と同じ先勝ち）。**親ディレクトリは、いちばん近い既にある祖先を `EvalSymlinks` して、作業場の中か・記録しない場所でないかを確かめてから作る**（`readTarget` は、まだないパスの途中のリンクを見ない）。`readTarget` は、`a/b` の `a` がファイルのとき（`ENOTDIR`）も「ない」として扱う
- **一致しない失敗の `nearMatches`**（`core.withNear`・`nearReplace`）：`look`・`edit` の `expect` と `replace` の `old`（見つかった数が `count` より少ないとき）が、空白の連続と行末の空白だけ違って見つかるとき、`core.Error.NearMatches` に入れる（`actual` の隣。5件まで）。空白だけ違う場所がなく、`expect` が行の一部として含まれるときは、その行を入れる（`partLines`・`nearOrPart`。`look`・`edit` だけ。`message` は「行全体で書く」と言う）。**行はファイルの中身なので、テープには書かない**（`failure` は `code` と `message` だけ。`message` は場所だけを言う）。新しい入口を足すときは、行が `message` に入らないことをテストで確かめる（`noContentInFailures`）
- **返事を変えない。** 書けなくても握りつぶす（AI の作業を止めない）。AI に返すエラー文は今のまま（パスを含む）
- **テープに実際のパスを書かない**：絶対パス・作業場の外・`ignored_file`・`internal_error`（`.srwrignore` が読めないとき）は `file` を `null` にし、エラー文を決まった文に替える。`replace` の `newText` も書かない。新しい失敗の入口を足したら、この伏せを通ることを確かめ、テストを書く
- `failure` のコマは `timeline.Builder.Add` が作る。送るかは `kinds`（`timeline.Filter`）で決まる。ふだんは送らない

## エラー

- エラーは握りつぶさない。`docs/reference/mcp.md`・`protocol.md` のエラーコードは定数として定義し、応答までそのまま伝える
- `panic` は使わない（プログラミングミスの検出を除く）
- `why` は入力スキーマとサーバーの両方で検査する（空白だけも不可）

## 記録しないファイル

- `look`・`edit`・`replace`・`new`・hook・external の検知の**すべての入口で**、`internal/ignore` を通す。1か所でも漏れると、秘密情報がテープに入る
- 判定は `core.readTarget` の1か所に置いてある（`.srwrignore` は呼ばれるたびに読む。読めなければ何も記録しない）。**新しい入口を足すときは、`readTarget` を通ることを確かめ、その入口のテストを書く**

## テスト

- 表駆動。`t.TempDir()` の中で行い、リポジトリ内のファイルを書き換えない
- `internal/core` の行番号補正は、境界（範囲より上・下・重なり・空範囲・末尾への追記・`external` をまたぐ）をすべてテストで押さえる
- `internal/tape` は `extension/test/fixtures/*.expected.json`、`internal/timeline` は `extension/test/golden/` の全本と一致することを確かめる（`.claude/rules/tape.md`）
- セッションとロック・鍵の作成は、**最初から**2つのプロセス（またはゴルーチン）が同時に動くテストで押さえ、`qsoku race` を通す
- `srwr view` のテストは、`XDG_CACHE_HOME` を `t.TempDir()` に向ける（サンドボックスでは `~/.cache` が読み取り専用）
- わざとロジックを壊してテストが落ちることを確かめる（`.claude/rules/testing.md`「壊して確かめる」）
