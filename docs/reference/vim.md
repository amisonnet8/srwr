# Vim (srwr-view.vim)

*[日本語](vim_ja.md) | **English***

**Readers**: people who view tapes in Vim in a terminal.

From the terminal where the AI agent runs, you can replay a tape in Vim as it is. It shows the same information as VSCode, in the same order.

## Getting started

- **`srwr view [tape]`**: writes the Vim scripts embedded in the `srwr` binary to the user's cache directory (`os.UserCacheDir()/srwr/vim/<version>-<hash of the files>/`), adds it to `runtimepath`, and starts Vim. **You do not have to install a plugin**
  - Run it in the workspace directory (where `.srwr/` is). From elsewhere, `--root <workspace>`
  - The tape is a tape ID (what shows in the list of `:SrwrOpen`) or the path of a `.tape.jsonl` or `.tape.jsonl.gz` (compressed). If left out, you choose from the list buffer
  - `--live` is live viewing (`srwr view --live`; it is not given together with a tape)
  - The Vim that is started is `$SRWR_VIM`, or `vim` on the PATH. Your `vimrc` is still read (settings of `g:srwr_…` can go in your `vimrc`)
  - With a Vim that cannot be used (too old, `vim-tiny` and the like), it says what is missing and ends
- **It can also be installed as a plugin**: install the repository's `vim/` with a plugin manager. After that, the following commands can be used inside Vim. The `srwr` binary is needed in this case too (on the PATH, or given with `g:srwr_path`)

| Command | Action |
|---|---|
| `:SrwrOpen [tape]` | Opens a tape. The list if left out |
| `:SrwrLive` | Live viewing |
| `:SrwrNext`, `:SrwrPrev`, `:SrwrClose` | Stepping (forward, back), and closing (the same as "Keys" below) |
| `:SrwrLatest` | In live, goes back to the newest frame |
| `:SrwrToggle {look\|edit\|external\|failure}` | Turns a kind of frame on or off (the same as the keys in "Which frames to show") |

The only setting is `g:srwr_path` (where the `srwr` binary is). srwr has no settings for the look (the colors can be overridden with highlight groups; see below).

## Language and time

- The texts are **English**. Set the environment variable `SRWR_LANG=ja` before starting Vim for Japanese (`SRWR_LANG=ja srwr view`). Anything else, or nothing set, is English
- The times in the tape list are shown in the time zone of the machine (the environment variable `TZ`). The tape holds UTC
- The word for folded lines of a diff is `lines` in English and `行` in Japanese, whatever the language of the Vim itself

## Supported Vim

- **Vim 9.0.0784 or later**, with `+channel`, `+job`, `+textprop` and `+vim9script`. The feature to show virtual text above a line came in 9.0.0438, and 9.0.0784, which includes the fix of a display bug of it, is the oldest version checked (the tests pass with 9.0.0784 and with the latest Vim). The Vim of Debian 12 (9.0.1378) and Ubuntu 24.04 (9.1) and later satisfy this
- 'encoding' is utf-8
- With a Vim that lacks features (`vim-tiny` and the like), it says what is missing at startup and ends
- When the `srwr` binary is not found, the Vim script says so in red ("srwr binary not found (g:srwr_path = 'srwr'). Install srwr or set g:srwr_path"). When the versions (`protocolVersion`) do not match, it asks you to update srwr (this can happen only when `vim/` is installed as a plugin; the scripts that `srwr view` embeds always match their binary)
- Neovim is not a target (it may work, but it is not checked)

## Screen specification

It shows the same information as VSCode ([vscode.md](vscode.md)), in the same order, in the same colors. You view by **stepping only**; there is no autoplay. The parts are replaced by those of Vim.

### 1. Starting and the layout of the screen
- Start with `srwr view [tape]` (see "Getting started" above). Inside Vim, `:SrwrOpen [tape]` and `:SrwrLive`
- If the tape is left out, a **tape list buffer** (start time, update time, number of operations, number of files, tape) is shown, and `<CR>` opens one
- When opened, one tab is used like this
  ```
  ┌──────────────┬──────────────────────────────────────┐
  │ Operations    │ The replay buffer (read-only)         │
  │ ● 1 look …    │  ◆ Checking whether main needs a fix  │ ← reason line (blue)
  │ ● 2 look …    │  func main() {                       │ ← range (light blue)
  │ ● 3 edit …    │      …                               │
  │ …             │  }                                    │
  ├──────────────┴──────────────────────────────────────┤
  │ srwr  3/12  [[ Back  ]] Forward  main.go:12-14        │ ← status line
  └─────────────────────────────────────────────────────┘
  ```
- The operation list is 40 columns wide

### 2. Colors

The same as [vscode.md](vscode.md). **look is blue, and what changes a file (edit, new) is orange.** The reason line is a dark color with white bold text, and the range is the lighter version of the same color. The highlight groups are defined with `highlight default`, so they can be overridden in your `vimrc`.

