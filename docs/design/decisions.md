# Design decisions and their reasons

*[日本語](decisions_ja.md) | **English***

**Readers**: people who want to know how srwr is built, and people who join the development. Read this when you want to know "why is it like this". The rules themselves are in [the reference documents](../reference/cli.md); the reasons for the selection token are in [token.md](token.md).

## How editing works

### Edit / Write are forbidden by settings, not by instructions

The biggest risk was that "the AI always edits through the two commands" would be enforced by an instruction document alone. In Claude Code, the project's `.claude/settings.json` can forbid the built-in editing tools.

```json
{ "permissions": { "deny": ["Edit", "Write", "MultiEdit", "NotebookEdit"] } }
```

With this, srwr is the only proper way to change a file. What there is to check also changes, from "does it obey the rule" to "**can the work be done with just two commands**". Editing through Bash (`sed -i`, redirects, and so on) cannot be shut out, so it is detected as `external` and shown. This is strict mode ([cli.md](../reference/cli.md)). If the user prefers, lenient mode is also available: nothing is forbidden, and a hook records instead.

### `select` is the precondition of `replace`

A `select` that only returns content looks to the AI like a call with "cost and no gain", and tends to be skipped. With the selection token, `replace` is impossible without `select`. `select` becomes a step that cannot be skipped, so the tape always holds "look, then change". For the AI too, there is no need to handle line numbers or hashes: it just passes the string it received.

### srwr absorbs the shifting of line numbers

When a range is given by line numbers, it drifts as edits pile up. This is why the Edit of Claude Code takes the `old_string` form. In srwr the token carries the `seq` at the time it was issued, and srwr corrects the numbers automatically from the later edit history. Then it checks the hash of the content, and edits nothing if it does not match ([token.md](token.md)).

### Only two commands

There are no commands for moving a cursor, fetching or searching. Searching and reading are left to the Read and grep the AI already has. srwr's responsibility is only "to record the moment a file really changes, and the range being looked at just before". This keeps visualization simple, and also helps the AI use the tool well, because an LLM tends to choose wrongly when there are many similar tools.

### `why` is required

Without a series of operations with a `why`, a person can hardly understand the AI's work. If it were optional, this value would be lost, and in practice it would be left out. The language is the same as the conversation with the user. If it is written in a language people cannot read, it has no effect.

### The investigation goes on the same tape

The AI often finishes investigating with Bash and Read and then `select`s only the place to fix, so the investigation stays outside the tape. Therefore a hook records the operations of the existing tools on the same tape. `srwr mcp` and `srwr hook` are one binary and write to the same tape. They are shown as frames without a `why` (`null`).

## The tape and the real files

### Write the real file first, then append to the tape

Even if the process dies in between, the next time srwr touches the files, the mismatch between the real file and the tape shows up as `external`. In the opposite order, an operation would stay on the tape that is not in the real file.

### Only files whose content the tape holds are subject to `external`

A change to a file that was never touched cannot be seen, because there is no base. Holding a base (the hash of every file at the start of a session) would make the tape big. This is accepted as a limit ([limitations.md](limitations.md)).

### An external change is inserted in a form that shows its content

A label alone does not tell what changed. So `external` and `final` are shown as a side-by-side diff. The label goes in the heading (the tab title). A line shaped like the `why` line cannot be made on a diff screen.

The tape holds an `external` as the lines that changed (`hunks`), not as the whole file, and no `snapshot` follows it. A measured tape was 89% whole texts, most of them the same text twice (an `external` and the `snapshot` after it). The whole text is kept only once per file, at the first touch; the tape still replays alone, because the lines that changed are applied to a text the same tape holds.

### The view server does not trust the paths written on the tape

So that a shared tape cannot make it read an arbitrary file. When the current file is read for the final diff, a path that points outside the workspace (`..`, an absolute path, a symbolic link that points outside) is not read and is treated as "does not exist".

## How display works

### The tape format is a promise inside Go only

