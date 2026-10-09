# The protocol of the view server

*[日本語](protocol_ja.md) | **English***

**Readers**: people who want to make srwr's display work in a new editor (IDE). VSCode and Vim replay tapes with this promise alone.

**Reading the tape and building the data needed for stepping is the job of the view server (`srwr view-server`).** The editor side only has to draw the data it receives from the server. **It need not know the format of the tape, and it neither reads nor writes the tape or the real files.** It does not read the key (`.srwr/key`) either.

| View server (Go) | Client (editor) |
|---|---|
| Lists and reads tapes | Starts and ends the server |
| **Builds the frames** | **Draws** (the color of the range, the `why` line, the diff screen, the sidebar, the status) |
| The **content of the document** at each frame | Inserts the `why` line into the document |
| Before and after of a diff frame, the final diff (the comparison with the current file) | Stepping, key operations |
| Receives a setting (whether to show diff frames) | Passes the setting (diff frames) to the server (VSCode and Vim send a fixed value) |
| Live: watches the current tape and notifies of appended frames | Live: receives the notifications and draws |

## Flow

What a client does.

1. Starts `srwr view-server --root <workspace>` as a child process
2. Sends `initialize` (a request before it gets `not_initialized`)
3. **Replay**: shows a list with `tapes/list` → when one is chosen, `tape/open` (the frames come back) → each time it moves to a frame, `frame/state` (the content of the document at that frame) → `tape/close` when closing
4. **Live**: `live/start` (the frames of the current tape so far come back, and watching begins) → each time something is appended, `live/frame` arrives from the server → `live/stop` to stop
5. Sends `shutdown` when finishing

One server process is started per client (there is no shared daemon. What is shared is the tape itself).

## Communication

- **JSON-RPC 2.0 over stdio.** **One message = one line** (newline-delimited JSON). There are no headers like those of LSP
- Standard error is a log. A client need not show it
- It is newline-delimited so that it can be handled with no library, both with the standard features of Vim (`job` and a channel in `nl` mode) and with a child process in VSCode

## Methods

The arguments are a JSON object (`{}` if none).

| Method | Kind | Arguments → result |
|---|---|---|
| `initialize` | request | `{client:"vscode"\|"vim", protocolVersion:3, options:{diffFrames}}` → `{serverVersion, protocolVersion:3}`. The default of `options` has `diffFrames` as `true`. Unknown `options` are ignored |
| `tapes/list` | request | `{}` → `{tapes:[TapeInfo…]}`. Only tapes that have one or more operations, newest first (by the time the tape started; a tape without a header by its last update). Tapes that cannot be read are not listed |
| `tape/open` | request | `{tapeId, withText?:false, kinds?}` → `{tapeId, frames:[Frame…], hidden?}`. The frames. If `diffFrames` is true, the **final diff** (`final`), compared with the current file of the workspace, is included at the end. Opening the same `tapeId` again reads it again |
| `frame/state` | request | `{tapeId, index, file?}` → `{before, after, content}`. The before and after of the frame at `index` (both `""` when `index` is −1). `content` is the content of `file` (the file of that frame if omitted) at the time that frame has finished. `null` for a file no frame has touched |
| `tape/close` | request | `{tapeId}` → `{}` |
| `live/start` | request | `{withText?:false, kinds?}` → `{tapeId:string\|null, frames:[Frame…], hidden?}`. The **frames so far** of the current tape (the one with the newest update time) (the client does not show them, only lists them). Watching begins after the reply |
| `live/hidden` | notification (server → client) | `{tapeId, hidden}`. A frame of a kind the client did not ask for arrived, so the count of hidden frames changed |
| `live/frame` | notification (server → client) | `{tapeId, frame:Frame}`. An appended frame. When `tapeId` differs from before (it moved to another tape), the client rebuilds its list. The frames of that tape are sent from the beginning |
| `live/stop` | request | `{}` → `{}`. Stops watching |
| `shutdown` | request | `{}` → `{}`. The server exits after writing the reply |

**`tapeId`**: the file name of the tape without `.tape.jsonl` (for example `20260929-0237-1359`). Only the characters `0-9 A-Z a-z - _ .` are accepted and it must not start with `.`; anything else (such as `/`) gives `invalid_params`.

**TapeInfo**: `{tapeId, startedAt, updatedAt, ops, files}`. `startedAt` is the value of the header (`""` if there is no header), `updatedAt` is the last update time of the tape's file (RFC 3339 in UTC with milliseconds, ending in `Z`), `ops` is the number of `look`, `edit` and `external`, and `files` are the files touched (in the order first touched). `startedAt` of a tape written by an older version may have an offset such as `+09:00`; it is the same moment. A client shows these times in the time zone of the machine.

## Frames

**The contract is "the frames" and "the content of the document at each frame".** A client draws by looking at only these.

| Kind (`kind`) | Made from | Information it holds (the main points) |
|---|---|---|
| `look` | A `look` of the tape (whether `source` is `mcp` or `hook`) | File, range, `why` (may be `null`), `seq`, lineage (`selection`) |
| `edit` | An `edit` of the tape | File, the range and text before and after, `why` (may be `null`), `seq`, lineage (`from` → `selection`) |
| `replace` | A `replace` of the tape: one file, all its places | File, the range and the text before and after, `why`, `seq`, `hits` (the number of places), `hunks`. Shown as a diff, like `external`, with the `why` in a band above each block of changed lines |
| `new` | A `new` of the tape: a whole new file | File, the range (the whole file) and the text after (before is empty), `why`, `seq`. Shown like an `edit`: one editor, the file painted orange, the `why` above it |
| `external` | An `external` of the tape | File, before (the content just before) and after (`text`), whether it was deleted |
| `final` | The last content of the tape compared with the current file | File, before (the end of the tape) and after (the current file), whether it no longer exists |
| `failure` | A `failure` of the tape (a `look`, `edit`, `replace` or `new` that gave the AI an error) | `tool`, `code`, `message`, `why`; `file` is `""` when it is not known or is left out; `range` is the range given to a `look` (`{start:0,end:-1}` when there is none). The text before and after is empty |

