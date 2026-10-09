<p align="center">
  <img src="media/readme/banner.png" alt="srwr-view: replay what an AI agent did, frame by frame, with the reason of each step" width="100%">
</p>

<p align="center"><i><a href="https://github.com/amisonnet8/srwr/blob/main/extension/README_ja.md">日本語</a> | <b>English</b></i></p>

<p align="center">
  <a href="https://marketplace.visualstudio.com/items?itemName=amisonnet8.srwr-view"><img alt="Version" src="https://flat.badgen.net/vs-marketplace/v/amisonnet8.srwr-view"></a>
  <img alt="VSCode ^1.90" src="https://img.shields.io/badge/VSCode-%5E1.90-007acc?logo=visualstudiocode&logoColor=white">
  <a href="https://github.com/amisonnet8/srwr/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/amisonnet8/srwr/ci.yml?branch=main&label=CI&logo=github"></a>
  <a href="https://github.com/amisonnet8/srwr/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/github/license/amisonnet8/srwr"></a>
</p>

# srwr-view

**See what an AI agent did to your code, one frame at a time, with the reason of each step.** srwr-view is the VSCode side of [srwr](https://github.com/amisonnet8/srwr): the AI edits files with just `select` and `replace`, every step goes on a *tape* with its reason, and this extension replays the tape.

![srwr-view: an orange reason line above the replaced lines, the operations list on the left, the bottom bar](media/readme/replay.png)

## 📋 Prerequisites

This extension only draws. Reading tapes is the job of the **`srwr` binary**, which the extension starts, so install it first. It is **not** included in the extension.

```bash
go install github.com/amisonnet8/srwr/cmd/srwr@latest
```

- Or take a prebuilt binary for Linux, macOS or Windows from [srwr on GitHub Releases](https://github.com/amisonnet8/srwr/releases)
- Put `srwr` on your `PATH`, or tell the extension where it is with the setting [`srwr.path`](#-settings)
- This version of the extension talks to a view server of `protocolVersion` **3**. If the two do not match, the extension asks you to update one of them
- To have an AI record tapes in the first place, set up your project with `srwr init`. See the [srwr README](https://github.com/amisonnet8/srwr#-install) for the whole picture

## ✨ Features

- 🎨 **A reason line above the code**: blue for a `look`, orange for an `edit`, in front of the range it is about
- 🔍 **Side-by-side diffs** for changes made outside srwr and after the recording, with only the changed lines painted
- ⏮️ **Frame by frame**: *Back* and *Forward* in the bottom bar. No autoplay, so you read each reason
- 📡 **Live view**: follow a tape while the AI is still writing it; step back and a *Back to LIVE (N new)* button counts what you missed
- 📜 **A flat list of operations** on the left, numbered from 1, with a colored dot for the kind
- 🌍 **English and Japanese**: it follows the display language of VSCode
- 🔒 **Read-only**: the extension never writes a tape or a file, and never reads the key

## 🚀 Getting started

1. Install `srwr` (above)
2. Open your project in VSCode
3. Click **srwr** at the left edge → **Operations** → **Open a tape** (or **Start live view**)

## ⌨️ Commands

Open the command palette with <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>P</kbd> (<kbd>⌘</kbd>+<kbd>Shift</kbd>+<kbd>P</kbd> on macOS).

| Command | ID | What it does |
|---|---|---|
| srwr: Open Tape | `srwr.openTape` | Choose a tape of this workspace and show it from the first frame |
| srwr: Close (Tape or Live) | `srwr.closeTape` | Close the tape or the live view that is open |
| srwr: Forward | `srwr.stepForward` | One frame forward (works in live view too) |
| srwr: Back | `srwr.stepBack` | One frame back |
| srwr: Start Live View | `srwr.liveStart` | Follow the tape of the current session as it is written |
| srwr: Stop Live View | `srwr.liveStop` | Stop following |
| srwr: Back to LIVE | `srwr.liveLatest` | After stepping back in live view, jump to the newest frame and follow again |

## 🔧 Settings

There is one setting. Change it in the Settings UI (search for *srwr*) or in `settings.json`.

| Setting | Type | Default | Meaning |
|---|---|---|---|
| `srwr.path` | `string` | `"srwr"` | Where the `srwr` binary is. The default is searched for on the `PATH`. If it is not found, give an absolute path |

```json
{
  "srwr.path": "/home/me/go/bin/srwr"
}
```

<details>
<summary>On Windows</summary>

```json
{
  "srwr.path": "C:\\Users\\me\\go\\bin\\srwr.exe"
}
```
</details>

There are no settings for the look: the colors and the layout are fixed so that everyone sees the same screen.

## 🩺 If something does not work

| You see | Do this |
|---|---|
| *Cannot start srwr … Install srwr … or set its location in the setting "srwr.path"* | `srwr` is not on the `PATH`. Install it, or set `srwr.path` (the *Open Settings* button goes there) |
| *srwr … and this extension do not match (the extension is protocolVersion 3)* | Update `srwr` (`go install …@latest`) or the extension, whichever is older |
| *Open a tape* lists nothing | Only tapes with at least one operation are listed, and only those of the folder you opened (`.srwr/tapes/`) |

## 🔗 Links

- **[srwr](https://github.com/amisonnet8/srwr)**: the tool itself, how to install it and set it up
- [Documentation](https://github.com/amisonnet8/srwr/blob/main/docs/README.md): [this extension in detail](https://github.com/amisonnet8/srwr/blob/main/docs/reference/vscode.md), [all settings](https://github.com/amisonnet8/srwr/blob/main/docs/reference/settings.md), [the tape format](https://github.com/amisonnet8/srwr/blob/main/docs/reference/tape.md)
- [Report a problem](https://github.com/amisonnet8/srwr/issues)

## 📄 License

[MIT](LICENSE) © amisonnet8
