# The command line (srwr)

*[日本語](cli_ja.md) | **English***

**Readers**: people who use srwr.

`srwr` is a single Go binary. Its subcommands divide the roles.

| Command | Used by | Role |
|---|---|---|
| `srwr mcp` | The AI (an MCP client) | The MCP server (stdio). Provides [`select` / `replace`](mcp.md) |
| `srwr hook` | The hook of Claude Code | Records Read, Bash, Grep and Edit on the same [tape](tape.md) as `srwr mcp` |
| `srwr view-server` | The editors (the VSCode extension, the Vim script) | The view server. People do not use it directly ([protocol.md](protocol.md)) |
| `srwr view [tape]` | People | Replays in Vim ([vim.md](vim.md)) |
| `srwr init` | People | Sets up a workspace for srwr |
| `srwr tapes` | People | Lists and tidies tapes |

`srwr --version` shows the version and `srwr --help` shows the usage.

What srwr prints is English by default. Set the environment variable `SRWR_LANG=ja` for Japanese ([settings.md](settings.md)).

## Installation

```
go install github.com/amisonnet8/srwr/cmd/srwr@latest
```

Prebuilt binaries for Linux, macOS and Windows (amd64 and arm64) are distributed on GitHub Releases. Each file is named `srwr_<version>_<os>_<arch>.tar.gz` (`.zip` for Windows) and holds `srwr` (`srwr.exe`), `LICENSE` and `README.md`; `checksums.txt` lists the SHA-256 of every file (`sha256sum -c checksums.txt`). cgo is not used, so it runs as a single binary on any OS.

Choose the tool for viewing as you like.

| Where to view | What to install |
|---|---|
| VSCode | The extension srwr-view ([vscode.md](vscode.md)). It does not contain `srwr`, so install `srwr` first (above) |
| Vim | Nothing. `srwr view` starts Vim with the Vim scripts embedded in the binary ([vim.md](vim.md)) |

## The workspace directory

The directory where srwr is used is called the **workspace**. srwr makes the following in the workspace.

```
<workspace>/
├── .srwr/
│   ├── key                 The HMAC key (32 bytes, 0600). Do not share
│   ├── lock                For exclusive writing (flock)
│   ├── active              The tape ID of the current session
│   └── tapes/
│       ├── <id>.tape.jsonl   A tape (the one being written)
│       └── <id>.tape.jsonl.gz   A tape of a session that ended (compressed)
├── .srwrignore             Files not to record (optional)
├── .mcp.json               Registers srwr mcp (written by srwr init)
└── .claude/settings.json   Registers the hook, forbids Edit/Write (written by srwr init)
```

## srwr mcp

The MCP server the AI uses. It is started by the AI agent (the MCP client). The workspace is `--root <workspace>` (the current directory if omitted). There are two tools, `select` and `replace` ([mcp.md](mcp.md)). Several may be started in the same workspace. They write to the same session (the same tape), and a selection token issued by one can be used by another ([tape.md](tape.md)).

## srwr hook

```
srwr hook [--root <workspace>]
```

Called from a hook of Claude Code, it records the operations of the tools the AI already has on the same tape, so that the investigation (where it read) is on the tape too. It reads the JSON of the hook from standard input. A recorded run is in [hook.md](../examples/hook.md).

- The workspace is `--root`. If omitted, the environment variable `CLAUDE_PROJECT_DIR`, and then the current directory
- Only the JSON of `PostToolUse` (after a tool has been used) is recorded. The items used are `hook_event_name`, `tool_name`, `tool_input`, `tool_response` and `cwd`. A relative path is read relative to `cwd`, and a path outside the workspace is not recorded
- **It does not stop the AI's work.** If the JSON cannot be read, a file is missing, or a file cannot be handled (binary, CRLF), it prints the reason to standard error and exits with code 0 (code 2 is not used, because Claude Code would stop running the tool)
- It is registered in `.claude/settings.json` (written by `srwr init`):

```json
{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Read|Bash|Grep|Edit", "hooks": [{ "type": "command", "command": "srwr hook" }] }
    ]
  }
}
```

| Operation of Claude Code | Recorded on the tape as |
|---|---|
| Read | `select` (the whole file when there is no `offset` or `limit`) |
| A reading Bash command (`cat`, `nl`, `head`, `tail`, `sed -n 'A,Bp'`, `grep -n`) | `select` |
| Grep (content mode) | `select` (consecutive lines are put together into one) |
| Edit | `replace` (the range is the whole lines that contain the replaced place; with `replace_all`, one per match) |
| Write, MultiEdit, NotebookEdit | Not recorded (the next time srwr touches the file, it shows up as `external`) |