The fields of a Frame:

| Field | Content |
|---|---|
| `index` | The position in the list, starting from 0 |
| `kind` | `look`, `edit`, `replace`, `new`, `external`, `final`, `failure` |
| `seq`, `ts` | The `seq` of the tape, and the time (epoch milliseconds; the value of the frame before if it cannot be read, 0 for the first). `final` has the value of the last frame |
| `file` | A path relative to the workspace (separated by `/`) |
| `range` | `{start, end}`. The range on the "after" side (`look` = that range, `edit`, `replace` and `new` = the new range, `external` and `final` = the whole file). `end < start` is an empty range |
| `oldRange` | `edit` only. The range on the "before" side |
| `why`, `selection`, `from` | A string or `null` |
| `parent` | The parent in the lineage (the `index` of the frame `from` points to), or `null` |
| `tool`, `code`, `message` | `failure` only: the tool (`look`, `edit`, `replace`, `new`), the error code, and the message (the real path is left out; see [tape.md](tape.md#failure)) |
| `hits` | `replace` only. The number of places it changed in the file (not output otherwise) |
| `hunks` | `replace` only. The blocks of changed lines, top to bottom: `[{beforeStart, beforeEnd, afterStart, afterEnd}]` (1-based, inclusive, lines of the text before and after). A block is changed lines that follow each other, so places with an unchanged line between them are two blocks. An insertion has `beforeEnd = beforeStart - 1`, a deletion `afterEnd = afterStart - 1`. The client puts the `why` above each block. When the texts differ by too much to compare, the whole change is one block (not output otherwise) |
| `deleted` | Diff frames only. The file does not exist after the change (`true` only then; not output otherwise) |
| `before`, `after` | **Only when `withText` is true.** The whole text before and after. Usually it is fetched with `frame/state` (so that not every frame carries the whole text in a big tape) |

- **Which kinds are sent (`kinds`)**: `tape/open` and `live/start` take `kinds`, a list of `look`, `edit`, `external`, `failure`. The server sends only those, **numbers the frames from 0 again** (so `index` is the position in what is sent, and `frame/state` takes that `index`), and tells how many it left out: `hidden` is an object such as `{"failure": 2}` (a kind with none left out is not in it; when nothing is left out, `hidden` is not there). `final` follows `external`, and `replace` and `new` follow `edit`. Left out, `kinds` is `["look","edit","external"]`: `failure` frames are not sent unless asked for. An unknown name is `invalid_params`. An empty list sends nothing. To change what is shown, a client opens the tape again with other `kinds`. `frame/state` is not affected by what is left out: the content of a file after a frame includes the frames that are not sent
- The frames of live (`live/start`, `live/frame`) do not include the final diff (an `external` appears as written on the tape)
- **The server decides the final diff.** The client only shows it
- Even if items are added to the tape (`source`, `tool`, `vcs` and so on), the client need not use them
- **The fields VSCode and Vim use** are only `index`, `kind`, `file`, `range`, `why`, `before`, `after`, `deleted`, `hits` and `hunks` (for `replace`), and for `failure` `tool`, `code` and `message`, and `seq` (to find the nearest frame again when the kinds that are shown are changed). They do not use `ts`, `selection`, `from`, `parent` or `oldRange`. Live also uses the text (`before`, `after`), so pass `withText: true` to `live/start`

## Behavior of the server

- When reading the current file for the final diff, a path that points outside the workspace (`..`, an absolute path, a symbolic link that points outside) is not read and is treated as "does not exist". This is so that a shared tape cannot make it read an arbitrary file
- The live watch polls the size of the tape (the interval is 200 milliseconds). A last line that does not end with a line break is held back until the next read
- The tapes are at `<root>/.srwr/tapes/`
- The server only **reads** tapes. It takes no lock and writes nothing
- The messages of errors are English, for developers. A client that shows an error to a person says the ones a person can meet (`tape_not_found`, `tape_unreadable`) in its own words, from the code

## Versions and errors

- `protocolVersion` is an integer. If it does not match in `initialize`, the server returns an error. The client shows words that make clear that "srwr and the editor side do not match"
- Errors come back as JSON-RPC errors. The error code of srwr goes in `error.data.code`

| `data.code` | `code` of JSON-RPC | Meaning |
|---|---|---|
| `protocol_mismatch` | −32000 | The `protocolVersion` of `initialize` does not match |
| `not_initialized` | −32000 | A request came before `initialize` |
| `tape_not_found` | −32000 | There is no tape with that `tapeId` (not open, or does not exist) |
| `tape_unreadable` | −32000 | The tape cannot be read |
| `invalid_params` | −32602 | A mistake in the arguments (type, range of `index`, an invalid `tapeId`, no `protocolVersion` in `initialize`) |
| (none) | −32601 | An unknown method |

## Compatibility

- Adding is fine. **An item that old clients can ignore without breaking is added without raising `protocolVersion`.** Raise it when changing or removing the meaning of an existing item
- A client ignores fields it does not know

## When you make a supported editor

- The standard for how to draw is [vscode.md](vscode.md). Show each kind of frame (the color of the range, the `why` line, the diff frame) with the same information in the same order
- Examples of working exchanges are in [the examples](../examples/look-edit.md)
