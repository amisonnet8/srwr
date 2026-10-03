# srwr documentation

[日本語](README_ja.md)

srwr lets an AI agent edit files with just two commands, `select` and `replace`, records the operations on a **tape**, and **replays them frame by frame** in an editor.

## Who reads what

| Reader | Reads |
|---|---|
| People who **use** srwr | [reference/cli.md](reference/cli.md) (install, commands), [reference/settings.md](reference/settings.md) (all settings), [reference/mcp.md](reference/mcp.md) (the tools the AI uses), [reference/vscode.md](reference/vscode.md) and [reference/vim.md](reference/vim.md) (viewing) |
| People who **connect to the tape or the view server** (support for another editor, a tool that reads tapes) | [reference/tape.md](reference/tape.md), [reference/protocol.md](reference/protocol.md), a recorded run [examples/protocol-session.md](examples/protocol-session.md), a hook example [examples/hook.md](examples/hook.md) |
| People who want to know **how srwr is built**, or who join the development | [design/overview.md](design/overview.md), [design/decisions.md](design/decisions.md), [design/limitations.md](design/limitations.md) |

## Layout

| Folder | Role |
|---|---|
| [reference/](reference/cli.md) | **The rules.** How to use srwr, and the promises it keeps |
| [design/](design/overview.md) | **Design and the reasons for decisions.** The big picture, the reasons, the selection token, limits and open points |
| [examples/](examples/select-replace.md) | **Working examples.** Exchanges taken from real runs |

## Reading order

1. [reference/cli.md](reference/cli.md) — the whole picture and installation
2. [reference/mcp.md](reference/mcp.md), then [examples/select-replace.md](examples/select-replace.md) — what the AI does
3. [reference/tape.md](reference/tape.md) — what is recorded
4. [reference/vscode.md](reference/vscode.md) or [reference/vim.md](reference/vim.md) — how to view it

## Languages

Every document here is in English, and the Japanese version of each has the same name with `_ja` (for example `reference/cli_ja.md`). The words srwr shows on the screen and in the terminal are English by default; see the setting `SRWR_LANG` in [reference/settings.md](reference/settings.md) for Japanese.
