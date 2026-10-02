# srwr

AIエージェントの編集を、**理由（`why`）付きで記録して、エディタでコマ送りで再生する**道具。

- AI は `select`（見る）と `replace`（変える）の2つのコマンドだけでファイルを編集する。どちらにも `why` を添える
- その操作は**テープ**（`.srwr/tapes/*.tape.jsonl`）に記録される
- **VSCode**（拡張 srwr-view）でも、**ターミナルの Vim**（`srwr view`）でも、選択範囲のコードと `why` を1コマずつ見られる

> **作り直しの最中。** 仕様は [`docs/`](docs/README.md)。開発の段階は [`dev/roadmap.md`](dev/roadmap.md)。

## 開発

- devcontainer を開く（Go・Node.js・`gh` は devcontainer の feature、`postCreate.sh` が Vim・mtqg・qsoku・ShellCheck・Trivy・golangci-lint と Bash サンドボックスの依存を入れる）
- `qsoku check`（Go・拡張・Vim の検査とテスト）
- ルールは [`CLAUDE.md`](CLAUDE.md) と `.claude/rules/`。開発の記録は mtqg（`mtqg context`）
- `handoff/` は引き継ぎ資料（git に入れない）。無ければ、前のリポジトリの `next-space/handoff/` を置く

## ライセンス

MIT
