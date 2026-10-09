# Settings

*[日本語](settings_ja.md) | **English***

**Readers**: people who use srwr. A list of what the user can set (and what cannot be set).

srwr has few settings. **srwr has no settings for the look (colors, widths, labels and so on).** (In Vim, the colors can still be overridden with highlight groups; see below.) What you set is mostly where the `srwr` binary is, and the language of what srwr shows.

## 1. Settings for viewing

### Where the srwr binary is

The editor starts the `srwr` binary to read tapes. If `srwr` is on the PATH, no setting is needed. If it is not found, give its location.

| Setting | Where | Default | Meaning |
|---|---|---|---|
| **`srwr.path`** | VSCode settings (the settings screen, or `settings.json`) | `srwr` | Where the `srwr` binary is. The default `srwr` is searched for on the PATH. If it is not found, give an absolute path |
| **`g:srwr_path`** | A Vim variable (write it in your `vimrc`) | `srwr` | The same. `let g:srwr_path = '/path/to/srwr'` |
| `SRWR_PATH` | Environment variable | none | Where `srwr` is, used only while `srwr.path` of VSCode is left at its default (for starting the extension for development) |
| `SRWR_VIM` | Environment variable | `vim` on the PATH | The Vim that `srwr view` starts |

- `srwr.path` of VSCode is **the only setting the extension has**. When the binary is not found, it says how to install it, with an "Open Settings" button; when the versions do not match, it asks you to update one of them
- In a Vim started by `srwr view`, `g:srwr_path` is set to that very `srwr` automatically
- Your `vimrc` is read also by `srwr view`. Settings of `g:srwr_…` can go in your `vimrc`

### Language

What srwr shows is **English by default**. Japanese is available.

| Where | How to switch to Japanese |
|---|---|
| The terminal output of `srwr` (usage, `srwr init`, `srwr tapes`, errors) | Set the environment variable `SRWR_LANG=ja` |
| Vim (`srwr view`, srwr-view.vim) | Set the environment variable `SRWR_LANG=ja` before starting Vim |
| VSCode (srwr-view) | Nothing to set for srwr: the extension follows the display language of VSCode. Install the Japanese language pack and VSCode shows the Japanese texts |

- `SRWR_LANG` is Japanese when its value starts with `ja` (`ja`, `ja_JP.UTF-8`); anything else, or nothing set, is English. `LANG` and other variables are not consulted
- The texts meant for the AI (the MCP tool descriptions and error messages), the notes of the hook, the error messages of the view server, and the "cannot run" notice that the Vim script prints when the Vim is too old (it is shown before the script can look at `SRWR_LANG`) are English only

### Time

- Times on a tape (`header.startedAt` and the `ts` of every event) are written in **UTC** (`2026-10-03T08:12:10.000Z`). The date and time in the tape ID are UTC too. Tapes written by older versions with an offset such as `+09:00` are read as they are
- What is shown to a person (`srwr tapes`, the list of Vim, the tape picker of VSCode) is shown in **the time zone of the machine**. To show another zone, set the environment variable `TZ` (for example `TZ=America/Los_Angeles srwr tapes`)
- `srwr tapes` writes `Oct 03 17:12` in English and `10/03 17:12` in Japanese. The list of Vim writes `2026-10-03 17:12` and the picker of VSCode `2026-10-03 17:12:10` (with seconds), in both languages

### Overriding the colors in Vim

The colors are defined as Vim highlight groups and can be overridden in your `vimrc` (they are defined with `highlight default`, so your definitions win). With no setting, fixed colors that fit a dark or a light background are used.

| Group | Used for |
|---|---|
| `SrwrWhySelect`, `SrwrWhyReplace` | The reason line of a look / edit (new) |
| `SrwrSelect`, `SrwrReplace` | The range of a look / edit (new), the before (left) and after (right) of a diff |
| `SrwrCurrent` | The current frame in the operation list |
| `SrwrDotSelect`, `SrwrDotReplace`, `SrwrDotExternal` | The dots in the operation list (blue, orange, purple) |
| `SrwrDim` | Buttons that cannot be used (Back, Forward) |

The values are in [vim.md](vim.md). The colors of VSCode cannot be set (except for the parts the theme decides; see [vscode.md](vscode.md)).

## 2. Arguments of the commands

| Command | Arguments | Meaning |
|---|---|---|
| `srwr mcp` | `--root <workspace>` | The workspace directory. The current directory if omitted |
| `srwr view-server` | `--root <workspace>` | The same (started by the editor) |
| `srwr view [tape]` | `--root <workspace>`, `--live` | The workspace (the current directory if omitted), live viewing. The tape is a tape ID or the path of a `.tape.jsonl`. With none, the list is shown |
| `srwr hook` | `--root <workspace>` | Reads the JSON of a Claude Code hook from standard input. The workspace is `CLAUDE_PROJECT_DIR` if omitted, and then the current directory |
| `srwr init` | `--lenient`, `--root <workspace>` | Sets up a workspace. `--lenient` does not forbid Edit and Write |
| `srwr tapes` | `new`, `prune --keep N` / `--older-than 30d`, `path <tape ID>`, `--root <workspace>` | Lists and tidies tapes |
| `srwr` | `--version`, `--help` | The version, the usage |

## 3. Files in the workspace

| File | Content | How to set it |
|---|---|---|
| `.srwr/` | The key, the lock, the current session and the tapes. **Do not share the key (`.srwr/key`)** | srwr makes it |
| `.mcp.json` | Registers `srwr mcp` with the AI agent | `srwr init` writes it (you may write it by hand) |
| `.claude/settings.json` | Registers the hook of Claude Code, and forbids Edit, Write, MultiEdit and NotebookEdit (strict mode) | `srwr init` writes it (you may write it by hand) |
| `.srwrignore` | Files not to record (the same format as `.gitignore`) | Write it by hand |

An example of `.mcp.json` (Claude Code):

```json
{ "mcpServers": { "srwr": { "command": "srwr", "args": ["mcp"] } } }
```

What `srwr init` ([cli.md](cli.md)) writes into `.claude/settings.json` (strict mode): the registration of the hook, the setting that lets Claude Code use the tools without asking, and the ban on the built-in editing tools (for the reason, see [decisions.md](../design/decisions.md)). In lenient mode (`--lenient`), `deny` is not written.

```json
{
  "hooks": { "PostToolUse": [ { "matcher": "Read|Bash|Grep|Edit", "hooks": [ { "type": "command", "command": "srwr hook" } ] } ] },
  "enabledMcpjsonServers": ["srwr"],
  "permissions": {
    "allow": ["mcp__srwr__look", "mcp__srwr__edit", "mcp__srwr__new"],
    "deny": ["Edit", "Write", "MultiEdit", "NotebookEdit"]
  }
}
```

## 4. What cannot be set

These are fixed and cannot be set.

| Item | Value |
|---|---|
| The time that separates sessions | 30 minutes after the last event |
| The interval of the live watch | 200 milliseconds |
| The wrapping width of the reason | VSCode: a display width of 100; Vim: the width of the window |
| The look: colors, labels, how a diff is shown, and so on | Fixed (except the overriding of colors in Vim) |
| Autoplay, speed | None (stepping only) |
| How long tapes are kept | They are not deleted automatically |
