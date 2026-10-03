// The side panel "操作一覧": frames in recorded order, numbered from 1 (the number is the position in the status bar).
// No indentation for parents. A colored dot tells the kind: select blue, replace orange, changes from outside purple.
import * as vscode from "vscode";
import { pick } from "./lang";
import { basename, formatRange, Frame, Timeline } from "./timeline";

// What the list shows. A replay session and the live view implement it.
export interface OpsSource {
  readonly timeline: Timeline;
  readonly title: string;
  readonly onDidChange: vscode.Event<void>;
  current(): number; // the frame on the screen, -1 when none
  jump(i: number): Promise<void>; // a frame was clicked
}

export function dotColor(f: Frame): string {
  return f.kind === "select" ? "charts.blue" : f.kind === "replace" ? "charts.orange" : "charts.purple";
}

// The label of a kind: the words select and replace are the names of the two commands; external and final are srwr's own.
function kindLabel(kind: Frame["kind"]): string {
  return { select: "select", replace: "replace", external: pick("external", "外部変更"), final: pick("final", "録画後") }[kind];
}

export class OpsView implements vscode.TreeDataProvider<Frame>, vscode.Disposable {
  readonly view: vscode.TreeView<Frame>;

  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;

  private source: OpsSource | undefined;
  private sub: vscode.Disposable | undefined;

  constructor() {
    this.view = vscode.window.createTreeView<Frame>("srwr.ops", { treeDataProvider: this });
  }

  setSource(src: OpsSource | undefined): void {
    this.sub?.dispose();
    this.source = src;
    this.view.description = src?.title;
    this.sub = src?.onDidChange(() => this.refresh());
    void vscode.commands.executeCommand("setContext", "srwr.hasTape", src !== undefined);
    this.changed.fire();
  }

  private refresh(): void {
    this.changed.fire();
    const src = this.source;
    const f = src?.timeline.frames[src.current()];
    if (f) {
      // Selects the current frame and makes it visible.
      this.view.reveal(f, { select: true, focus: false }).then(undefined, () => undefined);
    }
  }

  getChildren(el?: Frame): Frame[] {
    return el ? [] : (this.source?.timeline.frames ?? []);
  }

  getParent(): undefined {
    return undefined;
  }

  getTreeItem(f: Frame): vscode.TreeItem {
    const where = formatRange(f.range);
    const item = new vscode.TreeItem(`${f.index + 1}  ${kindLabel(f.kind)}  ${basename(f.file)}:${where}`, vscode.TreeItemCollapsibleState.None);
    item.id = `${this.source?.title ?? ""}#${f.index}`;
    item.description = f.why ?? (f.kind === "external" ? pick("File changed outside srwr", "srwr の外でファイルが変わった") : "");
    item.iconPath = new vscode.ThemeIcon("circle-filled", new vscode.ThemeColor(dotColor(f)));
    item.tooltip = new vscode.MarkdownString().appendText([`${f.index + 1}  ${kindLabel(f.kind)}  ${f.file}:${where}`, f.why ?? ""].filter(Boolean).join("\n"));
    item.command = { command: "srwr.goto", title: pick("Go to this operation", "この操作へ移動"), arguments: [f.index] };
    return item;
  }

  dispose(): void {
    this.sub?.dispose();
    this.view.dispose();
    this.changed.dispose();
  }
}
