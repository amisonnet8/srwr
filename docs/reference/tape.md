# The tape

*[日本語](tape_ja.md) | **English***

**Readers**: people who share tapes, and people who make tools that read tapes. For how to replay, see [vscode.md](vscode.md) and [vim.md](vim.md).

A **tape** is the series of operations srwr records. It is an append-only JSONL file (one event per line), kept at `.srwr/tapes/<id>.tape.jsonl`. **The tape alone can rebuild the replay completely** (however the real files change afterwards). To share, hand over the tape itself. The person who receives it opens it in their own editor.

## File name and session

- The file name is `<date and time>-<short ID>.tape.jsonl` (for example `20260929-0237-1359`). The date and time are **UTC**. The name without `.tape.jsonl` is the **tape ID**. The short ID at the end (for example `1359`) is the same as `session` in the `header`
- A tape is **made when the first operation is recorded**. If nothing is done, no empty tape is left
- One tape corresponds to one unit of work (a **session**)

The current session is the one `.srwr/active` (the current tape ID) of the workspace points to. **Even if several `srwr mcp` are started, they write to the same tape if the workspace is the same** (writes take turns using `.srwr/lock`. What another process wrote is read from the tape before writing). A new session (a new tape) starts in any of these cases:

1. There is no current session (no `.srwr/active`, or the tape it points to is missing)
2. A set time (30 minutes by default) has passed since the last event of the current session
3. The user ran `srwr tapes new`

`srwr hook` also writes to the same session (the same tape) as `srwr mcp`.

## Rules of writing

- Every event has `"v":1` (the version of the format)
- Append only. Existing lines are never rewritten or deleted
- `seq` is a sequence number that starts at 1 within the tape and has no gaps (the `header` has none)
- `ts` is RFC 3339 **in UTC**, with milliseconds and a trailing `Z` (`2026-09-29T02:20:04.123Z`). Tapes written by older versions have an offset such as `+09:00` (the time zone of the machine then); they are read as the same moments, and an old line is never rewritten
- A field with no value (`why`, `selection`, `from` and so on) is written as `null`, not left out. The exceptions are the optional fields `source` and `tool` (left out when empty) and `deleted` (written only when true)
- One event per line. A last line that does not end with a line break is treated as being in the middle of being written
- A reader ignores fields it does not know
- **Until v1, the tape format may change without compatibility.** A tape written by one version is not promised to be read by another. From v1 on, fields may be added, but the meaning of an existing one is not changed

## Events

### header

One line at the top of the tape.

```json
{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T02:20:00.000Z","author":{"kind":"ai","name":"claude"}}
```

It also has `vcs` and `tool` (`{"name":"srwr","version":"…"}`).

`vcs` is the state of git when the tape was made (when the first record of the session was written). It is not rewritten even if the state changes later.

```json
"vcs":{"type":"git","head":"3f2a… (40 hex digits)","dirty":true}
```

- `head`: the commit of HEAD. `null` in a repository that has no commit yet
- `dirty`: whether there are changes that are not committed. Changes, staging and deletions of tracked files, and new files that `.gitignore` does not ignore. Only below the workspace is looked at. **Anything inside `.srwr/` is not counted** (the tape itself would always make it dirty)
- When the workspace is not under git, or git cannot be used (not installed, refused, or not finished in 5 seconds), it is `null`. The AI's work is not stopped
- Neither the branch name nor the URL of the remote is written (tapes are shared). A reader ignores items of `vcs` it does not know

### snapshot

The **whole text** of a file. It is recorded when the file is first touched in the session, and when a file that an `external` removed comes back. After that the file is followed by `replace` and `external` events only, which hold just the changed lines. Replay is built by applying them in order from the last `snapshot`.

```json
{"v":1,"seq":1,"ts":"…","type":"snapshot","file":"cmd/app/main.go","fileHash":"a3f09c21","text":"package main\n…","sha":"sha256:…"}
```

### select

```json
{"v":1,"seq":2,"ts":"…","type":"select","file":"cmd/app/main.go","startLine":12,"endLine":14,"why":"Checking whether the main function needs a fix","selection":"sel_0410R3GZE4KV11C6325D32S7","source":"mcp"}
```

A `select` recorded by the hook (Read and the like) has a `why` of `null`. It has `source` (`mcp` or `hook`), and for the hook the name of the original tool, `tool` (`Read`, `Bash`, `Grep`, `Edit`). It has no selection token, and `selection` is `null` too. An old tape without `source` is read as `mcp`.