| Group | Used for | dark | light |
|---|---|---|---|
| `SrwrWhySelect` | The reason line of a look | `#0b61a4`, white bold | the same |
| `SrwrWhyReplace` | The reason line of an edit (and a new) | `#b45f06`, white bold | the same |
| `SrwrSelect` | The range of a look, the before (left) of a diff | `#1d3a5c` | `#cfe3fb` |
| `SrwrReplace` | The range of an edit (and a new), the after (right) of a diff | `#583c27` | `#fde3c8` |
| `SrwrCurrent` | The current frame in the operation list | `#3a3d41` | `#e4e6f1` |
| `SrwrDotSelect`, `SrwrDotReplace` | The dots in the operation list (blue, orange) | `#4aa3ff`, `#f0883e` | `#0b61a4`, `#b45f06` |
| `SrwrDotExternal` | The dots in the operation list (external change, final) | `#b180d7` | `#652d90` |
| `SrwrWhyFailure` | The reason line of a failure frame | `#d50000`, white bold | the same |
| `SrwrDotFailure` | The dots in the operation list (failure, red) | `#ff3b30` | `#d50000` |
| `SrwrDim` | Buttons that cannot be used | gray | gray |

The names with `Select` and `Replace` are older names of the colors (blue, orange). They were kept as they are, so that the overrides in your `vimrc` keep working; the names of the groups are not shown on the screen.

- It has values that can be told apart even on a 256-color terminal (`ctermbg`). If `termguicolors` is on, those colors are used
- Changing 'background' changes the colors (colors the user overrode are left as they are)

### 3. The replay buffer (frames of look and edit)
- The name is `srwr://<tape name>/<file>` (`srwr://live/<file>` in live while there is no tape yet; a failure frame is `srwr://<tape name>/failure<index>`; a diff frame has the two buffers `srwr://<tape name>/before/<file>` and `…/after/<file>`; the operation list is `srwr://operations`, and the tape list `srwr://tapes`). `buftype=nofile`, `nomodifiable`. `filetype` is decided from the extension of the original file (syntax highlighting is left to Vim)
- For each frame, the content is replaced with the content of the document received from the view server
- **A reason line is actually inserted just before the range.** A long reason is wrapped into several lines at the width of the window and shown in full. From the second line it is indented. A frame whose `why` is `null` shows no reason line
- The lines of the range are painted in the lighter color (up to the end of the line. An empty range is not painted)
- **Each frame is shown from the top of its content.** Only when the reason line and the first line of the range do not fit on the screen, it scrolls so that the reason line comes to the middle of the screen (the blank space below the end of the file is not shown). Even when the range is near the bottom edge of the screen, the first line of the range is visible below the reason line
- An edit shows the content after the rewrite, in orange from the start (there is no red → green motion)
- **Line numbers**: inserting the reason line makes the standard line numbers (`number`) wrong. So in a frame that has a reason line `number` is turned off, and **the real file's own numbers** are shown at the left edge of each line as virtual text (blank for the reason line). Frames without a reason line and diff frames use the standard `number`. The same look as VSCode

### 4. Diff frames (external change, final diff)
- The replay window is split into **two windows, left and right**, and lined up with `:diffthis`. Left = before, right = after. Both are read-only buffers
- **Only the changed lines are painted: blue on the left, orange on the right.** The standard diff colors of Vim (`DiffAdd` and the like) are taken away only while a diff frame is shown, and put back afterwards
- The cursor is put on the first changed line, and it scrolls so that the line comes about **30%** from the top of the screen. If one side has no changed line, it follows the lines of the other side
- The status line of each window shows a heading
  - Left: "Before  ⚠ Changed outside srwr: main.go", "Before  ⚠ Changed after recording (diff from current file): main.go"
  - Right: "After  " followed by the usual status (`srwr  6/12 …`)
- When it goes back to an ordinary frame, it returns to one window. **Windows and buffers do not keep increasing**
- Moving between the blocks of changes is done with Vim's `]c` and `[c`
- The unchanged lines are folded. The fold text is `+-- 17 lines: …` (`行` in Japanese)

### 4c. Frames of new

- A file made by `new` is shown like an `edit` frame, in one window: the whole file painted orange, the `why` in the orange rows above it. In the operation list a row reads `● n new     file:range` with an orange dot

### 5. The operation list
- The frames are listed **in the order recorded, 1, 2, 3…** (the number is the same as the position in the status line). There is no indenting by parent and child
- Each row: `● number kind file:range  why` (the range is `37`, `39-41`, or `before 12` for an empty range; the same order as the list of VSCode: dot, number, kind, file name). The color of the dot tells them apart (look = blue, edit and new = orange, external and final = purple). An external change is `external`, and the final diff is `final`. An `external` that has no `why` says `File changed outside srwr` in the place of the `why`, as in VSCode
- `<CR>` moves to that frame. The row of the current frame is painted