- After a Bash command (even one that does not read), every file whose content the tape holds is read again, and a difference is recorded as `external` (`detectedBy` is `hook`)
- After a Bash command, in a git work tree, a **new file** (one that git lists as untracked or as newly added to the index, including `git add -N`, that git does not ignore, and that is not in `.srwrignore`) is recorded as an `external` with `created: true`, so the tape shows what was made. A file with CRLF, a binary file, and a file over 256 KiB are left out. At most 50 new files are recorded per command; the rest are not, and the hook says so. Outside a git work tree new files are not found
- For an Edit, `old_string` → `new_string` is applied to the content before the edit (what the tape holds; if there is none, `tool_response.originalFile`), and if the result equals the current file it is a `replace`. If it does not (other changes are mixed in, for example), it is recorded as `external`
- A single call records at most 100 `select`s (so that a search over a wide range does not swell the tape)
- For a pipe, only the first command is looked at. `$( )`, redirects that write, `sed -i`, `tail -f` and `grep` without `-n` are not recorded
- A frame recorded by the hook is shown with a `why` of `null` (no `why` line). It has no selection token (`selection` and `from` are `null`). A token issued earlier by `srwr mcp` can still be used after a `replace` by the hook; its line numbers are corrected

## srwr view-server

The view server that an editor starts as a child process. It reads the tape and builds the data needed for stepping.

```
srwr view-server --root <workspace>
```

It is not for people to use directly. Those who add support for an editor read [protocol.md](protocol.md).

## srwr view

`srwr view [tape]` replays a tape in a plain Vim, with no plugin, using the Vim scripts embedded in the binary. If the tape is left out, you choose from the list. A tape is its ID or the path of a `.tape.jsonl` or `.tape.jsonl.gz` file. `--live` is live viewing. When running from outside the workspace, give `--root <workspace>`. Vim 9.0.0784 or later is needed. For how to use it, see [vim.md](vim.md).

## srwr init

```
srwr init [--lenient] [--root <workspace>]
```

Sets up the workspace for srwr. It gives the same result however many times it runs, and writes nothing when there is nothing to change.

| Item | Content |
|---|---|
| `.srwr/` | Makes the directory and the key |
| `.mcp.json` | Registers `srwr mcp` (existing servers are kept and this is added; if `srwr` is already there, it is left alone) |
| `.claude/settings.json` | Registers the hook, the setting that lets Claude Code use the tools without asking (`enabledMcpjsonServers`, and the permission of `select` and `replace`), and in strict mode the ban on Edit/Write (existing content is kept and this is added) |
| `.gitignore` | Adds `.srwr/key`, `.srwr/lock`, `.srwr/active` and `.srwr/init-backup/` (when under git). Tapes (`.srwr/tapes/`) are not ignored, so that they can be shared |

- It does not break existing files. The order of keys and the other settings, servers and hooks are kept as they are. The format is tidied to JSON with a 2-space indent
- What a file held before the rewrite is kept in `.srwr/init-backup/<date and time>/` (only when there is a file to change)
- If a file cannot be read as JSON (or the shape of an object or array is wrong), nothing is rewritten and it stops with exit code 1
- If `srwr` is not on the PATH, a note is printed at the end (`.mcp.json` and the hook run `srwr`)
- The only target agent is **Claude Code** (MCP itself can be used by other agents, but the ban on Edit/Write and the hook depend on the settings of Claude Code)

An example of the output (an empty workspace):

```
Workspace: /home/me/project

  created    .srwr/                key .srwr/key created
  created    .mcp.json             registered srwr mcp
  created    .claude/settings.json registered the hook; forbade Edit, Write, etc. (strict mode)
  created    .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

Ready. Reopen Claude Code and select / replace are available.
For lenient mode (Edit and Write stay allowed), run: srwr init --lenient.
```

### Strict mode and lenient mode

| Mode | Content |
|---|---|
| **Strict mode** (the default) | Forbids Edit, Write, MultiEdit and NotebookEdit of Claude Code. The only way for the AI to change a file is `select` / `replace`, so the tape always holds a `why` |
| **Lenient mode** (`srwr init --lenient`) | Does not forbid Edit and Write. The hook records an Edit as a `replace` (with a `why` of `null`). A Write is not recorded and shows up as `external` |

- Strict ⇄ lenient is switched with `srwr init` and `srwr init --lenient`. Lenient mode removes the four above (`Edit`, `Write`, `MultiEdit`, `NotebookEdit`) from `permissions.deny`. Other bans are left (a ban on `Edit` the user added by themselves has the same name, so it is removed too)
- Even strict mode cannot shut out everything. Editing through Bash (`sed -i`, redirects and so on) is detected and shown as `external`

