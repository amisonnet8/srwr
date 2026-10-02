# golden（固定の正解のデータ）

P2 で、実験版の TypeScript の `Timeline` が、テープからコマの列を作った結果を書き出したもの。Go の `internal/timeline`（`golden_test.go`）と、拡張のテスト（`fakeserver.ts`・`timeline.test.ts`・`server.test.ts`）が答え合わせに使う。

- **書き換えない。** 作った `Timeline`（TypeScript）は P3 で消したので、作り直す道具もない。食い違いが出たら、Go を直す。正解が誤りだと考えるときは、ユーザーに確認する（UI の見え方が変わるため。`.claude/rules/tape.md`）
- 1ケース1ファイル。`frames`（変更前・変更後つきのコマ）、`jumpLabels`（閾値 30）、`files`、`contentAt`、元になったテープ（`tape` のパス、または本文 `tapeText`）と、最後の差分の元（`current`）を持つ
- 実物のテープ（`basic`・`external-text`・`phase0-bash`・`real-playground`・`ui-check-*`）のほか、旧形式の `external`・消えたファイル・壊れた行・行番号の境界・ジャンプラベルの境界などの境界ケースを、本文ごと持つ