### 6. The status line and keys
- The status line: `srwr  position/total  [[ Back  ]] Forward  file:range`. **"Back" and "Forward" are always shown.** The side that cannot be taken (Back at the first frame, Forward at the last) is dimmed. The "(Close: q in the list on the left, or :SrwrClose)" that comes last in live is shown only when all of it fits in the window (when it does not fit, it is left out so that "L: Back to LIVE (N new)" can always be read). If the line is still too long for the window, the file and range at its end are cut (the position and the buttons stay)
- The keys work only inside the buffers of srwr (the operation list, the replay, the diff; `q` works in all of them). Even if a plugin for the file type (`filetype`) assigns the same keys (`]]`, `[[` and so on), the keys of srwr win

| Key | Command | Action |
|---|---|---|
| `]]`, `<Right>` | `:SrwrNext` | One forward |
| `[[`, `<Left>` | `:SrwrPrev` | One back |
| `L` | `:SrwrLatest` | In live, go back to the newest frame and follow again |
| `q` | `:SrwrClose` | Close |

### 6b. Failure frames
- A failure frame has no file to open, so the replay buffer is an **explanation**: a red line `✖ look failed  (invalid_range)` (`SrwrWhyFailure`, white bold; for an `edit`, `edit failed`), then the message, a blank line, and `why`, `tool`, `range` (a `look` only) and `file` (`(not shown)` when it is left out). The status line says `failure: look`
- In the operation list a failure row has a red dot (`SrwrDotFailure`), the kind `failure`, and the error code in the place of `file:range`

### 6c. Which frames to show
- The kinds **look, edit, external and failure** can each be turned on or off. At the start, look, edit and external are on and **failure is off**. `final` follows external, and `new` follows edit
- Keys (only inside the buffers of srwr): `tl` look, `te` edit, `tx` external, `tf` failure. `:SrwrToggle {kind}` does the same. In these buffers `t` followed by `l`, `e`, `x` or `f` is taken by srwr
- A change opens the tape again with the chosen kinds ([protocol.md](protocol.md)) and moves to the frame nearest to the current one (by the order recorded); in live it follows the newest again
- The numbers are 1, 2, 3… of what is shown. The right end of the status line says what is left out: `hidden: failure (2)`; in live the count follows the frames that arrive. A kind that is off is not in the list or in stepping, and "L: Back to LIVE (N new)" **counts only the kinds that are shown**. When nothing is shown, the replay buffer says "No frames to show"
- Each change says what is shown now (`srwr: showing look, edit, external`), and a name that is not a kind is a warning
- The choice is not saved: it is kept while Vim runs, and a new start is the default. srwr adds no setting for it

### 7. Live (`srwr view --live`, `:SrwrLive`)
- **The same screen as replay** (reason line, colors, diff frames, operation list). Real files are not touched. What is shown is the text the server hands over (`withText`)
- The frames up to the start are only listed. "Live view (waiting for the AI's operations)" is shown, and frames appended after the start are shown (`external` frames appear; the final diff does not, as in VSCode)
- **While following the newest**, a new frame is shown at once. The status line shows "● LIVE". It follows even when frames arrive one after another
- **When you step back to an old frame with `[[`**, the screen does not move and only the number of new frames is counted. The status line shows "L: Back to LIVE (N new)" (on an orange background). `L` moves to the newest and follows again
- When it moves to another tape, the list is rebuilt and shown from the beginning
- To close: `q` in the operation list on the left, or `:SrwrClose` from anywhere (it is written in the status line too)
- When no frame has come yet the status line says "● LIVE  (waiting for the AI's operations)". If the view server ends while live is open, the live view is closed with a warning

### 8. The tape list
- The buffer `srwr://tapes`. Newest first (by the time the tape started), with the columns `Started`, `Updated`, `Ops`, `Files` and `Tape` (the tape ID), the times in the time zone of the machine (`2026-10-03 17:12`). With no tape it says "No tapes (in .srwr/tapes/ of this workspace)". `<CR>` opens one. `q` closes it. When a tape has a title (the AI gave one with the [`session`](mcp.md#session) tool), a column `Title` is added at the end, and the tape has its title there; a list with no titled tape has no such column

## How it works (for those who build)

- The frames are built by the view server ([protocol.md](protocol.md)). Vim only draws
- Colors are text properties (`prop_add()`). The paint up to the end of a line is blank virtual text. The "changed lines" of a diff are found with `diff_hlID()`
- The language is chosen by `$SRWR_LANG` (`autoload/srwr/lang.vim`)
- Reasons for the design decisions are in [decisions.md](../design/decisions.md)