If each editor had its own code to read the tape and build the frames, VSCode and Vim would disagree, and every change of the format would mean fixing everything. So the reading side is gathered in one place, the view server (Go), and the promise with the editors is only the [protocol](../reference/protocol.md). A change of the tape format is absorbed in the server.

### No compatibility promise for the tape format until v1

srwr is still experimental and has no users who depend on old tapes. Keeping every old form readable would stop us from making the tape lighter and simpler (for example, dropping the whole-text `snapshot` that repeats an `external`). So until v1 the format may change without compatibility, and a tape is read by the version that wrote it. From v1 on, the promise in [tape.md](../reference/tape.md) applies: fields may be added, and the meaning of an existing one is not changed.

### Communication is newline-delimited JSON

The LSP format (with headers) is not handled by Vim as it is. Newline-delimited JSON is handled by Vim's own `job` and a channel in `nl` mode as it is, and VSCode can write it without a library. It is the same format as MCP, so the implementation can be shared.

### One server per client

It is not a shared daemon. What is shared is the tape itself.

### No more external dependencies

Go uses only the standard library (MCP and the view server use our own JSON-RPC, and live uses polling). The VSCode extension has zero runtime dependencies. Vim relies on no other plugin. Distribution is simple (`go install` is enough, no cgo), and there is no need to worry about the vulnerabilities and licenses of dependencies.

### The VSCode extension does not carry the `srwr` binary

The extension (`.vsix`) is distributed alone, and `srwr` is installed separately (`go install` or GitHub Releases), as [cli.md](../reference/cli.md) says. The reasons: whoever lets an AI use srwr installs `srwr` anyway (`srwr mcp` and `srwr hook` are the binary), and Vim uses the same binary, so there is one way to install it. A binary inside the extension could differ in version from the one the AI runs. And a `.vsix` for each OS and CPU (six of them) is avoided. The cost is two steps for a person who only wants to view a tape; the extension tells them how to install `srwr` when it is not found.

## UI decisions

The screen specifications are in [vscode.md](../reference/vscode.md) and [vim.md](../reference/vim.md). This part says why they were decided so.

| Decision | Reason |
|---|---|
| **Viewing is frame-by-frame stepping only.** There is no autoplay, speed or real-time button | The viewer steps one frame at a time while reading the reason. Motion does not help understanding, and adds things to verify |
| **select is blue, replace is orange**, two colors. Only the dots of external and final changes are purple | A few colors are enough to tell the kinds apart |
| **The reason line is inserted before the range, as a real line** (white bold text on a dark background). The range has the lighter version of the same color | The reason and the code can be read one after the other in the same place. What is inserted is in a virtual document, so the real file is not affected. In return the line numbers would not match, so the real file's own numbers are drawn by the extension |
| **A diff is side by side, with only the changed lines painted (before = blue, after = orange)** | What changed is clear from the content. The standard diff screen of VSCode cannot change its colors, so two editors, left and right, get our own colors |
| **The operation list is flat, numbered from 1** | Indenting by parent and child is hard to read and has no use. The number is the same as the position in the bottom bar |
| **"Back" and "Forward" and the position are always shown.** The side that cannot be taken is dimmed, not hidden | The position does not move, so the operations are easy to remember |
| **Live is the same screen as replay.** "● LIVE" while following, "Back to LIVE (N new)" on an old frame | There is no other screen to learn. The screen does not move while an old frame is being read |
| **srwr has no settings for the look** (the only setting is where `srwr` is; Vim's highlight groups can still be overridden) | The same look can be shared, and combinations of settings need not be checked |
| **Real files are not opened. There is no jump display** | The tape is self-contained. What is missing is added to the frames |
| **The final diff stays as one of the diff frames** | A change to the files after the recording can be checked in the same way |

## How to think about tests

- Go tests are table-driven. **Break the logic on purpose and check that the tests fail.** They run inside `t.TempDir()`
- For the view server, every fixture tape is checked: the frames must match the fixed golden data
- The extension's tests use **real tapes** written by Go as fixtures. Hand-written tapes are not used
- The look is hard to check with automatic tests. The content of the screen (documents, how colors are applied, the number of screens) is tested; the look itself is checked by eye
