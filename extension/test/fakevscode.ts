// A fake `vscode` module with just what the extension uses. It records what would be on the screen (documents, decorations,
// editors, the tree, the status bar, messages), so a test can look at it. It draws nothing: colors and layout are only recorded.
/* eslint-disable @typescript-eslint/no-explicit-any */

type Listener<T> = (e: T) => unknown;

export class EventEmitter<T> {
  private readonly ls = new Set<Listener<T>>();
  readonly event = (l: Listener<T>): { dispose(): void } => {
    this.ls.add(l);
    return { dispose: () => this.ls.delete(l) };
  };
  fire(e: T): void {
    for (const l of [...this.ls]) {
      l(e);
    }
  }
  dispose(): void {
    this.ls.clear();
  }
}

export class Position {
  constructor(
    readonly line: number,
    readonly character: number,
  ) {}
}

export class Range {
  constructor(
    readonly start: Position,
    readonly end: Position,
  ) {}
}

export class ThemeColor {
  constructor(readonly id: string) {}
}

export class ThemeIcon {
  constructor(
    readonly id: string,
    readonly color?: ThemeColor,
  ) {}
}

export class MarkdownString {
  value = "";
  appendText(s: string): this {
    this.value += s;
    return this;
  }
}

export class TreeItem {
  id?: string;
  description?: string | boolean;
  iconPath?: ThemeIcon;
  tooltip?: string | MarkdownString;
  command?: { command: string; title: string; arguments?: unknown[] };
  constructor(
    public label: string,
    public collapsibleState?: number,
  ) {}
}

export const TreeItemCollapsibleState = { None: 0, Collapsed: 1, Expanded: 2 };
export const StatusBarAlignment = { Left: 1, Right: 2 };
export const TextEditorLineNumbersStyle = { Off: 0, On: 1, Relative: 2 };
export const TextEditorRevealType = { Default: 0, InCenter: 1, InCenterIfOutsideViewport: 2, AtTop: 3 };
export const ViewColumn = { Active: -1, Beside: -2, One: 1, Two: 2 };

export class Uri {
  private constructor(
    readonly scheme: string,
    readonly path: string,
    readonly query: string,
  ) {}
  static from(c: { scheme: string; path?: string; query?: string }): Uri {
    return new Uri(c.scheme, c.path ?? "", c.query ?? "");
  }
  static file(p: string): Uri {
    return new Uri("file", p, "");
  }
  get fsPath(): string {
    return this.path;
  }
  // Not percent-encoded, so that a test reads the same text the user sees.
  toString(): string {
    return `${this.scheme}:${this.path}${this.query ? `?${this.query}` : ""}`;
  }
}

export class TabInputText {
  constructor(readonly uri: Uri) {}
}

// --- documents and editors ---

class TextLine {
  readonly range: Range;
  constructor(
    line: number,
    readonly text: string,
  ) {
    this.range = new Range(new Position(line, 0), new Position(line, text.length));
  }
}

export class TextDocument {
  private text: string;
  constructor(
    readonly uri: Uri,
    text: string,
  ) {
    this.text = text;
  }
  setText(t: string): void {
    this.text = t;
  }
  getText(): string {
    return this.text;
  }
  get lineCount(): number {
    return this.text.split("\n").length;
  }
  lineAt(n: number): TextLine {
    return new TextLine(n, this.text.split("\n")[n] ?? "");
  }
}

interface Decorated {
  line: number;
  before?: string;
}

export class TextEditor {
  private opts: { lineNumbers?: number };
  // Like the real editor, a setting given to the editor in a place (view column) is carried to the next editor opened there.
  get options(): { lineNumbers?: number } {
    return this.opts;
  }
  set options(o: { lineNumbers?: number }) {
    this.opts = { ...this.opts, ...o };
    state.carry.set(this.viewColumn, { ...this.opts });
  }
  reveal: number | null = null;
  revealType: number | null = null;
  readonly visibleRanges = [new Range(new Position(0, 0), new Position(40, 0))];
  readonly decorations = new Map<DecorationType, Decorated[]>();
  constructor(
    readonly document: TextDocument,
    public viewColumn: number,
  ) {
    this.opts = { lineNumbers: 1, ...state.carry.get(viewColumn) };
  }
  setDecorations(type: DecorationType, ranges: Array<Range | { range: Range; renderOptions?: { before?: { contentText?: string } } }>): void {
    if (ranges.length > 0 && !state.paintOrder.includes(type)) {
      state.paintOrder.push(type);
    }
    this.decorations.set(
      type,
      ranges.map((r) => {
        if (r instanceof Range) {
          return { line: r.start.line };
        }
        const before = r.renderOptions?.before?.contentText;
        return before === undefined ? { line: r.range.start.line } : { line: r.range.start.line, before };
      }),
    );
  }
  revealRange(r: Range, type?: number): void {
    this.reveal = r.start.line + 1;
    this.revealType = type ?? 0;
  }
}

