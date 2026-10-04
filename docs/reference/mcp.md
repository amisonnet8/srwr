# MCP tools (select, replace)

*[日本語](mcp_ja.md) | **English***

**Readers**: people who use srwr. For people who want to know what the AI is made to do and what errors come back.

The AI agent edits files with only **three tools** provided by the MCP server `srwr mcp`.

| Tool | What it does |
|---|---|
| `select` | Declares the range being looked at. A selection token is returned |
| `replace` | Replaces the range of a selection token with new text |
| `sub` | Replaces a text with another in several files at once, when the number of places is what was expected |

There is no `replace` without `select`. So the tape always holds "look, then change" together. `sub` is for a change that is the same in many places: it asks for the number of places, so the check of what is changed is made by that number. Searching and reading are left to the Read and grep the AI already has (the hook records them; see `srwr hook` in [cli.md](cli.md)).

All three require a `why` (the reason). It is the heart of what a person sees when replaying the [tape](tape.md).

The descriptions of the tools and the error messages are in English, whatever the language of the screen, because they are for the AI to read.

## select

Declares the range being looked at, and returns a **selection token** for editing that range.

```jsonc
// input
{ "file": "cmd/app/main.go", "startLine": 12, "endLine": 14, "why": "Checking whether the main function needs a fix" }
// output
{ "ok": true, "selection": "sel_0410R3GZE4KV11C6325D32S7", "startLine": 12, "endLine": 14, "lines": ["func main() {", "…", "}"] }
```

| Item | Meaning |
|---|---|
| `file` | A path relative to the workspace. Existing files only (a new file cannot be made) |
| `startLine`, `endLine` | Line numbers, 1-based, both inclusive. Give both or neither (with neither, `expect` finds the range) |
| `expect` | Optional. The lines the range must hold, joined with `\n`. See "Checking the content" below |
| `why` | Why it looks here |
| `selection` | The selection token. The AI passes it to `replace` as it is ([what the token is](../design/token.md)) |
| `startLine`, `endLine` in the output | The range that was selected (when `expect` found it, this is where) |
| `lines` | The current content of the range. Always returned |

- **A place to insert**: an empty range with `endLine = startLine - 1` means "just before line `startLine`". For example `startLine: 13, endLine: 12` is between lines 12 and 13. To append to the end of the file, `startLine = number of lines + 1`
- Conditions of the range: `1 ≤ startLine ≤ number of lines + 1`, `startLine - 1 ≤ endLine ≤ number of lines`. Outside them, `invalid_range`
- A path that points outside the workspace (`../`, an absolute path) is also `invalid_range`

### Checking the content (`expect`)

`expect` is the content of the range, line by line: the lines joined with `\n` (counted like `newText`: `""` is 0 lines, and a last `\n` is not counted). Whitespace counts. A part of a line is not searched for, since a range is whole lines.

| Call | What happens |
|---|---|
| `startLine` and `endLine` only | As before |
| `startLine`, `endLine` and `expect` | It passes only when the range holds exactly the lines of `expect`. Otherwise `content_mismatch`. This catches line numbers that have moved |
| `expect` only | srwr looks for the consecutive lines of `expect` in the file. Exactly one place: that is the range. None: `content_not_found`. Two or more: `content_ambiguous` |

- With neither line numbers nor `expect`, or with only one of `startLine` and `endLine`, the call is `invalid_input`. So is an `expect` of `""` without line numbers (a place to insert is pointed at with line numbers)
- `content_mismatch` says where the same lines are in the file (up to 5 places), which is usually the fix. `content_ambiguous` says where they are (up to 10 places): add line numbers, or more lines to `expect`
- `expect` is not written to the tape

## replace

Replaces the range with new text. An insertion is a replacement of an empty range, and a deletion is `newText` as an empty string.

```jsonc
// input
{ "selection": "sel_0410R3GZE4KV11C6325D32S7", "newText": "func main() {\n    setupSignals()\n    …\n}", "why": "Added the missing initialization of signal handling" }
// output
{ "ok": true, "selection": "sel_041GR3RZE4KV0BBH2S177Q36", "startLine": 12, "endLine": 15,
  "lines": ["func main() {", "    setupSignals()", "    …", "}"], "before": ["", "// main starts the app."], "after": ["", "func run() {"] }
```

