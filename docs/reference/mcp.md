# MCP tools (look, edit, replace, new)

*[日本語](mcp_ja.md) | **English***

**Readers**: people who use srwr. For people who want to know what the AI is made to do and what errors come back.

The AI agent edits files with only **four tools** provided by the MCP server `srwr mcp`.

| Tool | What it does |
|---|---|
| `look` | Looks at a range. A selection token is returned |
| `edit` | Changes the range of a selection token to new text |
| `replace` | Replaces a text with another in several files at once, when the number of places is what was expected |
| `new` | Creates a file that does not exist yet, with its content |

There is no `edit` without `look`. So the tape always holds "look, then change" together. `replace` is for a change that is the same in many places: it asks for the number of places, so the check of what is changed is made by that number. Searching and reading are left to the Read and grep the AI already has (the hook records them; see `srwr hook` in [cli.md](cli.md)).

All four require a `why` (the reason). It is the heart of what a person sees when replaying the [tape](tape.md).

The descriptions of the tools and the error messages are in English, whatever the language of the screen, because they are for the AI to read.

## look

Looks at a range, and returns a **selection token** for editing that range with `edit`.

```jsonc
// input
{ "file": "cmd/app/main.go", "startLine": 12, "endLine": 14, "why": "Checking whether the main function needs a fix" }
// output
{ "ok": true, "selection": "sel_0410R3GZE4KV11C6325D32S7", "startLine": 12, "endLine": 14, "lines": ["func main() {", "…", "}"] }
```

| Item | Meaning |
|---|---|
| `file` | A path relative to the workspace. Existing files only (make a new file with `new`) |
| `startLine`, `endLine` | Line numbers, 1-based, both inclusive. Give both or neither (with neither, `expect` finds the range) |
| `expect` | Optional. The lines the range must hold, joined with `\n`. See "Checking the content" below |
| `why` | Why it looks here |
| `selection` | The selection token. The AI passes it to `edit` as it is ([what the token is](../design/token.md)) |
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

## edit

Changes the range to new text, in one call. An insertion is a change of an empty range, and a deletion is `newText` as an empty string. The range is pointed at in one of two ways: with the **selection token** from `look` (the usual way: look, then edit), or with **`file` and `expect`**, with no token.

```jsonc
// input
{ "selection": "sel_0410R3GZE4KV11C6325D32S7", "newText": "func main() {\n    setupSignals()\n    …\n}", "why": "Added the missing initialization of signal handling" }
// output
{ "ok": true, "selection": "sel_041GR3RZE4KV0BBH2S177Q36", "startLine": 12, "endLine": 15,
  "lines": ["func main() {", "    setupSignals()", "    …", "}"], "above": ["", "// main starts the app."], "below": ["", "func run() {"] }
```

