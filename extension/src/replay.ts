// Replay: a virtual document with the frame's content, the why rows above the range, and the left/right editors for diffs.
import * as vscode from "vscode";
import { Nav } from "./controls";
import { insertBanner, wrapWhy } from "./lines";
import { OpsSource } from "./sidebar";
import { Presenter } from "./present";
import { basename, changedLines, Frame, isDiff, Timeline, splitLines, toneOf } from "./timeline";

export const REPLAY_SCHEME = "srwr-replay";

// Display width the why is wrapped at.
const WHY_WIDTH = 100;

// Serves the content of the virtual documents from the session that is open.
export class ReplayProvider implements vscode.TextDocumentContentProvider {
  session: ReplaySession | undefined;
  private readonly emitter = new vscode.EventEmitter<vscode.Uri>();
  readonly onDidChange = this.emitter.event;

  provideTextDocumentContent(uri: vscode.Uri): string {
    return this.session?.contentOfUri(uri) ?? "";
  }

  changed(uri: vscode.Uri): void {
    this.emitter.fire(uri);
  }
}

export class ReplaySession implements OpsSource, Nav, vscode.Disposable {
  readonly timeline: Timeline;
  readonly title: string;
  readonly closeCommand = "srwr.closeTape";
  index = -1; // the frame on the screen; -1 until one is shown

  private readonly emitter = new vscode.EventEmitter<void>();
  readonly onDidChange = this.emitter.event;

  private banner: { file: string; at: number; rows: string[] } | undefined;
  private gen = 0; // grows with every goto, so an old goto stops halfway
  private chain: Promise<void> = Promise.resolve(); // the screen updates, one at a time

  constructor(
    tapeId: string,
    frames: Frame[],
    private readonly provider: ReplayProvider,
    private readonly presenter: Presenter,
  ) {
    this.title = tapeId;
    this.timeline = new Timeline(frames);
    provider.session = this;
  }

  current(): number {
    return this.index;
  }

  // Live: adds a frame at the end.
  append(f: Frame): void {
    this.timeline.append(f);
    this.fire();
  }

  // Live: the view moved to another tape; start over.
  reset(): void {
    this.timeline.frames.length = 0;
    this.index = -1;
    this.banner = undefined;
    this.gen++;
    this.fire();
  }

  // Live: sets the position without showing anything (the frames up to the start are only listed).
  setIndex(i: number): void {
    this.index = i;
    this.fire();
  }

  position(): { index: number; length: number } {
    return { index: this.index, length: this.timeline.length };
  }

  canBack(): boolean {
    return this.index > 0;
  }

  canForward(): boolean {
    return this.index < this.timeline.length - 1;
  }

  behind(): undefined {
    return undefined;
  }

  stepForward(): Promise<void> {
    return this.goto(this.index + 1);
  }

  stepBack(): Promise<void> {
    return this.goto(this.index - 1);
  }

  jump(i: number): Promise<void> {
    return this.goto(i);
  }

  // The content of a virtual document of this session.
  contentOfUri(uri: vscode.Uri): string {
    const m = /(?:^|&)diff=(\d+)&side=(before|after)/.exec(uri.query);
    if (m) {
      const f = this.timeline.frames[Number(m[1])];
      return f ? (m[2] === "before" ? f.before : f.after) : "";
    }
    return this.contentFor(uri.path.split("/").slice(2).join("/"));
  }

  // What to show for `file` now: its content at this frame, with the why rows put in when they belong to this file.
  private contentFor(file: string): string {
    const text = this.timeline.contentAt(file, this.index) ?? "";
    const b = this.banner;
    return b && b.file === file ? insertBanner(text, b.at, b.rows) : text;
  }

  // Shows frame i. The position changes at once. The screen is updated one frame at a time, in order: a frame that
  // was asked for and replaced by a later one before its turn is skipped, so quick steps end on the last one asked for,
  // and an older step can never open an editor after a newer one cleaned up.
  goto(i: number): Promise<void> {
    const tl = this.timeline;
    if (tl.length === 0) {
      return this.chain;
    }
    i = Math.min(Math.max(i, 0), tl.length - 1);
    const g = ++this.gen;
    this.index = i;
    const run = this.chain.then(() => (g === this.gen ? this.render(i, g) : undefined));
    this.chain = run.catch(() => undefined);
    return run;
  }

  private async render(i: number, g: number): Promise<void> {
    const f = this.timeline.frames[i];
    if (isDiff(f)) {
      this.banner = undefined;
      await this.showDiff(f, g);
      if (g === this.gen) {
        this.fire();
      }
      return;
    }
    await this.closeAfterSide();

    const rows = f.why ? wrapWhy(f.why, WHY_WIDTH) : [];
    this.banner = rows.length > 0 ? { file: f.file, at: f.range.start, rows } : undefined;
    const editor = await this.open(f.file);
    if (g !== this.gen) {
      return;
    }
    const n = rows.length;
    // The why rows push the range down.
    const range = { start: f.range.start + n, end: f.range.end + n };
    const shown = this.presenter.show(editor, { range, tone: toneOf(f), banner: n > 0 ? { line: f.range.start, rows: n } : undefined });
    if (n > 0) {
      this.presenter.showLineNumbers(editor, f.range.start, n);
    }
    editor.revealRange(shown, vscode.TextEditorRevealType.InCenterIfOutsideViewport);
    this.fire();
  }

