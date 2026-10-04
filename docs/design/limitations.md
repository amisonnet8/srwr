# Limits and open points

*[日本語](limitations_ja.md) | **English***

**Readers**: people who use srwr, and people who join the development. Trade-offs of the design, what is not decided, and candidates to add.

## 1. Limits (trade-offs of the design)

- **A change to a file that was never touched cannot be seen.** Only a file whose content the tape already holds can become `external` (holding a base for every file would make the tape big). The exception is a new file made by a Bash command: in a git work tree it is found and recorded (`created`)
- **After an external change that alters the range or the number of lines above it, the selection tokens issued before it cannot be used.** The content check gives `selection_mismatch`. srwr does not estimate the shift of lines from the content of the external change
- Files with line breaks other than LF (CRLF) and binary files cannot be handled (`unsupported_file`)
- The ban on Edit/Write and the hook depend on the settings of Claude Code. With other agents, `select` / `replace` themselves can still be used, because they are MCP
- **Diffs in VSCode**: there are no blank lines to align the lines, and the left and right sides do not scroll together (because the standard diff screen is not used)
- **Line numbers in VSCode**: on a frame with a reason line, the real file's own numbers appear at the left edge of the text, not in the gutter
- **Wrapping width of the reason**: VSCode uses a fixed display width of 100; Vim uses the width of the window

## 2. Open points

Things not yet decided. When one is decided, it is written in the proper document and removed from here.

- How to show it in lenient mode when the AI creates a new file
- How to show it when there are many frames of the investigation (the hook records at most 100 `select`s for one call, so a single call does not fill the tape)
- The length of the HMAC of the selection token (3 bytes; a trade-off between the rate of catching copying mistakes and the number of characters)
- Reconsidering that `select` always returns `lines` (a trade-off with token consumption)

## 3. Candidates to add

None of these is decided. When adding to the screen, fix the screen specifications of VSCode and Vim ([vscode.md](../reference/vscode.md), [vim.md](../reference/vim.md)) first.

- **A setting to skip the frames of the investigation**: when the hook records many frames without a `why`, follow only the frames that have one
- **Frames of human edits** (`author.kind: human`): since the editor runs in the same workspace, let a person add a short note on the spot at the time of the edit. The note is shown in the same form as the `why` line. It could answer the problem that an external change in the middle has no `why`
- **A way to explain an external change in the middle**: the `author` of `external` is fixed and does not tell who changed it. Using a commit message or a note added by a person are candidates
- **Settings for the time that separates sessions** (fixed at 30 minutes after the last event) **and for the interval of the live watch** (fixed at 200 milliseconds) ([settings.md](../reference/settings.md) lists what cannot be set)
- **A retention period and automatic cleanup of tapes** (now the user deletes them explicitly with `srwr tapes prune`)
- **Bundling the `srwr` binary in the VSCode extension**, so that a person who has only received a tape can view it with the extension alone. It needs a `.vsix` for each OS; the current choice and its reason are in [decisions.md](decisions.md)
- Support for other editors (add a client that follows [protocol.md](../reference/protocol.md))
- Support for agents other than Claude Code