export interface DecorationType {
  opts: Record<string, unknown>;
  dispose(): void;
}

class StatusBarItem {
  text = "";
  tooltip: string | undefined;
  command: string | undefined;
  color: string | undefined;
  backgroundColor: ThemeColor | undefined;
  visible = false;
  constructor(
    readonly alignment: number,
    readonly priority: number,
  ) {}
  show(): void {
    this.visible = true;
  }
  hide(): void {
    this.visible = false;
  }
  dispose(): void {
    this.visible = false;
    state.statusItems.delete(this);
  }
}

// --- the recorded state, and the knobs a test turns ---

export const state = {
  documents: new Map<string, TextDocument>(),
  providers: new Map<string, { provideTextDocumentContent(uri: Uri): string; onDidChange?: (l: (u: Uri) => void) => unknown }>(),
  groups: new Map<number, TextEditor>(), // view column -> the one editor (preview) in it
  statusItems: new Set<StatusBarItem>(),
  trees: new Map<string, { provider: any; view: any }>(),
  commands: new Map<string, (...a: any[]) => unknown>(),
  contexts: new Map<string, unknown>(),
  messages: [] as Array<{ level: "error" | "warning" | "info"; text: string; buttons: string[] }>,
  quickPick: undefined as { placeHolder?: string; items: Array<{ label: string; description?: string; detail?: string }> } | undefined,
  executed: [] as Array<{ command: string; args: unknown[] }>,
  decorationTypes: [] as DecorationType[],
  carry: new Map<number, { lineNumbers?: number }>(), // editor options carried over in each view column
  opens: 0, // how many times a document was opened
  paintOrder: [] as DecorationType[], // decoration types in the order they were first painted (a screen lists them in this order)
  // knobs
  language: "en", // vscode.env.language; SRWR_TEST_LANG=ja starts a run in Japanese (the screens are captured in both)
  config: { path: "srwr" } as Record<string, string>,
  workspaceFolders: undefined as Array<{ uri: Uri }> | undefined,
  pickQuickPick: ((items: any[]) => items[0]) as (items: any[]) => any,
  pickMessageButton: (() => undefined) as (text: string, buttons: string[]) => string | undefined,
  onDidChangeTextDocument: new EventEmitter<{ document: TextDocument }>(),
};

export function reset(): void {
  state.documents.clear();
  state.providers.clear();
  state.groups.clear();
  state.statusItems.clear();
  state.trees.clear();
  state.commands.clear();
  state.contexts.clear();
  state.messages.length = 0;
  state.quickPick = undefined;
  state.executed.length = 0;
  state.decorationTypes.length = 0;
  state.paintOrder.length = 0;
  state.carry.clear();
  state.opens = 0;
  state.language = process.env.SRWR_TEST_LANG ?? "en";
  state.config = { path: "srwr" };
  state.workspaceFolders = undefined;
  state.pickQuickPick = (items) => items[0];
  state.pickMessageButton = () => undefined;
}

// --- the API ---

const tabGroups = {
  get all(): Array<{ tabs: Array<{ input: TabInputText }> }> {
    return [...state.groups.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([, ed]) => ({ tabs: [{ input: new TabInputText(ed.document.uri), editor: ed } as any] }));
  },
  async close(tab: { editor?: TextEditor }): Promise<boolean> {
    for (const [col, ed] of state.groups) {
      if (ed === tab.editor) {
        state.groups.delete(col);
        return true;
      }
    }
    return false;
  },
};

function makeDocument(uri: Uri): TextDocument {
  const p = state.providers.get(uri.scheme);
  if (!p) {
    throw new Error(`no content provider for ${uri.scheme}`);
  }
  return new TextDocument(uri, p.provideTextDocumentContent(uri));
}

export const window = {
  tabGroups,
  createOutputChannel: (_name: string) => ({ appendLine: (_s: string) => undefined, dispose: () => undefined }),
  createTextEditorDecorationType(opts: Record<string, unknown>): DecorationType {
    const t: DecorationType = { opts, dispose: () => undefined };
    state.decorationTypes.push(t);
    return t;
  },
  createTreeView(id: string, o: { treeDataProvider: any }) {
    const view = { description: undefined as string | undefined, reveal: async () => undefined, dispose: () => state.trees.delete(id) };
    state.trees.set(id, { provider: o.treeDataProvider, view });
    return view;
  },
  createStatusBarItem(alignment: number, priority: number): StatusBarItem {
    const it = new StatusBarItem(alignment, priority);
    state.statusItems.add(it);
    return it;
  },
  async showTextDocument(doc: TextDocument, o?: { viewColumn?: number }): Promise<TextEditor> {
    const column = o?.viewColumn === ViewColumn.Beside ? 2 : o?.viewColumn && o.viewColumn > 0 ? o.viewColumn : 1;
    const cur = state.groups.get(column);
    if (cur && cur.document === doc) {
      return cur;
    }
    const ed = new TextEditor(doc, column);
    state.groups.set(column, ed); // a preview tab is replaced
    return ed;
  },
  async showQuickPick(items: any[], o?: { placeHolder?: string }) {
    state.quickPick = {
      placeHolder: o?.placeHolder,
      items: items.map((i) => ({ label: i.label, description: i.description, detail: i.detail })),
    };
    return state.pickQuickPick(items);
  },
  showErrorMessage: (text: string, ...buttons: string[]) => message("error", text, buttons),
  showWarningMessage: (text: string, ...buttons: string[]) => message("warning", text, buttons),
  showInformationMessage: (text: string, ...buttons: string[]) => message("info", text, buttons),
};

