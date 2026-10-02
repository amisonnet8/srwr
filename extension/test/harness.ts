import { activate } from "../src/extension";
import { ServerClient } from "../src/server";
import { Uri, reset, screen, state } from "./fakevscode";

// Starts the extension on the fake vscode. `run` runs a command the way the UI would and waits for it.
export function start(server: ServerClient | (() => ServerClient), root = "/ws") {
  reset();
  state.workspaceFolders = [{ uri: Uri.file(root) }];
  const subs: Array<{ dispose(): void }> = [];
  let made = 0;
  const api = activate({ subscriptions: subs } as never, () => {
    made++;
    return typeof server === "function" ? server() : server;
  });
  return {
    api,
    screen,
    get serversMade() {
      return made;
    },
    run: async (cmd: string, ...args: unknown[]): Promise<unknown> => {
      const fn = state.commands.get(cmd);
      if (!fn) {
        throw new Error(`no command ${cmd}`);
      }
      return fn(...args);
    },
    dispose: () => {
      for (const s of subs.reverse()) {
        s.dispose();
      }
    },
  };
}