  // A diff frame: before on the left (blue), after on the right (orange), only the changed lines painted.
  private async showDiff(f: Frame, g: number): Promise<void> {
    const uri = (side: "before" | "after"): vscode.Uri =>
      vscode.Uri.from({
        scheme: REPLAY_SCHEME,
        path: `/${this.title}/diff${f.index}/${side === "before" ? `前 ${diffTitle(f)}` : `後 ${basename(f.file)}`}`,
        query: `diff=${f.index}&side=${side}`,
      });
    const left = await vscode.workspace.openTextDocument(uri("before"));
    const right = await vscode.workspace.openTextDocument(uri("after"));
    const leftEditor = await vscode.window.showTextDocument(left, { preview: true, preserveFocus: true });
    const rightEditor = await vscode.window.showTextDocument(right, { preview: true, preserveFocus: true, viewColumn: vscode.ViewColumn.Beside });
    if (g !== this.gen) {
      return;
    }
    const changed = changedLines(splitLines(f.before), splitLines(f.after));
    this.presenter.clear();
    this.presenter.paintLines(leftEditor, "select", changed.before);
    this.presenter.paintLines(rightEditor, "replace", changed.after);
    // The first changed line goes about 30% from the top. A side with no changed line (only added or only removed)
    // follows the other side.
    const first = changed.before[0] ?? changed.after[0] ?? 1;
    revealNearTop(leftEditor, changed.before[0] ?? first);
    revealNearTop(rightEditor, changed.after[0] ?? first);
  }

  // Closes the right-hand editor, when going from a diff frame back to a normal one. Tabs do not pile up.
  // It looks at the tabs every time, not at a flag: a diff frame that was cut short after its editors opened still leaves them.
  private async closeAfterSide(): Promise<void> {
    for (const group of vscode.window.tabGroups.all) {
      for (const tab of group.tabs) {
        const input = tab.input;
        if (input instanceof vscode.TabInputText && input.uri.scheme === REPLAY_SCHEME && input.uri.query.includes("side=after")) {
          await vscode.window.tabGroups.close(tab);
        }
      }
    }
  }

  // Opens the virtual document and makes its content match the current frame.
  private async open(file: string): Promise<vscode.TextEditor> {
    const doc = await vscode.workspace.openTextDocument(this.uriFor(file));
    const editor = await vscode.window.showTextDocument(doc, { preview: true, preserveFocus: true });
    await this.sync(file);
    return editor;
  }

  // When the open document differs from what the frame wants, asks for an update and waits for it.
  private async sync(file: string): Promise<void> {
    const uri = this.uriFor(file);
    const want = this.contentFor(file);
    const doc = vscode.workspace.textDocuments.find((d) => d.uri.toString() === uri.toString());
    if (!doc || doc.getText().replace(/\r\n/g, "\n") === want) {
      return;
    }
    const updated = new Promise<void>((resolve) => {
      const sub = vscode.workspace.onDidChangeTextDocument((e) => {
        if (e.document.uri.toString() === uri.toString()) {
          sub.dispose();
          resolve();
        }
      });
      setTimeout(() => {
        sub.dispose();
        resolve();
      }, 500);
    });
    this.provider.changed(uri);
    await updated;
  }

  private uriFor(file: string): vscode.Uri {
    return vscode.Uri.from({ scheme: REPLAY_SCHEME, path: `/${this.title}/${file}` });
  }

  private fire(): void {
    this.emitter.fire();
  }

  dispose(): void {
    this.gen++;
    if (this.provider.session === this) {
      this.provider.session = undefined;
    }
    this.presenter.clear();
    void this.closeAfterSide();
    this.emitter.dispose();
  }
}

// Scrolls so that line `line` (1-based) is about 30% from the top.
export function revealNearTop(editor: vscode.TextEditor, line: number): void {
  const v = editor.visibleRanges[0];
  const rows = v ? v.end.line - v.start.line : 30;
  const top = Math.max(line - 1 - Math.round(rows * 0.3), 0);
  const p = new vscode.Position(top, 0);
  editor.revealRange(new vscode.Range(p, p), vscode.TextEditorRevealType.AtTop);
}

// The heading of a diff frame (the left tab).
export function diffTitle(f: Frame): string {
  const name = basename(f.file);
  if (f.kind === "final") {
    return `⚠ 録画のあとで変更（${f.deleted ? "今は存在しない" : "今のファイルとの差分"}）：${name}`;
  }
  return `⚠ srwrの外で変更${f.deleted ? "（削除）" : ""}：${name}`;
}
