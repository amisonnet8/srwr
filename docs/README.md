# srwr のドキュメント

srwr は、AI エージェントに `select` / `replace` の2コマンドだけでファイルを編集させ、その操作を**テープ**に記録し、エディタで**コマ送りで再生**する道具。

## 誰が、何を読むか

| 読者 | 読むもの |
|---|---|
| srwr を**使う人** | [reference/cli.md](reference/cli.md)（導入・コマンド）、[reference/settings.md](reference/settings.md)（設定の一覧）、[reference/mcp.md](reference/mcp.md)（AI が使うツール）、[reference/vscode.md](reference/vscode.md)・[reference/vim.md](reference/vim.md)（見る） |
| srwr の**テープや表示サーバーとつなぐ人**（他のエディタへの対応、テープを読む道具） | [reference/tape.md](reference/tape.md)、[reference/protocol.md](reference/protocol.md)、動かした例 [examples/protocol-session.md](examples/protocol-session.md)、hook の例 [examples/hook.md](examples/hook.md) |
| srwr の**作りを知りたい人・開発に加わる人** | [design/overview.md](design/overview.md)、[design/decisions.md](design/decisions.md)、[design/limitations.md](design/limitations.md) |

## 構成

| フォルダ | 役割 |
|---|---|
| [reference/](reference/cli.md) | **決まり**。使い方と、守る約束 |
| [design/](design/overview.md) | **設計と判断の理由**。全体像、判断の理由、範囲トークン、制限と未定事項 |
| [examples/](examples/select-replace.md) | **動く例**。実際に動かして取ったやり取り |

## 読む順番

1. [reference/cli.md](reference/cli.md) — 全体と導入
2. [reference/mcp.md](reference/mcp.md) → [examples/select-replace.md](examples/select-replace.md) — AI が何をするか
3. [reference/tape.md](reference/tape.md) — 何が記録されるか
4. [reference/vscode.md](reference/vscode.md) または [reference/vim.md](reference/vim.md) — どう見るか
