# Design decisions and their reasons

*[日本語](decisions_ja.md) | **English***

**Readers**: people who want to know how srwr is built, and people who join the development. Read this when you want to know "why is it like this". The rules themselves are in [the reference documents](../reference/cli.md); the reasons for the selection token are in [token.md](token.md).

Many of the choices about the tools come from rounds of experiments: AI agents were given real requests (a fix, a new file with its test, a change in many places) and the tape and the complaints were read afterwards. The numbers below are the ones seen then. Only the present form and the reason are kept here; the history of each version is in git.

## The tools and how editing works

### Edit and Write are forbidden by settings, not by instructions

The biggest risk was that "the AI always edits through srwr's tools" would be enforced by an instruction document alone. In Claude Code, the project's `.claude/settings.json` can forbid the built-in editing tools.

```json
{ "permissions": { "deny": ["Edit", "Write", "MultiEdit", "NotebookEdit"] } }
```

With this, srwr is the only proper way to change a file. What there is to check also changes, from "does it obey the rule" to "**can the work be done with srwr's tools alone**". Editing through Bash (`sed -i`, redirects, and so on) cannot be shut out, so it is detected as `external` and shown. This is strict mode ([cli.md](../reference/cli.md)). If the user prefers, lenient mode is also available: nothing is forbidden, and a hook records instead.

### The tools: `look`, `edit`, `new`, and an optional `session`

The tools are few on purpose: an LLM tends to choose wrongly when there are many similar ones. There are no commands for moving a cursor. Reading a place is `look`, changing it is `edit`, making a file is `new`, and `session` (below) is the one that touches no file. srwr's job is to record the moment a file really changes, and the range being looked at just before.

A `look` that only returns content looks to the AI like a call with "cost and no gain", and tends to be skipped. In the first design a change was impossible without a `select`, and the AI skipped it with other tools: 69 of 74 calls of the old `sub` changed one place only and left no range on the tape. So the design was cut anew. `edit` takes the token that `look` returned, or points at its range by itself (next section), so there is no step that can be skipped, and every change still leaves on the tape the range it changed ("look, then change").

Two tools were taken out again after three rounds with 12 AIs, in which tasks were made on purpose to tempt them. A tool `replace` (the same change in several places) was used 0 times in 12: the AI chose `edits` with an `old` and a `new` for each place, because the places had different surroundings and `edits` is all-or-none as well. An `edits` item with `content` (a new file and the code that uses it in one call) was not used either, even by the request made for it: two `new` calls were simpler, and the build did not fail between them. Both could be done by something else that was used, so both are gone instead of being kept and described. The `replace` event of an old tape is read as an `edit`, the frame that showed a diff with a band for the reason is gone from both editors, and `hits` and `hunks` are no longer sent (`protocolVersion` 3).

### Pointing at a range

