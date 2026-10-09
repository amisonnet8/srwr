<h1 align="center">⚠️ Experimental until v0.2.0: breaking changes will happen ⚠️</h1>

<p align="center"><b>Until v0.2.0, srwr is an experiment. Tools, inputs, the tape format and the screens may change without notice, and old tapes may stop working.</b></p>

<p align="center">
  <img src="docs/images/banner.svg" alt="srwr: let AI agents edit files with just select and replace, and replay every step with its reason" width="100%">
</p>

<h3 align="center"><b>Let AI agents edit files with just <code>select</code> and <code>replace</code>, record every step with its reason on a tape, and replay it frame by frame in VSCode or Vim.</b></h3>

<p align="center"><i><a href="README_ja.md">日本語</a> | <b>English</b></i></p>

<p align="center">
  <a href="https://github.com/amisonnet8/srwr/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/amisonnet8/srwr/ci.yml?branch=main&label=CI&logo=github"></a>
  <a href="https://github.com/amisonnet8/srwr/releases"><img alt="Release" src="https://img.shields.io/github/v/release/amisonnet8/srwr?include_prereleases&sort=semver&logo=github"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/github/license/amisonnet8/srwr"></a>
  <img alt="Go" src="https://img.shields.io/github/go-mod/go-version/amisonnet8/srwr?logo=go&logoColor=white">
  <img alt="Vim 9.0.0784+" src="https://img.shields.io/badge/Vim-9.0.0784%2B-019733?logo=vim&logoColor=white">
  <a href="https://marketplace.visualstudio.com/items?itemName=amisonnet8.srwr-view"><img alt="VSCode Marketplace" src="https://flat.badgen.net/vs-marketplace/v/amisonnet8.srwr-view"></a>
</p>

<p align="center">
  <img alt="srwr view in Vim: the AI's look and edit, with the reason of each, stepping frame by frame" src="docs/images/demo-vim_dark.svg" width="900">
  <br>
  <sub>The real <code>srwr view</code> in Vim, stepping through a session: a reason line, then the code it is about.</sub>
</p>

## Contents