| Item | Meaning |
|---|---|
| `selection` | The token returned by `look`, `edit` or `new`. The file is decided from it (no `file` is needed). Give this, or `file` and `expect`; not both |
| `file`, `startLine`, `endLine`, `expect` | Without a token: the file, the range as the AI saw it (both line numbers, or neither) and the lines that range holds now, joined with `\n` (as in `look`). See below |
| `newText` | The text after the replacement. `""` is a deletion |
| `insert` | Optional, `"after"` or `"before"`. Keep the range (the token's, or the lines of `expect`) and put `newText` after (before) it, instead of replacing it |
| `why` | Why it changes it this way |
| `selection` in the output | A new token for **the range after the replacement**. To go on fixing the same place, it can be used without calling `look` again |
| `lines` in the output | The content of the range after the replacement (`[]` for a deletion) |
| `above`, `below` in the output | Up to 2 lines of the file as it is now, right above and right below the new range (fewer near the start or the end of the file, `[]` if none). They are **not** the old content: what was replaced is not returned (the AI has just given it as `expect`). With them the result can be checked without reading the file again |

**How the lines of `newText` are counted**: `""` is 0 lines (a deletion). Otherwise it is split into lines at `\n`, and if it ends with `\n` the last empty element is not counted (`"x\n"` is 1 line, `"\n"` is one empty line). The range of the returned token follows this count.

**The AI does not have to calculate line numbers.** If edits elsewhere shift the lines, srwr corrects them. Only when an edit overlaps the range does it become `selection_stale`.

### Without a token (`file` and `expect`)

`expect` is required, because it is what makes the call safe: the range is never taken for its line numbers alone. (One case needs no `expect`: an insertion, below.) srwr decides the range in this order:

1. the line numbers as given, if that range holds the lines of `expect`;
2. those line numbers moved to where the lines are now, following the `edit`, `replace` and `new` made on the file **after its last look** (a `look`, or a read of the hook), if that range holds the lines of `expect`;
3. otherwise, the one place where the lines of `expect` are in the file.

Zero places is `content_not_found` (`actual` holds the lines at the given line numbers, if there were any). Two or more places, or 1 and 2 pointing at different places, is `content_ambiguous`, which says where: give line numbers, or more lines in `expect`. So edits to one file can be sent together, in any order, even in parallel: each is found wherever the earlier ones moved it.

An **insertion** (`endLine = startLine - 1`, no `expect`) has no lines to check. It is accepted only if the file has not changed since its last look (no `edit`, `replace`, `new` or `external` after it), and the line numbers are then taken as given. Otherwise it is `content_not_found`: look again, or point at the line next to the place with `expect` and `insert`.

**`insert`** (`"after"` or `"before"`) inserts next to lines without needing them to be unchanged since a look: point at the lines as usual (a token, or `file` and `expect`, which is checked), and they are kept while `newText` goes just after (before) them. The result (`selection`, `startLine`, `endLine`, `lines`, `above`, `below`) is about the new lines, and the tape holds an `edit` of an empty range, as for any insertion. `insert` is `invalid_input` with an empty range (there are no lines to point at), with an `insert` other than `"after"` and `"before"`, and with an empty `newText`.

The tape holds the same `edit` as for a token, with `from` of `null`. `expect` is not written to the tape.

Files that cannot be handled: files with line breaks other than LF (CRLF), and binary files. They give `unsupported_file`.

## replace

Replaces a text with another in **2 or more places**, in one file or several, like a simple sed, and records why. For one place, use `edit`: `replace` is refused for it (`use_edit`, below). The text is searched for as it is (not a regular expression), from left to right in each file, and places do not overlap. **`count`, the number of places expected in all the files together, is required and is 2 or more: if the number found is different, nothing is changed.**

```jsonc
// input
{ "files": ["a.go", "b.go"], "old": "oldName(", "new": "newName(", "count": 3, "why": "Rename the helper to the new naming rule" }
// output
{ "ok": true, "count": 3, "files": [
  { "file": "a.go", "count": 2, "hits": [
      { "startLine": 12, "endLine": 12, "lines": ["…"], "above": ["…"], "below": ["…"] },
      { "startLine": 30, "endLine": 31, "lines": ["…", "…"], "above": [], "below": ["…"] } ] },
  { "file": "b.go", "count": 1, "hits": [ { "startLine": 7, "endLine": 7, "lines": ["…"], "above": ["…"], "below": ["…"] } ] } ] }
```

| Item | Meaning |
|---|---|
| `files` | Paths relative to the workspace. Existing files only. A path is given once |
| `old` | The text to look for. Not empty. It may have several lines (LF) |
| `new` | The text to put in its place. `""` deletes it |
| `count` | How many places you expect in all the files together. 2 or more. A `1` is answered with `use_edit` when the text is in one place |
| `why` | Why it changes them |
| `files` in the output | Only the files that changed. `count` is the number of places in the file. `hits` has one entry for each place, as the file is now: `startLine` and `endLine`, `lines` (what they hold), and `above` and `below` (the one line above and the one line below, `[]` if none). Places on the same line are one entry. At most 20 entries are listed for a file, and `more` is how many were left out. **There is no selection token**: to go on with a place, use `look` |

- **One place is for `edit`.** With a `count` of 1 and the text in exactly one place, the error is `use_edit`, and nothing is changed. Its message says where the place is (`a.go line 12`), without any of the file. `actual` has `hits` (`file`, `startLine`, `endLine`, `lines`: where the place is now) and `edit` (`file`, `startLine`, `endLine`, `expect`, `newText`: the `edit` call that makes the same change, to which only `why` is added)
- If the number found is not `count` (a `count` of 1 with no place, or with two or more, too), the error is `count_mismatch`. Its message and `actual` say how many places each file has (`{"a.go": 3, "b.go": 1}`), and **no file is changed and nothing but the `failure` is written to the tape**
- Every file is checked as `look` checks it (not recorded, CRLF, binary, outside the workspace). If one of them cannot be used, nothing is changed
- A change that cannot be told in lines (it adds or removes the final line break of the file) is `invalid_input`: use `look` and `edit` for it
- On the tape there is one `replace` for each file that changed, with the same `why` ([tape.md](tape.md#replace); its `selection` is `null`, since no token is returned). A viewer shows each as a diff of the file, with the `why` above it
- A regular expression is not supported. Use `look` and `edit` when the places must be chosen one by one

## new

Creates a file that does not exist yet, with its content, and records why.

```jsonc
// input
{ "file": "internal/config/config.go", "content": "package config\n\nfunc Read() {}", "why": "Start the config package" }
// output
{ "ok": true, "selection": "sel_041G417ZRKYSPWJ7X4Z5PPKP", "startLine": 1, "endLine": 3 }
```

| Item | Meaning |
|---|---|
| `file` | Path relative to the workspace. The file must not exist |
| `content` | The content of the file. Line breaks are LF (a CR is `invalid_input`). `""` makes an empty file |
| `why` | Why it creates the file |
| `selection` in the output | A token for the whole content (usable by `edit`). `startLine` and `endLine` are its lines (`endLine` is 0 for an empty file). The content is not returned: the AI has just written it |

- If the file already exists, the error is `file_exists`, and nothing is changed. Use `look` and `edit` to change a file
- Directories above the file are created when they are missing. A link in the way that leads out of the workspace, or to a place that is not recorded, is refused (`invalid_range`, `ignored_file`), and nothing is made
- The file always ends with a line break, whether `content` does or not (`"a"` and `"a\n"` make the same file)
- A file that is not recorded (`.env` and the like) cannot be made (`ignored_file`)
- On the tape it is one `new` ([tape.md](tape.md#new)). A viewer shows it as the whole file, painted like an `edit`, with the `why` above it
- Making a file with a shell command still works; it is then recorded as an [`external`](tape.md#external) with `created: true`, with no `why`

## why

- **Required in all four** (`look`, `edit`, `replace` and `new`). Blank is not allowed either (`invalid_input`)
- In `look` it is "why it looks here", in `edit` "why it changes it this way". Write the reason in one sentence, not a rephrasing of what is being done
- Write it in **the same language as the conversation with the user** (the descriptions of the tools and the input schema ask for this). It is for people to read

## Errors

In the MCP response `isError` is `true`, and the body is the following JSON.

```json
{"ok": false, "error": {"code": "selection_stale", "message": "…", "actual": ["…"]}}
```

| Code | Meaning | What the AI should do |
|---|---|---|
| `invalid_selection` | The form of the token is wrong, or it was altered. A token issued on another tape (another session) also gives this. So does a new session started after a gap of 30 minutes | Call `look` again |
| `selection_stale` | An edit that overlaps the range was made after the token was issued | Call `look` again |
| `selection_mismatch` | Even with the line numbers corrected, the content of the range differs from when `look` was called (it may have been changed outside srwr) | Check the content and call `look` again |
| `file_not_found` | The target file does not exist, or is not a regular file (a directory, for example) | — |
| `invalid_range` | The line numbers are outside the file, or the path is outside the workspace | Check the number of lines and call `look` again |
| `content_mismatch` | The range holds other lines than `expect` | Read where the message says the lines are, and call `look` again (or copy from `nearMatches`) |
| `content_not_found` | `expect` is not in the file | Check the content, or give line numbers. If `nearMatches` is there, copy `expect` from it |
| `content_ambiguous` | `expect` (given without line numbers) is in the file in more than one place | Give line numbers, or more lines in `expect` |
| `file_exists` | `new` was used on a file that already exists | Use `look` and `edit` on it |
| `count_mismatch` | `replace` found a number of places other than `count` | Read how many each file has, and call `replace` again with the right `count` (or use `look`). If `nearMatches` is there, copy `old` from it |
| `use_edit` | `replace` was used with a `count` of 1 for a text that is in one place only | Call `edit` as `actual.edit` says (add `why`) |
| `ignored_file` | `look`, `edit`, `replace` or `new` was used on a file that is not recorded | srwr cannot handle it. Ask the user |
| `invalid_input` | A required input is missing, has the wrong type, or `why` is empty; `file` is empty or has a NUL; only one of `startLine` and `endLine` is given, or none of them and no `expect`; `selection` is blank; `newText` has a CR; for `replace`, `files`, `old` or `count` is missing or empty, a path is given twice, `old` or `new` has a CR; for `new`, `content` is missing or has a CR, or a directory above the file is a file | Fix the input |
| `unsupported_file` | CRLF or binary | — |
| `internal_error` | An I/O error and the like | — |

An error may carry the current content (`actual`). What it holds is decided for each code.

| Code | `actual` |
|---|---|
| `selection_mismatch` | The current content of the corrected range (an array of lines) |
| `content_mismatch` | The current content of the range (an array of lines) |
| `selection_stale` | The current content of the range, corrected up to just before the overlapping edit (kept inside the file) |
| `count_mismatch` | The number of places in each file (`{"a.go": 3, "b.go": 1}`) |
| `use_edit` | `{"hits": [{file, startLine, endLine, lines}], "edit": {file, startLine, endLine, expect, newText}}`. `edit` is left out when the change cannot be told in lines |
| `invalid_range` | `{"lineCount": number of lines}`. None for a path outside the workspace |
| Others | None |

### `nearMatches`

When a text is not found, a mistake in spaces and tabs is the likeliest reason. So when `expect` (for `look` and `edit`) or `old` (for `replace`, only if fewer places were found than `count`) is not found but for spaces and tabs, the error has `nearMatches` (next to `actual`, not in it; `actual` is not changed): `[{"file", "startLine", "endLine", "lines"}]`, with the lines as they are in the file (`file` is only for `replace`). To compare, each line has its runs of spaces and tabs folded into one space and its trailing spaces dropped. A place where the text is exactly is not listed. At most 5 are listed, and `nearMatches` is left out if there is none. If there is no such place but `expect` is found as part of whole lines (`foo(` for the line `x := foo(1)`), those lines are listed instead, and the message says `expect` must be whole lines (`Line 12 holds expect only as part of the line: expect must be whole lines. Copy them from nearMatches`); this is for `look` and `edit` only. The message says where (`Line 12 differs from expect only in spaces or tabs: see nearMatches`), without any of the file. **`nearMatches` is not written to the tape**: its lines are the file's.

A failed call is also written to the tape as a [`failure`](tape.md#failure), so that the mistakes the AI makes can be read later. The real path is left out when it is absolute or outside the workspace.

## The order of processing

When srwr receives a `edit`, it works in this order.

1. Decode and verify the token (a failure is `invalid_selection`)
2. Find the file
3. Detect changes to files made outside srwr (if any, record them as [`external`](tape.md#external))
4. Correct the line numbers (an overlapping edit gives `selection_stale`)
5. Check the content (a mismatch gives `selection_mismatch`)
6. **Write the real file first, then append to the tape**

The file to detect is learned from the token, so decoding comes first. With a forged token, no external change is recorded. For `look`, the order is: the check of the path, the files that are not recorded and the kind of file (`invalid_range`, `ignored_file`, `file_not_found`, `unsupported_file`), then detection, then the check of the range (`invalid_range`), then the check of the content (`content_mismatch`, `content_not_found`, `content_ambiguous`), then recording.

Even if the process dies in between, the next time srwr touches the files, the mismatch with the real file shows up as `external`.

## Mistakes an AI tends to make

These are the mistakes seen when an AI used the tools. Each is an ordinary error with a code, and a failed call is written to the tape as a [`failure`](tape.md#failure).

- **Give `file` as a path relative to the workspace.** An absolute path such as `/home/me/app/main.go` is `invalid_range`, and so is `../main.go`. Write `cmd/app/main.go`.
- **An empty range is easy to place one line off.** `endLine = startLine - 1` means "just before line `startLine`", so `startLine: 13, endLine: 12` is between lines 12 and 13. Read the lines on both sides of the place first, and check the numbers before calling `look`.
- **The line numbers of a new `look` are the numbers of the file now.** srwr corrects the token it has already issued when another edit moves the lines, but not the `startLine` and `endLine` of a new `look`. After other edits, read the file again, or check the returned `lines` (and, after a `edit`, `above` and `below`). Better: pass `expect` with the lines you mean, and a wrong number is refused instead of selecting the wrong place.
- **Make a new file with `new`.** `look` on a file that does not exist gives `file_not_found`. A file made with a shell command is recorded too, as an [`external`](tape.md#external) with `created: true`, but without a `why`.
- **To change the same place again, use the new token that `edit` returned.** The token you used is spent: using it again gives `selection_stale`.
- **For the same change in many places, use `replace` with `count`**, not many `look` and `edit`. A wrong `count` is refused with the number each file has.
- **Read the error.** `invalid_range` returns `lineCount` (the number of lines of the file), which is enough to correct the numbers.

## Related

- What is recorded: [tape.md](tape.md)
- The command line and installation: [cli.md](cli.md)