**A selection token** is the usual way: it seals the file, the lines and the content that were looked at, and the AI passes it on as it is ([token.md](token.md)). When the line numbers drift as edits pile up (the reason Claude Code's Edit takes `old_string`), srwr corrects them from the edit history of the tape, then checks the hash of the content, and edits nothing if it does not match. So the AI never calculates a line number.

**`file` and `expect`, with no token.** The same experiments showed that a token makes edits slow to send: each needs a `look` before it, calls cannot go in parallel, and several places in one file had to be changed from the bottom up. `expect` (the lines the range holds) is required, because a call must never write to a place only because a line number says so; it is the same check as `look`'s `expect`. The range is found in the order of the line numbers as given, the line numbers moved by the changes after the file's last look, and the one place where `expect` is: the first two follow what the AI saw, and the last is a net for when that does not hold. When the first two point at different places the call is `content_ambiguous`, not a guess. An insertion has nothing to check, so it is accepted only when the file has not changed since a look. The flow "look, then edit with the token" stays the first way in the descriptions and the examples.

**`old` and `new`, for a part of a line.** The AI copied whole long lines (table rows) into `expect` to change a few words, and six calls failed on a character or two. `expect` stays whole lines, because the safety of `file` and `expect` is that a call writes only where the lines the AI saw are. A part of a line is a separate input, with the same safety by a different check: `old` must be in one place only, in the lines given (if any), else in the file, so a wrong place is refused, not guessed. With `old`, one line number is enough (a hint for where to look). `old` and `new` are an input of `edit`, not a new tool, and the tape holds the same `edit` of whole lines as for `expect`, without `old` and `new` themselves.

**`insert`, next to lines that are kept.** The AI wanted to add a line after a line. An empty range could do it, but only while the file had not changed since a look, so the AI wrote the neighbouring line again as `expect` and `newText`, and could get it wrong. With `insert: "after"` (or `"before"`) the lines are pointed at as usual (a token, or `file` and `expect`, which is checked), so the place is still checked after the file has changed; they are kept, and `newText` goes next to them. `insert: "start"` and `"end"` are for the top and the bottom of the file, which cannot move, so they need no look and no `expect`. `insert` is a part of the call, not a new tool or a new tape event: the tape holds an `edit` of an empty range, as before.

### When a call fails: say where, never guess

The aim of every answer to a failure is that the next call is right, without the AI reading the file again, and without srwr changing anything it was not asked to.

- **`nearMatches`.** The AI often wrote `expect` or `old` with spaces where the file has tabs (or the other way round), was told only that nothing was found, and read the file again to find out why. Now, if the text is found but for runs of spaces and tabs and trailing spaces, the error lists those places with their lines as they are, so the next call can copy them. It is next to `actual`, not in it, because `actual` already has a different form for each code. Only the lines of the file go in it, and **they are not written to the tape**: the message, which is on the tape, says only where. Only whitespace is folded; a different word is not "near", because a hint that is wrong costs the AI more than none. For an `expect` that is only part of a line, the lines that hold it as a part are listed instead, and the message says that `expect` must be whole lines
- **`retry`.** When exactly one place can be what the AI meant, the error holds the arguments of the call to make again. Nothing is applied by srwr: the AI reads it and calls. With no place, or two or more, there is none
- **`hint`.** Some things are not errors. An edit that follows an edit of the same file gets a line that tells of `edits`. When an insertion leaves two blocks touching, srwr says so and does not add the empty line: what a block is differs by language (a function, a paragraph, a table), and a wrong guess changes the file. The hint is not given between lines of the same shape (the rows `{…},` of a table test, where an empty line is wrong); functions, paragraphs and list items still get it

### Saying less

- **`above` and `below`**, not `before` and `after`. The result of `edit` holds the lines just above and just below the new range, as the file is now. They were called `before` and `after`, and an AI read them as the content before and after the change, and asked for the old lines. They are named by where they are, and the description says they are not the old content. The tape is not touched: its `before` and `after` are about the whole text, and are another thing
- **`brief` is an input, not the default.** The lines that come back are how the AI checks an edit without reading the file again; it chooses when it need not. For an `edits` of 6 or more items it is the default, because nobody reads back that many edits one by one (`brief: false` for the long form)
- **Long lines are cut** to their first 200 characters in `above` and `below` (`cut: true`), and a search result is cut around the text. `lines` and the tape stay whole. A history of entries of 1,000 to 2,000 characters returned whole lines at the end of a file; a row of a table of thousands of characters filled an answer
- **A `look` cuts an `endLine` past the end**, because it only reads; `edit` does not, since a range that writes must be exactly the one meant. A `look` with no range reads the whole file (up to 2000 lines): `looks` already read whole files, and a single `look` that refused to was the one thing the AI tripped on

### Finding places: `search` and `looks`

In the experiments the AI read files with Read and `sed`/`grep`, not with `look`, so the tape held its reading only as hook records without a `why`, and it still had to call `look` again before editing. `look` with `search` (plain text, one line) returns the matching lines with a token each, so finding and editing is one path in srwr. Every match returned is written as a `look` of its line with the same `why`: a token needs the `look` it came from (its `seq`), and the replay shows what the AI looked at. Neither the tape format nor the screens change.

- **A directory** lists files as git does (what it ignores is not worth reading), leaves out what is not recorded without a word, and writes to the tape only the files that have a match, so searching the workspace does not fill the tape with files nobody looked at
- **`include` and `exclude`** are in `.gitignore` syntax, the syntax `.srwrignore` already uses, so there is no second one. One string is taken as a list of one: two of four AIs wrote it that way or as broken JSON, and a refusal teaches nothing when the meaning is clear. A `!` at the start of `include`, or of the first `exclude`, is refused, since it would match nothing and the AI saw only an empty answer
- **`offset` and `byFile`** let the AI read past the first 20 matches, and say where the matches are when they are not all returned
- **`looks`** reads several files in one call (all or none), because the wall was the number of calls. Its description says it is an argument of `look`, since one AI called it a tool of its own
- **Left out:** a regular expression (a wrong pattern is another way to fail), and reading by symbol (it depends on the language, and a wrong guess is a wrong place)

### Many edits in one call: `edits`

The AI sent 5 to 10 similar edits one by one, each with the same `why`, and each made the build hook run. `edit` with `edits` makes them in one call. It is part of `edit`, not another tool, and the tape does not change: one `edit` for each item, as if they had been sent one after the other from the top of each file down. Every range is found as the files are before the call, so the items do not depend on each other and can be in any order (an item's line numbers are the ones the AI saw), and ranges that overlap are refused, since no order of them would be right. The files are written before the tape, as in every edit; if the process dies in between, the next call finds the difference as `external`. A mistake in one item changes nothing, and the error says which. A `why` in an item is told to be unused instead of being refused: the AI loses nothing by it.

### `why` is required

Without a series of operations with a `why`, a person can hardly understand the AI's work. If it were optional, this value would be lost, and in practice it would be left out. It is checked both in the input schema and on the server. The language is the same as the conversation with the user. If it is written in a language people cannot read, it has no effect.

### The investigation goes on the same tape

The AI often finishes investigating with Bash and Read and then `look`s only the place to fix, so the investigation stays outside the tape. Therefore a hook records the operations of the existing tools on the same tape. `srwr mcp` and `srwr hook` are one binary and write to the same tape. They are shown as frames without a `why` (`null`).

What the hook tells the agent is kept small and calm, because advice that comes at every `grep` is read past:

- The advice to use `look` after a read of one file, and the note after a command that made files, are each given once for a tape
- A file is called "made by the command" only if it was written after the last event of the tape. The hook once called every untracked file the tape did not know "created", so a file that was already there became the command's work
- A generator or a formatter that writes files is fine; the note only tells how to have a reason recorded when the agent writes by hand

### Starting a new tape: `session`

The 30-minute rule cuts tapes at pauses, not at units of work: a morning of "docs first, then the code" is one tape, and a long think in the middle makes two. A tool that lets the AI say where a unit begins, with a title and a reason, fixes that without a new idea in the tape: the title is written in the header of the new tape (one more field, only when there is one), so nothing is appended to a closed tape and a tape written by an earlier version reads as before. It is optional (the 30-minute rule stays, also for a titled tape: a pause is a pause), and it works on the whole workspace like `srwr tapes new`, since the workspace has one current tape. The title is shown only in the lists of tapes, so no replay screen changes and `protocolVersion` stays 3.

### Joining a commit to the tape: `srwr trace`, and why git is not involved

A commit says what changed, and a tape says why, but nothing joined them. The first idea was to put the tapes in git and read the added lines of each commit. That makes srwr depend on how a team uses git (what is committed, how big, and a closed tape is gzip, which git shows as "gone and back"). So srwr and git are kept apart: `srwr trace` takes the **text** of a diff (`git show`, `git log -p`, any diff) and matches the added lines to the lines the tapes wrote. Nothing in git is run, no hook is added, and the tape and the protocol do not change. A line is matched by what it says, not by time: the time of a commit is when a person ran `git commit`, which says little about when an operation happened. A single short line is not believed (`}` is everywhere); where two operations wrote the same line, the later one is told. What no tape holds is counted, not guessed.

`--as-tape` makes the matched operations a tape that can be replayed, since the text alone is hard to take in. Taking only those operations would break the replay (a line number counts on the edits before it), so the tape holds, for each file, everything between the first and the last of them, preceded by a snapshot. The tape is marked as cut by `author.kind` `derived`, so nothing in the format is new, and `srwr trace` does not count it as a source. The last diff of such a replay is against the file as it is now, so later work shows as a difference; that is left as it is, since it is true.

## The tape and the real files

### Write the real file first, then append to the tape

Even if the process dies in between, the next time srwr touches the files, the mismatch between the real file and the tape shows up as `external`. In the opposite order, an operation would stay on the tape that is not in the real file.

### Only files whose content the tape holds are subject to `external`

A change to a file that was never touched cannot be seen, because there is no base. Holding a base (the hash of every file at the start of a session) would make the tape big. This is accepted as a limit ([limitations.md](limitations.md)).

### The server chooses which kinds of frames are sent

A switch that turns look, edit, external and failure on and off could be done by each client. Then the numbering, the position in the bar, the stepping and the count of new frames in live would be made twice (VSCode and Vim), and could disagree. So the client says which kinds it wants (`kinds`) and the server sends only those, numbered again, so that a client keeps drawing what it is given. A change opens the tape again. The choice is not saved, because srwr has no settings for the look.

### A failed call is written to the tape, and shown only when asked for

A call that failed leaves no trace, so a mistake the AI makes again and again (a path that is absolute, a `why` that is missing) can only be found by asking the AI. The tape now holds it as a `failure`. The viewers show it as a red frame, only when the person turns `failure` on (see the choice of kinds above). The real path is not written when it is absolute, outside the workspace or of a file that is not recorded, since the tape is shared.

### An external change is inserted in a form that shows its content

A label alone does not tell what changed. So `external` and `final` are shown as a side-by-side diff. The label goes in the heading (the tab title). A line shaped like the `why` line cannot be made on a diff screen.

The tape holds an `external` as the lines that changed (`hunks`), not as the whole file, and no `snapshot` follows it. A measured tape was 89% whole texts, most of them the same text twice (an `external` and the `snapshot` after it). The whole text is kept only once per file, at the first touch; the tape still replays alone, because the lines that changed are applied to a text the same tape holds. (These `hunks` of the tape are a different thing from the `hunks` the protocol used to send for a `replace` frame.)

### The view server does not trust the paths written on the tape

So that a shared tape cannot make it read an arbitrary file. When the current file is read for the final diff, a path that points outside the workspace (`..`, an absolute path, a symbolic link that points outside) is not read and is treated as "does not exist".

## How display works

### The tape format is a promise inside Go only

If each editor had its own code to read the tape and build the frames, VSCode and Vim would disagree, and every change of the format would mean fixing everything. So the reading side is gathered in one place, the view server (Go), and the promise with the editors is only the [protocol](../reference/protocol.md). A change of the tape format is absorbed in the server.

### No compatibility promise for the tape format until v1

srwr is still experimental and has no users who depend on old tapes. Keeping every old form readable would stop us from making the tape lighter and simpler. So until v1 the format may change without compatibility. In practice the tapes of versions 1 and 2 are still read, in one place (`tape.Parse`), and [tape.md](../reference/tape.md) says so. From v1 on, the promise in [tape.md](../reference/tape.md) applies: fields may be added, and the meaning of an existing one is not changed.

### Closed tapes are compressed with gzip

About nine tenths of a tape is the whole text that each file had when it was first touched (the `snapshot`). It cannot be dropped: the replay must complete from the tape alone. So the tape is made smaller after the session ends. The tape of a session that ended is compressed with gzip (`<id>.tape.jsonl.gz`), by the call that starts the next session, under the lock.

- gzip is in Go's standard library (no dependency), `zcat` reads it, and the same tape always gives the same bytes, so a shared tape does not change when it is compressed again
- A tape being written stays plain JSONL, so appending, live viewing, and the reading of the last line that is not finished are as they were. Everything that reads tapes opens both forms
- Not chosen: keeping only the part of an unchanged file that was read (it breaks the replay on its own), and compressing each event (it makes the tape unreadable by eye and by `grep`)
- Failing to compress changes nothing else: the plain tape stays and is read as before

### Communication is newline-delimited JSON

The LSP format (with headers) is not handled by Vim as it is. Newline-delimited JSON is handled by Vim's own `job` and a channel in `nl` mode as it is, and VSCode can write it without a library. It is the same format as MCP, so the implementation can be shared.

### One server per client

It is not a shared daemon. What is shared is the tape itself.

### No more external dependencies

Go uses only the standard library (MCP and the view server use our own JSON-RPC, and live uses polling). The VSCode extension has zero runtime dependencies. Vim relies on no other plugin. Distribution is simple (`go install` is enough, no cgo), and there is no need to worry about the vulnerabilities and licenses of dependencies.

### The VSCode extension does not carry the `srwr` binary

The extension (`.vsix`) is distributed alone, and `srwr` is installed separately (`go install` or GitHub Releases), as [cli.md](../reference/cli.md) says. The reasons: whoever lets an AI use srwr installs `srwr` anyway (`srwr mcp` and `srwr hook` are the binary), and Vim uses the same binary, so there is one way to install it. A binary inside the extension could differ in version from the one the AI runs. And a `.vsix` for each OS and CPU (six of them) is avoided. The cost is two steps for a person who only wants to view a tape; the extension tells them how to install `srwr` when it is not found.

### English by default, and time in UTC

What srwr shows is English unless the person switches to Japanese (`SRWR_LANG`; VSCode follows its own display language), and the Japanese version of every document is kept as a pair. What the AI reads (tool descriptions, error messages, the notes of the hook) is English only, whatever the screen. The tape holds UTC, so that a shared tape names the same moment wherever it was written, and every viewer shows it in the time zone of the machine. Tapes written with `+09:00` are read as they are.

## UI decisions

The screen specifications are in [vscode.md](../reference/vscode.md) and [vim.md](../reference/vim.md). This part says why they were decided so.

| Decision | Reason |
|---|---|
| **Viewing is frame-by-frame stepping only.** There is no autoplay, speed or real-time button | The viewer steps one frame at a time while reading the reason. Motion does not help understanding, and adds things to verify |
| **look is blue, edit is orange**, two colors. Only the dots of external and final changes are purple, and a failure is red | A few colors are enough to tell the kinds apart, and red is kept for what went wrong |
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
