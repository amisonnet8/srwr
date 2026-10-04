// What the editor shows for a frame: the why row, the range and the line numbers. Replay and live both use this.
// select is blue, and what changes a file is orange. The why row is a dark color with white bold text, the range a light one.
import * as vscode from "vscode";
import { LineRange, Tone } from "./timeline";

// The colors live here and nowhere else. The why row looks the same in both themes; only the range follows the theme.
const WHY_BG: Record<Tone, string> = { select: "#0b61a4", replace: "#b45f06", failure: "#d50000" };
const RANGE_BG: Record<Tone, { dark: string; light: string }> = {
  select: { dark: "#1d3a5c", light: "#cfe3fb" },
  replace: { dark: "#583c27", light: "#fde3c8" },
  failure: { dark: "#4a2326", light: "#f9d6d6" }, // not painted today: a failure frame has only its why row
};

export interface ShowSpec {
  range: LineRange; // in the document, after the why rows were put in
  tone: Tone;
  banner?: { line: number; rows: number }; // the why rows (1-based first line)
}

export class Presenter implements vscode.Disposable {
  private readonly range: Record<Tone, vscode.TextEditorDecorationType>;
  private readonly why: Record<Tone, vscode.TextEditorDecorationType>;
  private readonly numbers = vscode.window.createTextEditorDecorationType({ before: { color: new vscode.ThemeColor("editorLineNumber.foreground") } });
  private readonly numbered = new Set<vscode.TextEditor>();
  private readonly painted = new Set<vscode.TextEditor>();

  constructor() {
    const make = <T>(f: (t: Tone) => T): Record<Tone, T> => ({ select: f("select"), replace: f("replace"), failure: f("failure") });
    this.range = make((t) =>
      vscode.window.createTextEditorDecorationType({
        isWholeLine: true,
        backgroundColor: RANGE_BG[t].dark,
        light: { backgroundColor: RANGE_BG[t].light },
      }),
    );
    this.why = make((t) => vscode.window.createTextEditorDecorationType({ isWholeLine: true, backgroundColor: WHY_BG[t], color: "#ffffff", fontWeight: "bold" }));
  }

  // Paints the range and the why rows, and clears what was shown before. Returns the range to reveal.
  show(editor: vscode.TextEditor, spec: ShowSpec): vscode.Range {
    this.clear();
    this.painted.add(editor);
    this.nativeLineNumbers(editor);
    const { range, empty } = lineRange(editor.document, spec.range);
    if (!empty) {
      editor.setDecorations(this.range[spec.tone], [range]);
    }
    if (spec.banner) {
      const rows = Array.from({ length: spec.banner.rows }, (_, k) => lineAt(editor, spec.banner!.line + k));
      editor.setDecorations(this.why[spec.tone], rows);
    }
    return range;
  }

  // Says explicitly that the standard line numbers are on. A setting that turned them off is carried over to the next
  // editor opened in the same place, so every frame without why rows (diff frames too) states it again.
  private nativeLineNumbers(editor: vscode.TextEditor): void {
    try {
      editor.options = { lineNumbers: vscode.TextEditorLineNumbersStyle.On };
    } catch {
      // the editor was already closed
    }
  }

  // The why rows make the standard numbers wrong, so they are turned off and the file's own numbers are drawn at the
  // left of each row (a why row gets blank space). `at` is the first why row (1-based), `rows` how many there are.
  showLineNumbers(editor: vscode.TextEditor, at: number, rows: number): void {
    const doc = editor.document;
    const last = rows > 0 ? doc.lineCount - rows : doc.lineCount;
    const width = String(Math.max(last, 1)).length;
    const gap = " ".repeat(2);
    const decos: vscode.DecorationOptions[] = [];
    for (let i = 1; i <= doc.lineCount; i++) {
      const isWhy = i >= at && i < at + rows;
      const n = i < at ? i : i - rows;
      const text = isWhy ? " ".repeat(width) : String(n).padStart(width, " ");
      decos.push({ range: lineAt(editor, i), renderOptions: { before: { contentText: text + gap } } });
    }
    editor.options = { lineNumbers: vscode.TextEditorLineNumbersStyle.Off };
    editor.setDecorations(this.numbers, decos);
    this.numbered.add(editor);
  }

  // A diff frame: paints the changed lines (1-based). Does not clear; it is called for the left and the right editor.
  // A sub frame has a band of `band` rows above the text (blue on the left, orange on the right): the band is painted in the
  // tone, and the file's own line numbers are drawn, since the band makes the standard ones wrong.
  paintLines(editor: vscode.TextEditor, tone: Tone, lines: number[], band = 0): void {
    this.painted.add(editor);
    this.nativeLineNumbers(editor);
    if (band > 0) {
      editor.setDecorations(
        this.why[tone],
        Array.from({ length: band }, (_, k) => lineAt(editor, 1 + k)),
      );
      this.showLineNumbers(editor, 1, band);
    }
    editor.setDecorations(
      this.range[tone],
      lines.map((n) => lineAt(editor, n)),
    );
  }

  clear(): void {
    for (const editor of this.numbered) {
      try {
        editor.setDecorations(this.numbers, []);
        editor.options = { lineNumbers: vscode.TextEditorLineNumbersStyle.On };
      } catch {
        // the editor was already closed
      }
    }
    this.numbered.clear();
    const editors = [...this.painted];
    this.painted.clear();
    for (const editor of editors) {
      try {
        for (const d of [...Object.values(this.range), ...Object.values(this.why)]) {
          editor.setDecorations(d, []);
        }
      } catch {
        // the editor was already closed
      }
    }
  }

  dispose(): void {
    this.clear();
    this.numbers.dispose();
    for (const d of [...Object.values(this.range), ...Object.values(this.why)]) {
      d.dispose();
    }
  }
}

// A 1-based line range as a Range inside the document. An empty range gives the one line at its position.
export function lineRange(doc: vscode.TextDocument, r: LineRange): { range: vscode.Range; empty: boolean } {
  const last = Math.max(doc.lineCount - 1, 0);
  const clamp = (n: number): number => Math.min(Math.max(n, 0), last);
  const empty = r.end < r.start;
  const s = clamp(r.start - 1);
  const e = empty ? s : clamp(r.end - 1);
  return { range: new vscode.Range(doc.lineAt(s).range.start, doc.lineAt(e).range.end), empty };
}

function lineAt(editor: vscode.TextEditor, line: number): vscode.Range {
  const doc = editor.document;
  const n = Math.min(Math.max(line - 1, 0), Math.max(doc.lineCount - 1, 0));
  return doc.lineAt(n).range;
}
