# srwr のドキュメント

*[English](README.md) | **日本語***

srwr は、AI エージェントに `select` / `replace` の2コマンドだけでファイルを編集させ、その操作を**テープ**に記録し、エディタで**コマ送りで再生**する道具。

## 誰が、何を読むか

| 読者 | 読むもの |
|---|---|
| srwr を**使う人** | [reference/cli.md](reference/cli_ja.md)（導入・コマンド）、[reference/settings.md](reference/settings_ja.md)（設定の一覧）、[reference/mcp.md](reference/mcp_ja.md)（AI が使うツール）、[reference/vscode.md](reference/vscode_ja.md)・[reference/vim.md](reference/vim_ja.md)（見る） |
| srwr の**テープや表示サーバーとつなぐ人**（他のエディタへの対応、テープを読む道具） | [reference/tape.md](reference/tape_ja.md)、[reference/protocol.md](reference/protocol_ja.md)、動かした例 [examples/protocol-session.md](examples/protocol-session_ja.md)、hook の例 [examples/hook.md](examples/hook_ja.md) |
| srwr の**作りを知りたい人・開発に加わる人** | [design/overview.md](design/overview_ja.md)、[design/decisions.md](design/decisions_ja.md)、[design/limitations.md](design/limitations_ja.md) |

## 構成

| フォルダ | 役割 |
|---|---|
| `reference/` | **決まり**。使い方と、守る約束 |
| `design/` | **設計と判断の理由**。全体像、判断の理由、範囲トークン、制限と未定事項 |
| `examples/` | **動く例**。実際に動かして取ったやり取り |

## 読む順番

1. [reference/cli.md](reference/cli_ja.md) — 全体と導入 → [examples/workflow.md](examples/workflow_ja.md) — `srwr init` から片付けまで、すべてのコマンドで一回り
2. [reference/mcp.md](reference/mcp_ja.md) → [examples/look-edit.md](examples/look-edit_ja.md) — AI が何をするか
3. [reference/tape.md](reference/tape_ja.md) — 何が記録されるか
4. [reference/vscode.md](reference/vscode_ja.md) または [reference/vim.md](reference/vim_ja.md) — どう見るか

## 言語

文書は、英語が基準で、それぞれの日本語版は同じ名前に `_ja` を付けたもの（例：`reference/cli_ja.md`）。srwr が画面や端末に出す文言も、英語が既定。日本語にするには、[reference/settings.md](reference/settings_ja.md) の `SRWR_LANG` を見る。