| Item | Meaning |
|---|---|
| `selection` | The token returned by `select` or by the previous `replace`. The file is decided from it (no `file` is needed) |
| `newText` | The text after the replacement. `""` is a deletion |
| `why` | Why it changes it this way |
| `selection` in the output | A new token for **the range after the replacement**. To go on fixing the same place, it can be used without calling `select` again |
| `lines` in the output | The content of the range after the replacement (`[]` for a deletion) |
| `before`, `after` in the output | Up to 2 lines right before and right after that range (fewer near the start or the end of the file, `[]` if none). With them the result can be checked without reading the file again |

**How the lines of `newText` are counted**: `""` is 0 lines (a deletion). Otherwise it is split into lines at `\n`, and if it ends with `\n` the last empty element is not counted (`"x\n"` is 1 line, `"\n"` is one empty line). The range of the returned token follows this count.

**The AI does not have to calculate line numbers.** If edits elsewhere shift the lines, srwr corrects them. Only when an edit overlaps the range does it become `selection_stale`.

Files that cannot be handled: files with line breaks other than LF (CRLF), and binary files. They give `unsupported_file`.

## sub

Replaces a text with another in several files at once, like a simple sed, and records why. The text is searched for as it is (not a regular expression), from left to right in each file, and places do not overlap. **`count`, the number of places expected in all the files together, is required: if the number found is different, nothing is changed.**

```jsonc
// input
{ "files": ["a.go", "b.go"], "old": "oldName(", "new": "newName(", "count": 3, "why": "Rename the helper to the new naming rule" }
// output
{ "ok": true, "count": 3, "files": [
  { "file": "a.go", "hits": 2, "startLine": 12, "endLine": 30, "selection": "sel_041G417ZRKYSPWJ7X4Z5PPKP", "lines": ["…"] },
  { "file": "b.go", "hits": 1, "startLine": 7, "endLine": 7, "selection": "sel_042020AD5CH4C75M7DH075CM", "lines": ["…"] } ] }
```

| Item | Meaning |
|---|---|
| `files` | Paths relative to the workspace. Existing files only. A path is given once |
| `old` | The text to look for. Not empty. It may have several lines (LF) |
| `new` | The text to put in its place. `""` deletes it |
| `count` | How many places you expect in all the files together. 1 or more |
| `why` | Why it changes them |
| `files` in the output | Only the files that changed. `hits` is the number of places in the file; `startLine` and `endLine` are the lines from the first place to the last, after the change; `selection` is a token for those lines (usable by `replace`); `lines` is their content |

