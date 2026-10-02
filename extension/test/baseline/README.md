# baseline（画面の基準）

偽の `vscode` の上で本物の拡張と本物の `srwr` を動かして取った、コマごとの画面（文書・装飾・一覧・バー）の基準。`capture.test.ts` が、今の取得結果と比べる。

- 最初の基準は、前の実装が取った記録（`handoff/checklist/captured/`）の写し。**書き換えない。** 食い違えば実装を直す
- 違いが正しい変更によるものなら、理由を書き、人間の了承をもらってから更新する（`.claude/rules/working-with-human.md`）
- `all_*.json`：録画（`why-basic`・`external`・`no-why`）の全コマ。`live_*.json`：ライブの各段階
