# The big picture of srwr's design

*[日本語](overview_ja.md) | **English***

**Readers**: people who want to know how srwr is built, and people who join the development. For how to use it, see [the reference documents](../reference/cli.md); for the reasons behind decisions, see [decisions.md](decisions.md).

## What srwr is

A tool that lets an AI agent edit files with just two commands (`select` / `replace`), records the operations on a **tape**, and **replays them frame by frame** in an editor. Showing the `why` (the reason) together with the code of the selected range helps a person understand the AI's work.

## Premises

These five are where the design starts.

| Premise | Effect on the design |
|---|---|
| A series of operations with a `why` lets a person understand what the AI did. Without the `why`, neither the series nor the diff is easy to read | Editing is limited to two commands that require a `why` |
| The `why` is the heart of understanding. The code of the selected range only makes the picture concrete | The screen shows the reason line and the range in the same place |
| In an editor, syntax highlighting, the surrounding context and language features come for free | The place to view is an editor (VSCode and Vim) |
| A change made by something other than the AI (an external change) cannot be understood from a label. It can be understood if the content is visible | An external change is shown as a side-by-side diff |
| Replay is mostly read one frame at a time. Live may run at the AI's real speed | Viewing is frame-by-frame stepping only |

## Basic policy

### The boundary is "the frames"

- The look and behavior are fixed ([vscode.md](../reference/vscode.md), [vim.md](../reference/vim.md))
- The boundary between the UI and the back end is **the frames** ([protocol.md](../reference/protocol.md)). The view server builds them and hands them to the editor
- Changes at the back (the tape format, sessions, ignored files, and so on) are absorbed inside the view server and do not reach the editors
- When a display part changes, the screen specifications of both VSCode and Vim are fixed first

### Lines that are held

| Line | Content |
|---|---|
| The tape is the only record | Replay can be rebuilt completely from the tape alone. The real files are used only for live and for the comparison in the "final diff" |
| Writing, building and drawing are separate | `srwr mcp` and `srwr hook` **write** the tape. `srwr view-server` **reads the tape and builds the frames**. The editor side only **draws**; it does not read the tape directly. It writes neither the tape nor the real files |
| The boundary between Go and the editors is the protocol | The tape format is a promise inside Go only. The promise with the editors is the view server's protocol. To change it, fix the documents first, then the server and both clients together |
| Editing takes two commands only | There is no `replace` without `select` |
| `why` is required (for edits through srwr) | Checked both in the input schema and on the server |
| No more dependencies | Go has zero external dependencies (MCP and the view server use our own JSON-RPC). The VSCode extension has zero runtime dependencies (`tsc` only). Vim relies on no other plugin |

### View in the editor, share the tape

- The places to view are **VSCode (srwr-view) and Vim (srwr-view.vim)**. There is no export to Markdown or HTML
- To share, hand over the tape itself. The person who receives it opens it in their own editor

### Why Vim is supported

- **So that it can be viewed in the terminal alone.** srwr is used in a development environment that has an AI agent (Claude Code and the like). A person who runs the agent in a terminal can replay the tape from the same terminal
- **Vim is the target; Neovim is not.** Neovim is installed only by those who use it, which does not fit the aim of "anyone can view it in a terminal"
- Environments with an AI agent are new, so **Vim 9.0 or later** is assumed. There are no branches for older Vim
- **Users are not asked to install a plugin.** `srwr view` loads the Vim scripts embedded in the binary and starts Vim
- Syntax highlighting is left to Vim

## Structure

```
                           ┌────────────────────────────────┐
AI ──select / replace────▶│ srwr mcp                       │──▶ real files
AI ──Read/Bash/Grep/Edit─▶│ srwr hook (Claude Code hook)   │
                           └──────────────┬─────────────────┘
                                          │ take the lock and append
                                          ▼
                        .srwr/tapes/<id>.tape.jsonl (one session = one tape)
                                          │ read only
                                          ▼
                           ┌────────────────────────────────┐
                           │ srwr view-server (Go)          │
                           │ reads tapes, builds frames,    │
                           │ the document state of each     │
                           │ frame, the final diff, and     │
                           │ watches for live               │
                           └───────┬──────────────┬─────────┘
                 JSON-RPC (stdio)   │              │  JSON-RPC (stdio)
                                   ▼              ▼
                        srwr-view (VSCode)   srwr-view.vim (Vim)
                        draws only           draws only (started by srwr view)
```

| Part | What it is |
|---|---|
| **`srwr`** (a single Go binary) | The subcommands `mcp`, `hook`, `view-server`, `view`, `init` and `tapes` ([cli.md](../reference/cli.md)) |
| **Tape** | An append-only JSONL file. One session = one tape ([tape.md](../reference/tape.md)) |
| **srwr-view** (VSCode extension) | **Displays** live, replay, stepping and diff frames |
| **srwr-view.vim** (Vim script) | Shows the same UI in Vim. Embedded in the `srwr` binary |

## Related

- Reasons for decisions: [decisions.md](decisions.md)
- The selection token: [token.md](token.md)
- Limits and open points: [limitations.md](limitations.md)
