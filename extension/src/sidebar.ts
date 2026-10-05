// The side panel "操作一覧": frames in recorded order, numbered from 1 (the number is the position in the status bar).
// No indentation for parents. A colored dot tells the kind: select blue, replace orange, changes from outside purple.
import * as vscode from "vscode";
import { pick } from "./lang";
import { basename, formatRange, Frame, Hidden, Timeline } from "./timeline";

// What the list shows. A replay session and the live view implement it.
export interface OpsSource {
  readonly timeline: Timeline;
  readonly title: string;
  readonly onDidChange: vscode.Event<void>;
  hidden(): Hidden; // how many frames of each kind are left out (nothing when none)
  current(): number; // the frame on the screen, -1 when none
  jump(i: number): Promise<void>; // a frame was clicked
}

export function dotColor(f: Frame): string {
  return f.kind === "look" ? "charts.blue" : f.kind === "edit" || f.kind === "replace" || f.kind === "new" ? "charts.orange" : f.kind === "failure" ? "srwr.failureForeground" : "charts.purple";
}

// The label of a kind: look, edit, replace and new are the names of the four commands; external and final are srwr's own.
function kindLabel(kind: Frame["kind"]): string {
  return { look: "look", edit: "edit", replace: "replace", new: "new", external: pick("external", "外部変更"), final: pick("final", "録画後"), failure: pick("failure", "失敗") }[kind];
}

// "Hiding: failure (2)" under the heading of the list; "No frames to show" when nothing is there.
export function hiddenMessage(hidden: Hidden, shown: number): string {
  const parts = Object.entries(hidden)
    .filter(([, n]) => n > 0)
    .map(([k, n]) => `${k} (${n})`);
  const hiding = parts.length > 0 ? pick(`Hiding: ${parts.join(", ")}`, `隠している：${parts.join("、")}`) : "";
  const none = shown === 0 ? pick("No frames to show", "表示するコマがありません") : "";
  return [none, hiding].filter(Boolean).join(" \u00b7 ");
}

// What a row says where a frame has a file and a range: a failure has the error code instead.
export function rowPlace(f: Frame): string {
  if (f.kind === "replace") {
    const n = f.hits ?? 0;
    return `${basename(f.file)} (${pick(`${n} ${n === 1 ? "hit" : "hits"}`, `${n}か所`)})`;
  }
  return f.kind === "failure" ? (f.code ?? "") : `${basename(f.file)}:${formatRange(f.range)}`;
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
    this.view.message = src ? hiddenMessage(src.hidden(), src.timeline.length) : undefined;
    this.sub = src?.onDidChange(() => this.refresh());
    void vscode.commands.executeCommand("setContext", "srwr.hasTape", src !== undefined);
    this.changed.fire();
  }

  private refresh(): void {
    this.changed.fire();
    const src = this.source;
    this.view.message = src ? hiddenMessage(src.hidden(), src.timeline.length) : undefined;
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
    const where = rowPlace(f);
    const item = new vscode.TreeItem(`${f.index + 1}  ${kindLabel(f.kind)}  ${where}`, vscode.TreeItemCollapsibleState.None);
    item.id = `${this.source?.title ?? ""}#${f.index}`;
    item.description = f.why ?? (f.kind === "external" ? pick("File changed outside srwr", "srwr の外でファイルが変わった") : "");
    item.iconPath = new vscode.ThemeIcon("circle-filled", new vscode.ThemeColor(dotColor(f)));
    item.tooltip = new vscode.MarkdownString().appendText(
      [`${f.index + 1}  ${kindLabel(f.kind)}  ${f.kind === "failure" ? where : `${f.file}:${formatRange(f.range)}`}`, f.kind === "failure" ? (f.message ?? "") : "", f.why ?? ""].filter(Boolean).join("\n"),
    );
    item.command = { command: "srwr.goto", title: pick("Go to this operation", "この操作へ移動"), arguments: [f.index] };
    return item;
  }

  dispose(): void {
    this.sub?.dispose();
    this.view.dispose();
    this.changed.dispose();
  }
}
