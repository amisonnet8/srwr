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
