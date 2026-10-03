import * as vscode from "vscode";
import { settings } from "./config";
import { Controls } from "./controls";
import { setJapanese, pick } from "./lang";
import { LiveView } from "./live";
import { Presenter } from "./present";
import { REPLAY_SCHEME, ReplayProvider, ReplaySession } from "./replay";
import { ServerClient, ServerError, ServerProcess, TapeInfo, resolveCommand } from "./server";
import { OpsView } from "./sidebar";
import { localStamp } from "./times";

const TAPE_SUFFIX = ".tape.jsonl";

// Makes the client for a server. Tests pass a fake.
export type ServerFactory = (root: string, command: string, log: (line: string) => void) => ServerClient;

const defaultServerFactory: ServerFactory = (root, command, log) => new ServerProcess({ root, command: () => command, log });

// What activate gives back. Only tests use it.
export interface Api {
  // Resolves when the live view's screen has caught up.
  settled(): Promise<void>;
}

export function activate(context: vscode.ExtensionContext, createServer: ServerFactory = defaultServerFactory): Api {
  setJapanese(vscode.env.language.toLowerCase().startsWith("ja"));
  const output = vscode.window.createOutputChannel("srwr");
  const presenter = new Presenter();
  const provider = new ReplayProvider();
  const ops = new OpsView();
  const controls = new Controls();
  let replay: ReplaySession | undefined;
  let live: LiveView | undefined;
  const active = (): ReplaySession | LiveView | undefined => replay ?? live;

  const workspaceRoot = (): string | undefined => vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  // The server starts when it is first used, one per workspace. A new workspace or a new srwr.path makes a new one.
  let server: { client: ServerClient; root: string; command: string } | undefined;
  const serverFor = (root: string): ServerClient => {
    const command = resolveCommand(settings().path, process.env);
    if (server && (server.root !== root || server.command !== command)) {
      server.client.dispose();
      server = undefined;
    }
    server ??= { client: createServer(root, command, (line) => output.appendLine(line)), root, command };
    return server.client;
  };

  const showServerError = async (e: unknown): Promise<void> => {
    if (e instanceof ServerError && e.code === "binary_not_found") {
      const openSettings = pick("Open Settings", "設定を開く");
      const choice = await vscode.window.showErrorMessage(
        pick(
          `srwr: ${e.message}\nInstall srwr (go install github.com/amisonnet8/srwr/cmd/srwr@latest) or set its location in the setting "srwr.path".`,
          `srwr: ${e.message}\nsrwr を入れる（go install github.com/amisonnet8/srwr/cmd/srwr@latest）か、設定「srwr.path」に場所を指定してください。`,
        ),
        openSettings,
      );
      if (choice === openSettings) {
        await vscode.commands.executeCommand("workbench.action.openSettings", "srwr.path");
      }
      return;
    }
    void vscode.window.showErrorMessage(`srwr: ${shownMessage(e)}`);
  };

  const stopLive = (): void => {
    if (live) {
      live.dispose();
      live = undefined;
      controls.bind(undefined);
      ops.setSource(undefined);
    }
  };
  const closeReplay = (): void => {
    if (replay) {
      const tapeId = replay.title;
      server?.client.closeTape(tapeId).catch(() => undefined); // the server may already be gone
      replay.dispose();
      replay = undefined;
      controls.bind(undefined);
      ops.setSource(undefined);
    }
  };

  const openTape = async (): Promise<void> => {
    const root = workspaceRoot();
    if (!root) {
      void vscode.window.showWarningMessage(pick("srwr: Open a folder first", "srwr: フォルダを開いてから実行してください"));
      return;
    }
    let tapes: TapeInfo[];
    try {
      tapes = await serverFor(root).listTapes();
    } catch (e) {
      await showServerError(e);
      return;
    }
    if (tapes.length === 0) {
      void vscode.window.showInformationMessage(pick("srwr: No tape with recorded operations yet (.srwr/tapes/)", "srwr: 操作を記録したテープがありません（.srwr/tapes/）"));
      return;
    }
    const items = tapes.map((t) => ({
      label: startedLabel(t),
      description: `${pick(`${t.ops} ${t.ops === 1 ? "operation" : "operations"}`, `${t.ops}操作`)} · ${t.files.join(", ")}`,
      detail: t.tapeId + TAPE_SUFFIX,
      tape: t,
    }));
    const chosen = await vscode.window.showQuickPick(items, { placeHolder: pick("Pick a tape to replay", "再生するテープを選ぶ") });
    if (!chosen) {
      return;
    }
    stopLive();
    closeReplay();
    let frames;
    try {
      frames = await serverFor(root).openTape(chosen.tape.tapeId);
    } catch (e) {
      await showServerError(e);
      return;
    }
    replay = new ReplaySession(chosen.tape.tapeId, frames, provider, presenter);
    ops.setSource(replay);
    controls.bind(replay);
    await replay.goto(0);
  };

  context.subscriptions.push(
    presenter,
    ops,
    controls,
    vscode.workspace.registerTextDocumentContentProvider(REPLAY_SCHEME, provider),

    vscode.commands.registerCommand("srwr.openTape", openTape),
    // The panel's close button calls this too: it closes whichever of replay and live is open.
    vscode.commands.registerCommand("srwr.closeTape", () => {
      closeReplay();
      stopLive();
    }),
    vscode.commands.registerCommand("srwr.stepForward", () => active()?.stepForward()),
    vscode.commands.registerCommand("srwr.stepBack", () => active()?.stepBack()),
    vscode.commands.registerCommand("srwr.liveLatest", () => live?.latest()),
    vscode.commands.registerCommand("srwr.goto", (index: number) => active()?.jump(index)),

    vscode.commands.registerCommand("srwr.liveStart", () => {
      const root = workspaceRoot();
      if (!root) {
        void vscode.window.showWarningMessage(pick("srwr: Open a folder first", "srwr: フォルダを開いてから実行してください"));
        return;
      }
      closeReplay();
      stopLive();
      const view = new LiveView(presenter, serverFor(root), provider);
      live = view;
      ops.setSource(view);
      controls.bind(view);
      return view.start().catch((e) => {
        if (live === view) {
          stopLive();
        }
        return showServerError(e);
      });
    }),
    vscode.commands.registerCommand("srwr.liveStop", stopLive),

    output,
    {
      dispose: () => {
        stopLive();
        closeReplay();
        server?.client.dispose();
        server = undefined;
      },
    },
  );
  return { settled: () => live?.settled() ?? Promise.resolve() };
}

export function deactivate(): void {}

// What a person is told of an error: the view server's messages are in English and meant for developers, so the ones a person
// can meet (no such tape, an unreadable tape) are said again here in the language of the screen.
function shownMessage(e: unknown): string {
  if (e instanceof ServerError && e.code === "tape_not_found") {
    return pick("Tape not found", "テープが見つからない");
  }
  if (e instanceof ServerError && e.code === "tape_unreadable") {
    return pick("Cannot read the tape", "テープを読めない");
  }
  return e instanceof Error ? e.message : String(e);
}

// The heading of a tape in the picker: "2026-09-29 18:37:12", in the time zone of the machine. Without a header, the name of the tape file.
function startedLabel(t: TapeInfo): string {
  return (t.startedAt && localStamp(t.startedAt)) || t.tapeId + TAPE_SUFFIX;
}