- If the number found is not `count`, the error is `count_mismatch`. Its message and `actual` say how many places each file has (`{"a.go": 3, "b.go": 1}`), and **no file is changed and nothing but the `failure` is written to the tape**
- Every file is checked as `select` checks it (not recorded, CRLF, binary, outside the workspace). If one of them cannot be used, nothing is changed
- A change that cannot be told in lines (it adds or removes the final line break of the file) is `invalid_input`: use `select` and `replace` for it
- On the tape there is one `replace` for each file that changed, with the same `why` ([tape.md](tape.md#replace)). A viewer shows each as a diff of the file, with the `why` above it
- A regular expression is not supported. Use `select` and `replace` when the places must be chosen one by one

## why

- **Required in all three** (`select`, `replace` and `sub`). Blank is not allowed either (`invalid_input`)
- In `select` it is "why it looks here", in `replace` "why it changes it this way". Write the reason in one sentence, not a rephrasing of what is being done
- Write it in **the same language as the conversation with the user** (the descriptions of the tools and the input schema ask for this). It is for people to read

## Errors

In the MCP response `isError` is `true`, and the body is the following JSON.

```json
{"ok": false, "error": {"code": "selection_stale", "message": "…", "actual": ["…"]}}
```

| Code | Meaning | What the AI should do |
|---|---|---|
| `invalid_selection` | The form of the token is wrong, or it was altered. A token issued on another tape (another session) also gives this. So does a new session started after a gap of 30 minutes | Call `select` again |
| `selection_stale` | An edit that overlaps the range was made after the token was issued | Call `select` again |
| `selection_mismatch` | Even with the line numbers corrected, the content of the range differs from when `select` was called (it may have been changed outside srwr) | Check the content and call `select` again |
| `file_not_found` | The target file does not exist, or is not a regular file (a directory, for example) | — |
| `invalid_range` | The line numbers are outside the file, or the path is outside the workspace | Check the number of lines and call `select` again |
| `content_mismatch` | The range holds other lines than `expect` | Read where the message says the lines are, and call `select` again |
| `content_not_found` | `expect` (given without line numbers) is not in the file | Check the content, or give line numbers |
| `content_ambiguous` | `expect` (given without line numbers) is in the file in more than one place | Give line numbers, or more lines in `expect` |
| `count_mismatch` | `sub` found a number of places other than `count` | Read how many each file has, and call `sub` again with the right `count` (or use `select`) |
| `ignored_file` | `select`, `replace` or `sub` was used on a file that is not recorded | srwr cannot handle it. Ask the user |
| `invalid_input` | A required input is missing, has the wrong type, or `why` is empty; `file` is empty or has a NUL; only one of `startLine` and `endLine` is given, or none of them and no `expect`; `selection` is blank; `newText` has a CR; for `sub`, `files`, `old` or `count` is missing or empty, a path is given twice, `old` or `new` has a CR | Fix the input |
| `unsupported_file` | CRLF or binary | — |
| `internal_error` | An I/O error and the like | — |

An error may carry the current content (`actual`). What it holds is decided for each code.

| Code | `actual` |
|---|---|
| `selection_mismatch` | The current content of the corrected range (an array of lines) |
| `content_mismatch` | The current content of the range (an array of lines) |
| `selection_stale` | The current content of the range, corrected up to just before the overlapping edit (kept inside the file) |
| `count_mismatch` | The number of places in each file (`{"a.go": 3, "b.go": 1}`) |
| `invalid_range` | `{"lineCount": number of lines}`. None for a path outside the workspace |
| Others | None |

A failed call is also written to the tape as a [`failure`](tape.md#failure), so that the mistakes the AI makes can be read later. The real path is left out when it is absolute or outside the workspace.

## The order of processing

When srwr receives a `replace`, it works in this order.

1. Decode and verify the token (a failure is `invalid_selection`)
2. Find the file
3. Detect changes to files made outside srwr (if any, record them as [`external`](tape.md#external))
4. Correct the line numbers (an overlapping edit gives `selection_stale`)
5. Check the content (a mismatch gives `selection_mismatch`)
6. **Write the real file first, then append to the tape**

The file to detect is learned from the token, so decoding comes first. With a forged token, no external change is recorded. For `select`, the order is: the check of the path, the files that are not recorded and the kind of file (`invalid_range`, `ignored_file`, `file_not_found`, `unsupported_file`), then detection, then the check of the range (`invalid_range`), then the check of the content (`content_mismatch`, `content_not_found`, `content_ambiguous`), then recording.

Even if the process dies in between, the next time srwr touches the files, the mismatch with the real file shows up as `external`.

## Mistakes an AI tends to make

These are the mistakes seen when an AI used the tools. Each is an ordinary error with a code, and a failed call is written to the tape as a [`failure`](tape.md#failure).

- **Give `file` as a path relative to the workspace.** An absolute path such as `/home/me/app/main.go` is `invalid_range`, and so is `../main.go`. Write `cmd/app/main.go`.
- **An empty range is easy to place one line off.** `endLine = startLine - 1` means "just before line `startLine`", so `startLine: 13, endLine: 12` is between lines 12 and 13. Read the lines on both sides of the place first, and check the numbers before calling `select`.
- **The line numbers of a new `select` are the numbers of the file now.** srwr corrects the token it has already issued when another edit moves the lines, but not the `startLine` and `endLine` of a new `select`. After other edits, read the file again, or check the returned `lines` (and, after a `replace`, `before` and `after`). Better: pass `expect` with the lines you mean, and a wrong number is refused instead of selecting the wrong place.
- **A new file cannot be made with `select` and `replace`.** `select` gives `file_not_found`. Make it with a shell command. The file is then recorded as an [`external`](tape.md#external) with `created: true`.
- **To change the same place again, use the new token that `replace` returned.** The token you used is spent: using it again gives `selection_stale`.
- **For the same change in many places, use `sub` with `count`**, not many `select` and `replace`. A wrong `count` is refused with the number each file has.
- **Read the error.** `invalid_range` returns `lineCount` (the number of lines of the file), which is enough to correct the numbers.

## Related

- What is recorded: [tape.md](tape.md)
- The command line and installation: [cli.md](cli.md)