- [✨ Features](#-features)
- [🚀 Install](#-install)
- [🧭 How to use it](#-how-to-use-it)
- [👀 Viewing a tape](#-viewing-a-tape)
- [🤝 Sharing a tape](#-sharing-a-tape)
- [📚 Documentation](#-documentation)
- [💬 Interactive guide](#-interactive-guide)
- [📄 License](#-license)

## ✨ Features

- ✍️ **Two commands only**: the AI edits with `select` (look) and `replace` (change). No other way to touch a file, so nothing slips past the record
- 🧾 **A reason for every step**: each call carries a `why`, and an empty one is refused
- 🎞️ **Frame by frame**: replay what the AI did one frame at a time, with the reason line above the code it is about
- 🧩 **VSCode and Vim**: the same screen in the [VSCode extension](extension/README.md) and in a plain Vim (`srwr view`, nothing to install)
- 📡 **Live**: follow a tape while the AI is still writing it, and jump back to the newest frame when you want
- 🔍 **Changes from outside are not lost**: if a file changes outside srwr, the next call notices and shows it as a side-by-side diff
- 🔒 **Secrets stay out**: `.env`, keys and anything in `.srwrignore` are never put on a tape
- 📦 **One binary, no dependencies**: a single Go binary, no cgo, Linux, macOS and Windows
- 🕒 **Times that make sense**: tapes keep UTC, and screens show your own time zone
- 🌍 **English and Japanese**: English by default; `SRWR_LANG=ja` for Japanese

## 🚀 Install

You need the `srwr` binary first. It is what the AI talks to, and what the editors start to read tapes.

```bash
go install github.com/amisonnet8/srwr/cmd/srwr@latest
```

Prebuilt binaries for Linux, macOS and Windows are on [GitHub Releases](https://github.com/amisonnet8/srwr/releases). Put the binary on your `PATH`.

To view in VSCode, install the extension **srwr-view** as well (it does not contain `srwr`; see [its README](extension/README.md)). To view in Vim, you need Vim 9.0.0784 or later and nothing else.

## 🧭 How to use it

1. **Set up your project** (once). This registers `srwr` for [Claude Code](https://claude.com/claude-code) and, by default, forbids its Edit and Write tools so that every change goes through `look`, `edit` and `new`:

   ```bash
   cd your-project
   srwr init
   ```

   <details>
   <summary>What does <code>srwr init</code> write?</summary>

   | File | Content |
   |---|---|
   | `.srwr/` | The directory for tapes, and the key |
   | `.mcp.json` | Registers `srwr mcp`, the MCP server the AI uses |
   | `.claude/settings.json` | The hook that records what the AI reads, and the ban on Edit/Write (`srwr init --lenient` keeps them allowed) |
   | `.gitignore` | Ignores the key and the lock, not the tapes |

   Existing settings are kept, and running it again changes nothing. Details: [docs/reference/cli.md](docs/reference/cli.md).
   </details>

2. **Let the AI work.** Every `look`, `edit` and `new` goes on a tape in `.srwr/tapes/`:

   <details>
   <summary>What does a tape look like?</summary>

   A tape is a JSONL file, one event per line. This is a `look` (the AI looks at lines 9 to 11) and the `edit` that follows:

   ```json
   {"v":2,"seq":2,"type":"look","file":"main.go","startLine":9,"endLine":11,"why":"greet is what main prints: check what it does when the name is empty","selection":"sel_041061E48KVH3K24RN324MN2","source":"mcp"}
   {"v":2,"seq":3,"type":"edit","file":"main.go","from":"sel_041061E48KVH3K24RN324MN2","startLine":9,"endLine":11,"oldText":"…","newText":"…","why":"An empty name printed \"hello \" with nothing after it, so greet a stranger instead","source":"mcp"}
   ```

   The format is in [docs/reference/tape.md](docs/reference/tape.md).
   </details>

3. **Look at what happened** (below).

A whole round with every command, from `srwr init` to tidying up the tapes, is in [docs/examples/workflow.md](docs/examples/workflow.md).

## 👀 Viewing a tape

| Where | How |
|---|---|
| **Vim** | `srwr view` (pick a tape from the list), or `srwr view <tape>`. `]]` forward, `[[` back, `q` to close. `srwr view --live` follows a tape being written |
| **VSCode** | Install the extension, then *srwr* at the left edge → *Operations* → *Open a tape* or *Start live view* |
| **Terminal** | `srwr tapes` lists the tapes of the project |

<p align="center">
  <img alt="srwr-view in VSCode: an orange reason line above the replaced lines, the operations list on the left, the bottom bar" src="docs/images/vscode_dark.svg" width="900">
  <br>
  <sub>srwr-view in VSCode. Blue is a <code>look</code>, orange is an <code>edit</code>, purple is a change from outside.</sub>
</p>

You step with *Back* and *Forward*; there is no autoplay, because the point is to read each reason. Details: [VSCode](docs/reference/vscode.md) and [Vim](docs/reference/vim.md).

## 🤝 Sharing a tape

A tape holds the whole text of the files it touched, and it replays on its own. Hand it over with `srwr tapes path <tape>`, and the other person opens it in their own editor.

- 🔑 **Never share `.srwr/key`.** It is not needed for replay
- 🔒 Files with secrets are not recorded (`.env`, `*.pem`, `*.key`, SSH keys, and what you list in `.srwrignore`). Check the tape before you share it all the same
- 🕒 A tape keeps UTC; the other person sees their own time zone

## 📚 Documentation

| You want to | Read |
|---|---|
| Use srwr | [Command line](docs/reference/cli.md) · [Settings](docs/reference/settings.md) · [The tools the AI uses](docs/reference/mcp.md) |
| View in an editor | [VSCode](docs/reference/vscode.md) · [Vim](docs/reference/vim.md) |
| Read tapes or add an editor | [Tape format](docs/reference/tape.md) · [View server protocol](docs/reference/protocol.md) |
| Know how it is built | [Overview](docs/design/overview.md) · [Decisions](docs/design/decisions.md) · [Limits](docs/design/limitations.md) |

Everything is in [docs/](docs/README.md), in English and in Japanese.

## 💬 Interactive guide

Ask questions and explore srwr in a conversation: **[the srwr guide, made with Gemini Notebook](https://notebook.google.com/notebook/e2afe5b3-fb70-4889-adb7-7f6bbe4d2912)**.

## 📄 License

[MIT](LICENSE) © amisonnet8

<p align="right"><a href="#">⬆ back to top</a></p>