async function message(level: "error" | "warning" | "info", text: string, buttons: string[]): Promise<string | undefined> {
  state.messages.push({ level, text, buttons });
  return state.pickMessageButton(text, buttons);
}

export const workspace = {
  get workspaceFolders() {
    return state.workspaceFolders;
  },
  get textDocuments(): TextDocument[] {
    return [...state.documents.values()];
  },
  onDidChangeTextDocument: state.onDidChangeTextDocument.event,
  async openTextDocument(uri: Uri): Promise<TextDocument> {
    state.opens++;
    const key = uri.toString();
    let doc = state.documents.get(key);
    if (!doc) {
      doc = makeDocument(uri);
      state.documents.set(key, doc);
    }
    return doc;
  },
  registerTextDocumentContentProvider(scheme: string, p: { provideTextDocumentContent(uri: Uri): string; onDidChange?: (l: (u: Uri) => void) => unknown }) {
    state.providers.set(scheme, p);
    const sub = p.onDidChange?.((uri) => {
      const doc = state.documents.get(uri.toString());
      if (doc) {
        doc.setText(p.provideTextDocumentContent(uri));
        setImmediate(() => state.onDidChangeTextDocument.fire({ document: doc }));
      }
    });
    return { dispose: () => void (sub as { dispose?: () => void } | undefined)?.dispose?.() };
  },
  getConfiguration: (_section: string) => ({
    get: <T>(key: string, dflt: T): T => (state.config[key] as unknown as T) ?? dflt,
  }),
};

export const env = {
  get language(): string {
    return state.language;
  },
};

export const commands = {
  registerCommand(id: string, fn: (...a: any[]) => unknown) {
    state.commands.set(id, fn);
    return { dispose: () => state.commands.delete(id) };
  },
  async executeCommand(id: string, ...args: unknown[]): Promise<unknown> {
    state.executed.push({ command: id, args });
    if (id === "setContext") {
      state.contexts.set(args[0] as string, args[1]);
      return undefined;
    }
    const fn = state.commands.get(id);
    return fn ? await fn(...args) : undefined;
  },
};

export type Disposable = { dispose(): void };

// --- what is on the screen, in the shape of test/baseline ---

export interface ScreenTab {
  uri: string;
  column: number;
  text: string;
  options: { lineNumbers?: number };
  reveal: number | null;
  decorations: Array<{ opts: Record<string, unknown>; ranges: Decorated[] }>;
}

export interface Screen {
  tabs: ScreenTab[];
  tree: Array<{ label: string; description: string; color: string; kind: string }>;
  status: Array<{ text: string; color?: string; bg?: string; visible: boolean }>;
  viewDescription: string | undefined;
}

export function screen(): Screen {
  const tabs: ScreenTab[] = [...state.groups.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([column, ed]) => ({
      uri: ed.document.uri.toString(),
      column,
      text: ed.document.getText(),
      options: { ...ed.options },
      reveal: ed.reveal,
      decorations: state.paintOrder
        .filter((t) => (ed.decorations.get(t)?.length ?? 0) > 0)
        .map((t) => ({ opts: JSON.parse(JSON.stringify(t.opts)), ranges: ed.decorations.get(t)! })),
    }));
  const t = state.trees.get("srwr.ops");
  const tree = t
    ? (t.provider.getChildren() as any[]).map((el) => {
        const item = t.provider.getTreeItem(el) as TreeItem;
        return { label: item.label, description: String(item.description ?? ""), color: item.iconPath?.color?.id ?? "", kind: el.kind };
      })
    : [];
  const status = [...state.statusItems]
    .filter((s) => s.visible)
    .sort((a, b) => b.priority - a.priority)
    .map((s) => ({
      text: s.text,
      ...(s.color ? { color: s.color } : {}),
      ...(s.backgroundColor ? { bg: s.backgroundColor.id } : {}),
      visible: true,
    }));
  return { tabs, tree, status, viewDescription: t?.view.description };
}