### replace

```json
{"v":1,"seq":3,"ts":"…","type":"replace","file":"cmd/app/main.go","from":"sel_0410R3GZE4KV11C6325D32S7","startLine":12,"endLine":14,"oldText":"…","newText":"…","newStartLine":12,"newEndLine":15,"selection":"sel_041GR3RZE4KV0BBH2S177Q36","why":"Added the missing initialization of signal handling","fileShaBefore":"sha256:…","fileShaAfter":"sha256:…","source":"mcp"}
```

- `from` is the selection token that was given, and `selection` is the one returned. Following `from` → `selection` shows the **lineage**: which `select` a `replace` came from
- `startLine` and `endLine` are the real range after correction
- `oldText` and `newText` are the lines of the range joined with `\n` (without a trailing line break). A deletion has `newEndLine = newStartLine - 1` and an empty `newText`. One empty line has `newEndLine = newStartLine` and an empty `newText` too, so the number of lines is read from `newStartLine` and `newEndLine`
- The `seq` inside a selection token is the `seq` of the event that issued the token
- A `replace` recorded by the hook (Edit) has `from`, `selection` and `why` of `null`, `source` of `hook` and `tool` of `Edit`. The range is the whole lines that contain the replaced place

### external

Recorded when a change to a file made outside srwr is detected. No `snapshot` follows it: the event itself holds what changed.

```json
{"v":1,"seq":4,"ts":"…","type":"external","file":"cmd/app/main.go","author":{"kind":"external"},"detectedBy":"select","expectedSha":"sha256:…","actualSha":"sha256:…","hunks":[{"startLine":3,"endLine":3,"newText":"b","newStartLine":3,"newEndLine":3},{"startLine":40,"endLine":41,"newText":"x\ny","newStartLine":40,"newEndLine":41}]}
```

- `hunks`: **the lines that changed**, against the content the tape held before (the one `expectedSha` is the hash of). The places come from top to bottom and do not overlap. `startLine` and `endLine` are the lines before, `newText` the lines after (joined with `\n`), and `newStartLine` and `newEndLine` their numbers, as in `replace`. An insertion has `endLine = startLine - 1`; a deletion has an empty `newText` and `newEndLine = newStartLine - 1`. Applying them to the content before gives the content after, which `actualSha` is the hash of. It is replayed as a diff frame (a side-by-side diff)
- `text`: **the whole text of the file after the change**, written instead of `hunks` when the change cannot be told as lines (only the line break at the end of the file changed) or is too big to compare. When the file was gone it is `null`, `actualSha` is an empty string, and `deleted: true` is added
- `created`: written (as `true`) when the file is **new**: the tape held no content of it, and a shell command made it. `expectedSha` is an empty string and `hunks` (or `text`) holds the whole file, so it is replayed as a diff frame with an empty left side. Only the hook finds these (see `srwr hook` in [cli.md](cli.md))
- `detectedBy`: what led to the detection (`select`, `replace`, `hook`)
- `author.kind` is always `external` (srwr cannot know who changed it)
- An `external` of the old forms can be read too: one with the whole `text` and a `snapshot` after it, and one without `text` (then the `snapshot` right after it is shown as the content after the change)

**What can be detected**: an `external` happens only when a file for which **the tape already holds the content (a `snapshot`)** later differs. A file first touched in the session has its content at that time as its first `snapshot`. A change to a file that is never touched cannot be seen, except that after a Bash command a **new** file in a git work tree is recorded as an `external` with `created`.

If a `replace` is made with a selection token issued before an `external`, the token is followed in the usual way (line numbers are corrected only for the edits srwr made itself). If the external change did not change the range or the number of lines above it, the token still works. If it did, the content check gives `selection_mismatch`. srwr does not estimate the shift of lines from the content of the external change.

## The selection token

The `sel_…` string that `select` returns. It goes into `from` and `selection`. The AI only passes it on and need not know what is inside.

- **Valid only within the tape.** One issued on another tape (another session) gives `invalid_selection`. When the session changes (after a gap of 30 minutes, for example), the tokens of the previous session cannot be used
- The format and the verification are described in [token.md](../design/token.md), for developers

## Notes when sharing

- A tape holds the **whole text of files**. Files that contain secrets are not recorded (see "Files that are not recorded" in [cli.md](cli.md)). Check the content before sharing
- The key (`.srwr/key`) is not needed for replay. Do not share it. Neither the view server nor the editors read the key
