// Replay: a virtual document with the frame's content, the why rows above the range, and the left/right editors for diffs.
import * as vscode from "vscode";
import { pick } from "./lang";
import { Nav } from "./controls";
import { BANNER_PREFIX, insertBanner, wrapWhy } from "./lines";
import { OpsSource } from "./sidebar";
import { Presenter } from "./present";
import { basename, changedLines, formatRange, Frame, Hidden, isDiff, isSub, Timeline, splitLines, toneOf } from "./timeline";

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
    private hiddenCounts: Hidden = {},
  ) {
    this.title = tapeId;
    this.timeline = new Timeline(frames);
    provider.session = this;
  }

  current(): number {
    return this.index;
  }

  hidden(): Hidden {
    return this.hiddenCounts;
  }

  // Live: the count of the frames that were left out changed.
  setHidden(h: Hidden): void {
    this.hiddenCounts = h;
    this.fire();
  }

  // Live: adds a frame at the end.
  append(f: Frame): void {
    this.timeline.append(f);
    this.fire();
  }

  // Live: the view moved to another tape; start over.
  reset(): void {
    this.timeline.frames.length = 0;
    this.hiddenCounts = {};
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
      if (!f) {
        return "";
      }
      const text = m[2] === "before" ? f.before : f.after;
      // A sub frame has its why in a band above the text, on the right; on the left there are empty rows, so the lines line up.
      const band = isSub(f) ? subBand(f) : [];
      return band.length > 0 ? insertBanner(text, 1, m[2] === "before" ? band.map(() => "") : band) : text;
    }
    const fm = /(?:^|&)failure=(\d+)/.exec(uri.query);
    if (fm) {
      const f = this.timeline.frames[Number(fm[1])];
      return f ? failureText(f).join("\n") + "\n" : "";
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
    if (isDiff(f) || isSub(f)) {
      this.banner = undefined;
      await this.showDiff(f, g);
      if (g === this.gen) {
        this.fire();
      }
      return;
    }
    await this.closeAfterSide();
    if (f.kind === "failure") {
      this.banner = undefined;
      await this.showFailure(f, g);
      return;
    }

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

  // A failure frame: it has no file to open, so a document that explains it, with its first row in red.
  private async showFailure(f: Frame, g: number): Promise<void> {
    const label = failureTitle(f);
    const uri = vscode.Uri.from({ scheme: REPLAY_SCHEME, path: `/${this.title}/failure${f.index}/${label}`, query: `failure=${f.index}` });
    const doc = await vscode.workspace.openTextDocument(uri);
    const editor = await vscode.window.showTextDocument(doc, { preview: true, preserveFocus: true });
    if (g !== this.gen) {
      return;
    }
    this.presenter.show(editor, { range: { start: 1, end: 0 }, tone: "failure", banner: { line: 1, rows: 1 } });
    editor.revealRange(new vscode.Range(0, 0, 0, 0), vscode.TextEditorRevealType.AtTop);
    this.fire();
  }

  // A diff frame: before on the left (blue), after on the right (orange), only the changed lines painted.
  private async showDiff(f: Frame, g: number): Promise<void> {
    const uri = (side: "before" | "after"): vscode.Uri =>
      vscode.Uri.from({
        scheme: REPLAY_SCHEME,
        path: `/${this.title}/diff${f.index}/${side === "before" ? `${pick("Before", "前")} ${diffTitle(f)}` : `${pick("After", "後")} ${basename(f.file)}`}`,
        query: `diff=${f.index}&side=${side}`,
      });
    const left = await vscode.workspace.openTextDocument(uri("before"));
    const right = await vscode.workspace.openTextDocument(uri("after"));
    const leftEditor = await vscode.window.showTextDocument(left, { preview: true, preserveFocus: true });
    const rightEditor = await vscode.window.showTextDocument(right, { preview: true, preserveFocus: true, viewColumn: vscode.ViewColumn.Beside });
    if (g !== this.gen) {
      return;
    }
    const band = isSub(f) ? subBand(f).length : 0;
    const changed = changedLines(splitLines(f.before), splitLines(f.after));
    const below = (lines: number[]): number[] => lines.map((n) => n + band);
    this.presenter.clear();
    this.presenter.paintLines(leftEditor, "select", below(changed.before), band);
    this.presenter.paintLines(rightEditor, "replace", below(changed.after), band);
    // The first changed line goes about 30% from the top. A side with no changed line (only added or only removed)
    // follows the other side.
    const first = (changed.before[0] ?? changed.after[0] ?? 1) + band;
    revealNearTop(leftEditor, changed.before[0] !== undefined ? changed.before[0] + band : first);
    revealNearTop(rightEditor, changed.after[0] !== undefined ? changed.after[0] + band : first);
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

// The rows of the band above a sub frame: its why, wrapped. There is always at least one, even when the why is missing.
export function subBand(f: Frame): string[] {
  const rows = f.why ? wrapWhy(f.why, WHY_WIDTH) : [];
  return rows.length > 0 ? rows : [BANNER_PREFIX.trimEnd()];
}

// The heading of a diff frame (the left tab).
export function diffTitle(f: Frame): string {
  const name = basename(f.file);
  if (f.kind === "sub") {
    return `⚠ sub: ${name}`;
  }
  if (f.kind === "final") {
    return pick(
      `⚠ Changed after recording (${f.deleted ? "no longer exists" : "diff from current file"}): ${name}`,
      `⚠ 録画のあとで変更（${f.deleted ? "今は存在しない" : "今のファイルとの差分"}）：${name}`,
    );
  }
  return pick(`⚠ Changed outside srwr${f.deleted ? " (deleted)" : ""}: ${name}`, `⚠ srwrの外で変更${f.deleted ? "（削除）" : ""}：${name}`);
}

// The tab title of a failure frame.
export function failureTitle(f: Frame): string {
  return `${pick("failure", "失敗")}: ${f.tool ?? ""} (${f.code ?? ""})`;
}

// The rows of a failure frame: a red first row, the message, then what is known of the call. Plain text, so the tests can look at it.
export function failureText(f: Frame): string[] {
  const tool = f.tool ?? "";
  const rows = [
    pick(`\u2716 ${tool} failed (${f.code ?? ""})`, `\u2716 ${tool} が失敗しました (${f.code ?? ""})`),
    f.message ?? "",
    "",
  ];
  const known: Array<[string, string]> = [
    ["why", f.why ?? pick("(none)", "(なし)")],
    ["tool", tool],
    ["range", tool === "select" ? pick(`lines ${formatRange(f.range)}`, `${formatRange(f.range)} 行`) : "-"],
    ["file", f.file !== "" ? f.file : "(not shown)"],
  ];
  for (const [k, v] of known) {
    rows.push(`${k.padEnd(6)} ${v}`);
  }
  return rows;
}
