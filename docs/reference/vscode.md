# VSCode (srwr-view)

*[日本語](vscode_ja.md) | **English***

**Readers**: people who view tapes in VSCode.

With the VSCode extension **srwr-view**, you step through the AI's operations frame by frame. The look and behavior are fixed. You view by **stepping only**; there is no autoplay.

## Getting started

1. Install the `srwr` binary ([cli.md](cli.md)). The extension starts it to read tapes
2. Install the extension srwr-view (the `.vsix`)
3. Open the workspace in VSCode. From "srwr" at the left edge → "Operations", choose "Open a tape" or "Start live view"

- Where `srwr` is, is the setting `srwr.path` (the default `srwr` is searched for on the PATH). When it is not found, the extension says how to install it, with an "Open Settings" button. When the versions (`protocolVersion`) do not match, it asks you to update one of them
- The only setting is `srwr.path`. There are no settings for the look
- The view server (`srwr view-server`) is started when it is first needed. One per workspace
- The extension writes neither tapes nor real files. It does not read the key (`.srwr/key`) either
- Tapes of old formats can be read too

## Language and time

- The texts the extension shows are **English**, and **Japanese when VSCode's display language is Japanese** (install the Japanese language pack). The names of the commands in the command palette follow the same language. There is no setting of srwr for this
- The times in the tape picker are shown in the time zone of the machine. The tape holds UTC

## Screen specification

### 1. Colors

**select is blue, and what changes a file (replace, external change) is orange.** The reason line is a dark color with white bold text, and the range is the lighter version of the same color.

| Part | select (blue) | replace (orange) |
|---|---|---|
| Reason (why) line | Background `#0b61a4`, white bold | Background `#b45f06`, white bold |
| Range | Dark theme `#1d3a5c` / light theme `#cfe3fb` | Dark theme `#583c27` / light theme `#fde3c8` |

- The color of the reason line is the same in both themes (to keep the white bold text readable). Only the range changes with the theme
- A **failure** (a `select` or `replace` that gave the AI an error) is **red**: the reason line `#d50000` (white bold, the same in both themes), and the dot in the operation list `srwr.failureForeground` (a color of the extension: dark `#ff3b30`, light `#d50000`). Red and orange are told apart by the mark `✖` and the word `failure` as well
- For an external change (a change made outside srwr), only the dot in the operation list is **purple** (`charts.purple`). The color of the range follows how diffs are shown (3 below)

### 2. Frames of select and replace

- The tape is expanded into a virtual document (`srwr-replay:/<tape name>/<file>`) and shown. Real files are not touched. The extension of the original file is kept, so syntax highlighting works
- **A reason (`why`) line is actually inserted just before the range.** White bold text on a blue or orange background. A long reason is wrapped into several lines at the width (a display width of 100) and shown in full. From the second line it is indented. A frame whose `why` is `null` shows no reason line (only the color of the range)
- The lines of the range are painted in the lighter color (an empty range is not painted)
- It is replaced each time the frame changes, and the reason line is always only that of the current frame
- A replace shows **the content after the rewrite, in orange from the start** (there is no red → green motion)
- **Line numbers**: inserting the reason line makes the standard line numbers wrong. So in a frame that has a reason line the standard numbers are turned off, and **the real file's own numbers** are shown at the left edge of the text (blank for the reason line). Frames without a reason line and diff frames use the standard line numbers

### 3. Diff frames (external change, final diff)

- An `external` (a change made outside srwr, in the middle of the tape) and a `final` (the difference from the current file, after the recording) are shown in **two editors, left and right**. Left = before, right = after
- **Only the changed lines are painted: blue on the left, orange on the right.** The extension decides the changed lines by comparing before and after line by line. The standard diff screen is not used (its colors cannot be changed). There are no blank lines to align the lines, and the two sides do not scroll together
- It scrolls so that the first changed line comes about **30%** from the top of the screen. If one side has no changed line (only added, or only removed), it follows the lines of the other side
- The heading is shown on the left tab
  - "Before ⚠ Changed outside srwr: main.go"
  - "Before ⚠ Changed after recording (diff from current file): main.go"
  - For a file that no longer exists: "… (no longer exists)" on the "after recording" heading, and "… (deleted)" on the "outside srwr" heading when the external change deleted it. The right tab is "After main.go"
- When it goes back to an ordinary frame, the editor on the right is closed. **Tabs do not pile up when you move back and forth**
- One frame per change. If there is no change, none is shown

### 3b. Failure frames

- A failure frame has no file to open (a failure is often about an absolute path, a file that is not recorded, or a wrong input). It is shown as a **virtual document that explains it**: a red line `✖ select failed (invalid_range)` (white bold; for a `replace` it reads `replace failed`), then the message, then `why`, `tool` and `range` (the range given to a `select`; a `replace` has none) and `file` (`(not shown)` when it is left out). The editor tab is named `failure: select (invalid_range)`
- It is on the list only when `failure` is shown (see 8 below)

