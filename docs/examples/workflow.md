# An example of a whole round: from `srwr init` to tidying up

*[日本語](workflow_ja.md) | **English***

**Readers**: people who start to use srwr. One small project is taken through the whole round: set up, let the AI work, look at what it did, check that nothing changed behind the tape, start a new session, and tidy up. The rules of each command are in [cli.md](../reference/cli.md).

Every command here was run for real. Only the last four characters of a tape ID, the sizes, and the place of the workspace change from run to run.

## The subject

A small Go project in a git work tree, with `main.go` and `README.md`. `main.go` is the file the AI works on:

```go
package main

import "fmt"

func main() {
	fmt.Println(greet(""))
}

func greet(name string) string {
	return "hello " + name
}
```

## 1. Set up once

In the project, run `srwr init`. It registers srwr for Claude Code and forbids the Edit and Write tools, so that every change goes through `select` and `replace`. What it writes is in [cli.md](../reference/cli.md#srwr-init).

```console
$ srwr init
Workspace: /work

  created    .srwr/                key .srwr/key created
  created    .mcp.json             registered srwr mcp
  created    .claude/settings.json registered the hook; forbade Edit, Write, etc. (strict mode)
  created    .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

Ready. Reopen Claude Code and select / replace are available.
For lenient mode (Edit and Write stay allowed), run: srwr init --lenient.
```

Reopen Claude Code, so that it reads the new settings. Commit what `srwr init` wrote: the tapes (`.srwr/tapes/`) are not ignored, so that they can be shared.

## 2. Let the AI work

Ask for the change as you always do, for example: "`greet` prints `hello ` when the name is empty. Fix it." The AI looks with `select` and changes with `replace`, each with a reason, and every call goes on a tape in `.srwr/tapes/`. The AI's side of this is in [select-replace.md](select-replace.md).

`srwr tapes` lists the tapes of the project:

```console
$ srwr tapes
  Tape                Started       Last update   Events    Files     Size
  20261004-1200-7k7f  Oct 04 12:00  Oct 04 12:00  3         1          1.9 KB  <- current session

1 tape (1.9 KB in all). Replay one with srwr view <tape>; share one with srwr tapes path <tape>.
```

## 3. Look at what it did

Open the tape in an editor and step through it frame by frame, with the reason above the lines: `srwr view` in Vim ([vim.md](../reference/vim.md)), or the *Operations* view of the extension in VSCode ([vscode.md](../reference/vscode.md)). `srwr view --live` follows a tape that is being written.

## 4. Check that nothing changed behind the tape

srwr records what goes through `select` and `replace`, and what the hook sees. A file changed some other way, by hand or by a shell command, may not be on the tape. `srwr tapes check` compares the tape with `git status`. Here the README was edited by hand:

```console
$ srwr tapes check
Tape 20261004-1200-7k7f (current session)
Changed in the work tree but not on the tape (1):
  M    README.md
On the tape: 1 file.
```

The exit code is 1 when there is a warning, 0 when there is none. It only reads: it takes no lock and writes nothing.

## 5. Start a new session

A tape is one unit of work. To start the next one, run `srwr tapes new`:

```console
$ srwr tapes new
Closed the current session 20261004-1200-7k7f. The next write starts a new tape.
```

- The next write starts a new tape. A session also ends by itself when 30 minutes pass without an event
- The tape of the session that ended is **compressed** to `<id>.tape.jsonl.gz`. Everything that reads tapes opens it as before
- The selection tokens the AI got in the old session stop working. If the AI is in the middle of a task, it calls `select` again
- The editors cannot start a new session: it is done here, in the terminal

When the AI works again, the new session has its own tape:

```console
$ srwr tapes
  Tape                Started       Last update   Events    Files     Size
  20261004-1300-p2q9  Oct 04 13:00  Oct 04 13:00  3         1          1.9 KB  <- current session
  20261004-1200-7k7f  Oct 04 12:00  Oct 04 12:00  3         1          1.0 KB

2 tapes (2.9 KB in all). Replay one with srwr view <tape>; share one with srwr tapes path <tape>.
```

`srwr tapes path` prints where a tape is, which is the file you hand over to share it. The old tape is now the compressed file:

```console
$ srwr tapes path 20261004-1200-7k7f
/work/.srwr/tapes/20261004-1200-7k7f.tape.jsonl.gz
```

## 6. Tidy up

There is no automatic cleanup. `srwr tapes prune` deletes old tapes, compressed or not, and never the tape of the current session:

```console
$ srwr tapes prune --keep 1
Deleted  20261004-1200-7k7f  (1.0 KB)
Deleted 1 tape. 1 left (1.9 KB).
```

## Related

- The commands: [cli.md](../reference/cli.md)
- What is on a tape, and how a closed one is stored: [tape.md](../reference/tape.md)
- The tools the AI uses: [mcp.md](../reference/mcp.md)
