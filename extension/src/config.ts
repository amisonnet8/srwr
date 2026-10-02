import * as vscode from "vscode";

// The only setting is srwr.path. There are no settings for how things look.
export function settings(): { path: string } {
  return { path: vscode.workspace.getConfiguration("srwr").get<string>("path", "srwr") };
}