### 4. The operation list (the left panel)

- "srwr" at the left edge → "Operations". The frames are listed **in the order recorded, 1, 2, 3… from the top**. The number is the same as the position in the bottom bar (5/7). There is no indenting by parent and child
- Each row: `number  kind  file:range` (`37`, `39-41`; an empty range reads `before 12`), with the `why` as the description. The color of the dot at the front tells them apart: select = blue, replace = orange, external and final = purple. The kinds are written `select`, `replace`, `external`, `final` and `failure` (in the Japanese UI: `select`, `replace`, `外部変更`, `録画後` and `失敗`). A failure row is a **red** dot, and its place is the error code (`invalid_range`) instead of `file:range`
- Clicking moves to that frame. The row of the current frame is selected
- The ☓ at the top right closes the tape (or the live view)

### 5. The bottom bar (status bar)

- **"Back" and "Forward" and the position (5/7) are always shown.** The side that cannot be taken (Back at the first frame, Forward at the last) is dimmed (not hidden)
- There are no buttons for play, speed, or real-time / even spacing
- The ☓ at the right edge closes it

### 6. Live

- **The same screen as replay** (reason line, colors, line numbers, diff frames). Real files are not touched. What is shown is the text the server hands over (`withText`)
- The frames up to the start are only listed, not shown. Frames appended after the start are shown. `external` frames of the tape appear, but the final diff (`final`) does not: it is a comparison made when a recorded tape is opened
- **While following the newest**, a new frame is shown at once. The bottom bar shows "● LIVE". It follows even when frames arrive one after another
- **When you step back to an old frame with the stepping**, the screen does not move and only the number of new frames is counted. The bottom bar shows "Back to LIVE (N new)" (on an orange background). Pressing it moves to the newest and follows again
- When it moves to another tape, the list is rebuilt and shown from the beginning

### 8. Which frames to show

- The kinds **select, replace, external and failure** can each be turned on or off. At the start, select, replace and external are on and **failure is off**. `final` (the diff after the recording) follows external
- The **funnel button** at the top right of the operation list (and the command `srwr: Choose Frames to Show`) opens a multi-select list with the four kinds. Applying it opens the tape again with the chosen kinds ([protocol.md](protocol.md)) and moves to the frame nearest to the current one (by the order recorded). In live, it opens again and follows the newest
- The numbers are 1, 2, 3… of what is shown, so the number on the list is still the position in the bottom bar
- Under the heading of the list it says what is left out: `Hiding: failure (2)`. In live the count follows the frames that arrive
- A kind that is off does not appear in the list or in stepping. In live, **"Back to LIVE (N new)" counts only the kinds that are shown**
- When nothing is shown, the screen says "No frames to show"
- The choice is not saved: it is kept while the extension runs, and a new start is the default. srwr adds no setting for it

### 7. The tape picker

- "Open a tape" shows the tapes that have one or more operations, newest first (the start time with seconds in the time zone of the machine, then the number of operations and the files, and the tape file name below). Choosing one shows it from the first frame

## Commands and settings

| Command | Action |
|---|---|
| srwr: Open Tape (`srwr.openTape`) | Choose from the list and open |
| srwr: Close (Tape or Live) (`srwr.closeTape`) | Closes whichever of replay and live is open |
| srwr: Forward / Back (`srwr.stepForward`, `srwr.stepBack`) | Stepping (works in live too) |
| srwr: Start Live View / Stop Live View (`srwr.liveStart`, `srwr.liveStop`) | |
| srwr: Back to LIVE (`srwr.liveLatest`) | Moves to the newest and follows again |
| srwr: Choose Frames to Show (`srwr.chooseKinds`) | Turns the kinds select, replace, external and failure on and off |
| `srwr.goto` | Internal (a click in the operation list). Not shown in the command palette |

| Setting | Default | Meaning |
|---|---|---|
| `srwr.path` | `srwr` | Where the `srwr` binary is (in development, the environment variable `SRWR_PATH`) |

## How it works (for those who build)

- The frames are built by the view server ([protocol.md](protocol.md)). The extension only draws
- The colors of the range, the reason line and the lines of a diff are `TextEditorDecorationType`s. The standard line numbers are switched with `editor.options.lineNumbers`, and the extension's own numbers are shown with the `before` (`contentText`) of a decoration
- The texts of the screen are chosen by `vscode.env.language` (`src/lang.ts`); the names in `package.json` are `%keys%` of `package.nls.json` (English) and `package.nls.ja.json` (Japanese)
- Reasons for the design decisions are in [decisions.md](../design/decisions.md)
