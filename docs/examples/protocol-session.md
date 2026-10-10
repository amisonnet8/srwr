# An example of an exchange with the view server

*[日本語](protocol-session_ja.md) | **English***

**Readers**: people who want to make srwr's display work in a new editor. See, in real output, what exchange a client has with `srwr view-server`. The rules are in [protocol.md](../reference/protocol.md).

This example was taken by actually running `srwr view-server`. The way the version is written and the update time of the tape, `updatedAt`, change with the environment.

## The subject

A workspace that has the tape `20261001-0806-1795`, which recorded the work of [look-edit.md](look-edit.md) (`internal/docs/testdata/demo-en/`; the date and time in the name are UTC). There are two operations (`look` and `edit`). The client starts `srwr view-server --root <workspace>`, writes to standard input one line at a time, and reads from standard output one line at a time. In what follows `→` is a line from the client and `←` is a line from the server.

## Replay

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"client":"vim","protocolVersion":3,"options":{"diffFrames":true}}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":3,"serverVersion":"(devel)"}}
→ {"jsonrpc":"2.0","id":2,"method":"tapes/list","params":{}}
← {"jsonrpc":"2.0","id":2,"result":{"tapes":[{"tapeId":"20261001-0806-1795","startedAt":"2026-10-01T08:06:15.381Z","updatedAt":"2026-10-01T08:06:21.026Z","ops":2,"files":["cmd/main.go"]}]}}
→ {"jsonrpc":"2.0","id":3,"method":"tape/open","params":{"tapeId":"20261001-0806-1795"}}
← {"jsonrpc":"2.0","id":3,"result":{"frames":[{"index":0,"kind":"look","seq":2,"ts":1790841975381,"file":"cmd/main.go","range":{"start":3,"end":5},"why":"Check whether a call to initialization can be added to main","selection":"sel_041061E48KVH3K24RN324MN2","from":null,"parent":null},{"index":1,"kind":"edit","seq":3,"ts":1790841975384,"file":"cmd/main.go","range":{"start":3,"end":6},"oldRange":{"start":3,"end":5},"why":"run needs the settings loaded first, so call setup","selection":"sel_041G61P48KVH3AYWAY6Q99GP","from":"sel_041061E48KVH3K24RN324MN2","parent":0}],"tapeId":"20261001-0806-1795"}}
→ {"jsonrpc":"2.0","id":4,"method":"frame/state","params":{"tapeId":"20261001-0806-1795","index":1}}
← {"jsonrpc":"2.0","id":4,"result":{"after":"package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n","before":"package main\n\nfunc main() {\n\trun()\n}\n","content":"package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n"}}
→ {"jsonrpc":"2.0","id":5,"method":"tape/close","params":{"tapeId":"20261001-0806-1795"}}
← {"jsonrpc":"2.0","id":5,"result":{}}
```

- `initialize` (id 1): passes the kind of the client and the setting (diff frames). A request before it gets `not_initialized`
- `tapes/list` (id 2): the list of tapes that have one or more operations. Times are in UTC (`…Z`); a client shows them in the time zone of the machine
- `tape/open` (id 3): **the frames**. Two frames come back. The `edit` frame (`index` 1) has `oldRange` (lines 3 to 5 before the change) and `range` (lines 3 to 6 after the change), and its `parent` points to the `look` frame (`index` 0). The real file is the same as the last content of the tape, so no `final` (final diff) frame is added
- `frame/state` (id 4): the whole text before (`before`) and after (`after`) the frame at `index` 1, and the content of the file at the time that frame has finished (`content`). A client that does not take the text of every frame with the frames asks for it with this, one frame at a time (VSCode and Vim open the tape with `withText: true` and build it themselves)
- `tape/close` (id 5): close

The shape of a frame (the fields used for the `why` line and so on) is in [protocol.md](../reference/protocol.md), and how to show it is in [vscode.md](../reference/vscode.md).

This exchange leaves out what the real clients add: `kinds` (which kinds of frames are sent; `failure` frames only when asked for), the `hidden` count and the `live/hidden` notification, `withText`, and the `title` of a tape in the list. They are described in [protocol.md](../reference/protocol.md).

## Errors

Opening a tape that does not exist returns a JSON-RPC error. `error.data.code` is the error code of srwr.

```jsonrpc
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"client":"vim","protocolVersion":3,"options":{}}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":3,"serverVersion":"(devel)"}}
→ {"jsonrpc":"2.0","id":2,"method":"tape/open","params":{"tapeId":"nothing"}}
← {"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"no such tape: nothing","data":{"code":"tape_not_found"}}}
→ {"jsonrpc":"2.0","id":3,"method":"shutdown","params":{}}
← {"jsonrpc":"2.0","id":3,"result":{}}
```

To finish, `shutdown`. The server exits after writing the reply.

## Live

After the reply to `live/start`, the server watches the tape and sends each appended frame as a notification `live/frame`, separately from any request. A notification has no `id`.

```
→ {"jsonrpc":"2.0","id":2,"method":"live/start","params":{}}
← {"jsonrpc":"2.0","id":2,"result":{"tapeId":"…","frames":[ …the frames so far… ]}}
← {"jsonrpc":"2.0","method":"live/frame","params":{"tapeId":"…","frame":{ …the appended frame… }}}
```

Live depends on time (it waits for the AI's operations), so it is not in this example. The shape of the notification is `live/frame` in [protocol.md](../reference/protocol.md).
