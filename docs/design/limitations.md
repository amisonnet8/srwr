# Limits and open points

*[日本語](limitations_ja.md) | **English***

**Readers**: people who use srwr, and people who join the development. Trade-offs of the design, what is not decided, and candidates to add.

## 1. Limits (trade-offs of the design)

- **A change to a file that was never touched cannot be seen.** Only a file whose content the tape already holds can become `external` (holding a base for every file would make the tape big). The exception is a new file made by a Bash command: in a git work tree it is found and recorded (`created`). A file made by the agent's own Write is not
- **After an external change that alters the range or the number of lines above it, the selection tokens issued before it cannot be used.** The content check gives `selection_mismatch`. srwr does not estimate the shift of lines from the content of the external change
- Files with a CR (CRLF), a NUL or content that is not UTF-8 cannot be handled (`unsupported_file`)
- The ban on Edit/Write and the hook depend on the settings of Claude Code. With other agents, `look` / `edit` / `new` / `session` themselves can still be used, because they are MCP
- **Diffs in VSCode**: there are no blank lines to align the lines, and the left and right sides do not scroll together (because the standard diff screen is not used)
- **Line numbers in VSCode**: on a frame with a reason line, the real file's own numbers appear at the left edge of the text, not in the gutter
- **Wrapping width of the reason**: VSCode uses a fixed display width of 100; Vim uses the width of the window
- **The choice of which frames to show is not saved.** It is kept while the editor runs; a new start shows look, edit and external. The kind `look` cannot tell the AI's own `look`s from those the hook recorded (the ones without a `why`)
- **A failure frame shows a range only for a `look`** (and, in the tape, for an `edit` that pointed by `file` and line numbers). The viewers print `-` for the others
- **Live follows the tape with the newest update time.** A tape that `srwr trace --as-tape` writes is passed over (it is a derived tape, not a recording), but a new session's tape is the newest, so live moves to it
- **A tape cut by `srwr trace --as-tape` has no lineage** (`selection` and `from` are `null`), and its last diff is against the file as it is now, so work after the commit shows as a difference
- **VSCode uses the first folder of the workspace** (one view server per window)

## 2. Open points

Things not yet decided. When one is decided, it is written in the proper document and removed from here.

- How to show it when the agent creates a new file with its own Write in lenient mode (now it is not recorded, and shows only if the file is touched later)
- How to show it when there are many frames of the investigation. Turning `look` off ([vscode.md](../reference/vscode.md)) hides them all, and the hook records at most 100 `look`s for one call, so a single call does not fill the tape; showing only what has a `why` is not decided
- The length of the HMAC of the selection token (3 bytes; a trade-off between the rate of catching copying mistakes and the number of characters)
- Whether `look` should always return `lines` (a trade-off with token consumption; `edit` can already leave them out with `brief`)

## 3. Candidates to add

None of these is decided. When adding to the screen, fix the screen specifications of VSCode and Vim ([vscode.md](../reference/vscode.md), [vim.md](../reference/vim.md)) first.

- **A way to skip the frames of the hook's investigation only**: when the hook records many frames without a `why`, follow only the frames that have one (kinds are for `look` as a whole)
- **Frames of human edits** (`author.kind: human`): since the editor runs in the same workspace, let a person add a short note on the spot at the time of the edit. The note is shown in the same form as the `why` line. It could answer the problem that an external change in the middle has no `why`
- **A way to explain an external change in the middle**: the `author` of `external` is fixed and does not tell who changed it. Using a commit message or a note added by a person are candidates (`srwr trace` tells which operation wrote an added line of a commit, which is a different question)
- **Settings for the time that separates sessions** (fixed at 30 minutes after the last event; the AI's `session` tool and `srwr tapes new` end a session at once) **and for the interval of the live watch** (fixed at 200 milliseconds) ([settings.md](../reference/settings.md) lists what cannot be set)
- **Automatic cleanup of tapes** (now the user deletes them explicitly with `srwr tapes prune`; a closed tape is already compressed)
- **Bundling the `srwr` binary in the VSCode extension**, so that a person who has only received a tape can view it with the extension alone. It needs a `.vsix` for each OS; the current choice and its reason are in [decisions.md](decisions.md)
- Support for other editors (add a client that follows [protocol.md](../reference/protocol.md))
- Support for agents other than Claude Code