## srwr tapes

```
srwr tapes [new | prune (--keep N | --older-than 30d) | path <tape ID> | check [<tape ID>]] [--root <workspace>]
```

| Command | Content |
|---|---|
| `srwr tapes` | A list (tape ID, start, last update, number of events, number of files, size, whether it is the current session). Newest first (by the time the tape started). Times are in the time zone of the machine |
| `srwr tapes new` | Closes the current session, and the next write starts a new session. The tape is [compressed](tape.md#a-closed-tape-is-compressed); if that fails, the tape stays as it was and a warning is printed. Selection tokens issued before the closing can no longer be used |
| `srwr tapes prune --keep N` / `--older-than 30d` | Deletes old tapes, compressed or not. `--keep N` keeps the newest N. `--older-than` takes the form `30d` or `12h`. It does not ask for confirmation, and prints each deleted tape on a line. **It does not delete the tape of the current session** |
| `srwr tapes path <id>` | Prints the path of a tape, `.tape.jsonl` or `.tape.jsonl.gz` (used when sharing) |
| `srwr tapes check [<id>]` | Lists the files that changed in the git work tree but are not on the tape. Without an ID it checks the current session (or the newest tape). It only reads |

There is no automatic cleanup. Deleting is done explicitly by the user.

### srwr tapes check

srwr records only the files that `select`, `replace` and the hook saw. A file the AI changed some other way (for example, written by a shell command) may not be on the tape. `check` compares the tape with `git status` and tells you.

- A file is **on the tape** if any event of the tape is about it
- A changed file that is not on the tape is a **warning**. A changed file that [is not recorded by the settings](#files-that-are-not-recorded) is counted apart and is not a warning. Anything inside `.srwr/` is left out
- If the work tree already had uncommitted changes when the tape started (the `dirty` of the header), the output says that some warnings may be older than the tape
- The exit code is 0 when there is no warning and 1 when there is. It needs git; outside a git work tree it prints why and exits with 1
- It does not take the lock and does not create `.srwr/`

```
Tape 20261004-1530-ab12 (current session)
Changed in the work tree but not on the tape (2):
  M    docs/a.md
  ??   newfile.txt
Changed, and not recorded by the settings (1): .env
On the tape: 3 files.
The work tree had uncommitted changes when this tape started, so some of these may be older than the tape.
```

An example of the output (the time zone is `Asia/Tokyo`; the date and time in a tape ID are UTC):

```
  Tape                Started       Last update   Events    Files     Size
  20261003-0812-k3f9  Oct 03 17:12  Oct 03 17:48  14        3          5.1 KB  <- current session
  20261002-0030-a8z1  Oct 02 09:30  Oct 02 11:05  62        7         21.4 KB

2 tapes (26.5 KB in all). Replay one with srwr view <tape>; share one with srwr tapes path <tape>.
```

## Files that are not recorded

A tape holds the whole text of files, and to share, the tape itself is handed over. So **files that contain secrets are not put on the tape**.

- **The default targets**: `.env`, `.env.*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`, and anything inside `.srwr/`
- **Additional targets**: `.srwrignore` in the workspace (the same format as `.gitignore`). `.gitignore` is not followed (it also holds generated files and the like, which the AI may handle)

| Situation | Behavior |
|---|---|
| `select` / `replace` | An `ignored_file` error. Nothing is written on the tape |
| The hook (Read, Bash, Grep, Edit) | Not recorded. Not subject to the detection of `external` either |
| The AI edited with Edit in lenient mode | Not recorded |

Details:

- **The default targets cannot be brought back with `!` in `.srwrignore`.** `!` can cancel only what `.srwrignore` itself specifies. If a directory is a target, the files in it are targets too and cannot be brought back with `!`
- **Case is not distinguished.** On macOS and Windows `.ENV` opens `.env`, so the judgment is the same on every OS
- **For a symbolic link, both the name of the link and its target are checked.** If `notes.txt` is a link to `.env`, it is not recorded
- **`.srwrignore` is read every time it is needed, and only the one directly under the workspace.** An addition takes effect at once. If you add a file that is already on the tape, from then on `select` and `replace` give `ignored_file` and the detection of `external` skips it
- **If `.srwrignore` cannot be read (no permission, or it is a directory), nothing is recorded.** `select` and `replace` give `internal_error`, and the hook prints one line to standard error

### Before sharing a tape

- Review `.srwrignore`
- Check the content of the tape (the list of `srwr tapes` shows the size and the number of files)
- The key (`.srwr/key`) is not needed for replay. Do not share it
