// The why rows that are put into the virtual document above a range.

export const BANNER_PREFIX = "◆ ";
export const BANNER_INDENT = "  ";

// Puts rows before line `at` (1-based) of text. An `at` outside the text is moved to the start or the end.
export function insertBanner(text: string, at: number, rows: string[]): string {
  const lines = text === "" ? [] : (text.endsWith("\n") ? text.slice(0, -1) : text).split("\n");
  const trailingNL = text === "" || text.endsWith("\n");
  const i = Math.min(Math.max(at - 1, 0), lines.length);
  lines.splice(i, 0, ...rows);
  return lines.join("\n") + (trailingNL ? "\n" : "");
}

// Puts `rows` above each of the lines `starts` (1-based, in the text as it is, top to bottom). With the rows in, block i starts
// `starts[i] + i * rows.length` lines down; bandRows gives those first rows.
export function insertBands(text: string, starts: number[], rows: string[]): string {
  let out = text;
  for (let i = starts.length - 1; i >= 0; i--) {
    out = insertBanner(out, starts[i], rows);
  }
  return out;
}

// The first row of each band after insertBands (1-based).
export function bandRows(starts: number[], rows: number): number[] {
  return starts.map((s, i) => s + i * rows);
}

// Moves line `n` (of the text before the bands went in) down by the bands above it. The bands are at `starts`, `rows` each.
export function belowBands(starts: number[], rows: number, n: number): number {
  return n + rows * starts.filter((s) => s <= n).length;
}

// Display width of one character: 2 for wide ones, 1 for the others.
function cells(ch: string): number {
  const c = ch.codePointAt(0) ?? 0;
  const wide =
    c >= 0x1100 &&
    (c <= 0x115f || (c >= 0x2e80 && c <= 0xa4cf) || (c >= 0xac00 && c <= 0xd7a3) || (c >= 0xf900 && c <= 0xfaff) || (c >= 0xfe30 && c <= 0xfe6f) || (c >= 0xff00 && c <= 0xff60) || (c >= 0xffe0 && c <= 0xffe6));
  return wide ? 2 : 1;
}

// Cuts a why into rows of at most `width` cells. The first row starts with the mark, the others are indented to match.
// The whole why is shown.
export function wrapWhy(text: string, width: number): string[] {
  const room = Math.max(width - 2, 8);
  const rows: string[] = [];
  for (const para of text.split("\n")) {
    let cur = "";
    let w = 0;
    const flush = (): void => {
      rows.push((rows.length === 0 ? BANNER_PREFIX : BANNER_INDENT) + cur);
      cur = "";
      w = 0;
    };
    for (const ch of para) {
      const n = cells(ch);
      if (w + n > room && cur !== "") {
        flush();
      }
      cur += ch;
      w += n;
    }
    flush();
  }
  return rows;
}
