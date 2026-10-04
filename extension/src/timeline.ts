// The frames the display server builds (docs/reference/protocol.md) and the small calculations the editor does on them.
// This file does not import vscode.

import { pick } from "./lang";

export type FrameKind = "select" | "replace" | "external" | "final" | "failure";

// The kinds a person can turn on and off (final follows external), and what the server was told is hidden: kind -> how many.
export type ShownKind = "select" | "replace" | "external" | "failure";
export const ALL_KINDS: ShownKind[] = ["select", "replace", "external", "failure"];
export const DEFAULT_KINDS: ShownKind[] = ["select", "replace", "external"];
export type Hidden = Record<string, number>;

export interface LineRange {
  start: number;
  end: number; // end < start is an empty range
}

// Only the fields the editor uses (docs/reference/protocol.md). Unknown fields are ignored.
export interface Frame {
  index: number;
  seq?: number; // the seq of the tape; used to find the nearest frame again after the kinds shown were changed
  kind: FrameKind;
  file: string;
  range: LineRange;
  why: string | null;
  before: string;
  after: string;
  deleted?: boolean;
  // failure frames only
  tool?: string;
  code?: string;
  message?: string;
}

// select is blue, everything that changes a file is orange, a failure is red.
export type Tone = "select" | "replace" | "failure";

export function isDiff(f: Frame): boolean {
  return f.kind === "external" || f.kind === "final";
}

export function toneOf(f: Frame): Tone {
  return f.kind === "select" ? "select" : f.kind === "failure" ? "failure" : "replace";
}

export function basename(file: string): string {
  const i = file.lastIndexOf("/");
  return i < 0 ? file : file.slice(i + 1);
}

// "37", "39-41", and "12の前" for an empty range.
export function formatRange(r: LineRange): string {
  if (r.end < r.start) {
    return pick(`before ${r.start}`, `${r.start}の前`);
  }
  return r.start === r.end ? `${r.start}` : `${r.start}-${r.end}`;
}

// Turns a frame of the server's JSON into a Frame. before/after are only there when withText was asked for.
export function toFrame(raw: Record<string, unknown>): Frame {
  const f = raw as unknown as Frame;
  return {
    index: f.index,
    seq: typeof f.seq === "number" ? f.seq : 0,
    kind: f.kind,
    file: f.file,
    range: f.range,
    why: typeof f.why === "string" ? f.why : null,
    before: typeof f.before === "string" ? f.before : "",
    after: typeof f.after === "string" ? f.after : "",
    ...(f.deleted ? { deleted: true } : {}),
    ...(f.kind === "failure" ? { tool: f.tool ?? "", code: f.code ?? "", message: f.message ?? "" } : {}),
  };
}

export class Timeline {
  readonly frames: Frame[];

  constructor(frames: Frame[] = []) {
    this.frames = [...frames];
  }

  append(f: Frame): void {
    this.frames.push(f);
  }

  get length(): number {
    return this.frames.length;
  }

  // The content of file after frame i. A file nobody touched yet shows what the first frame that touches it saw.
  // undefined when no frame touches it.
  contentAt(file: string, i: number): string | undefined {
    for (let j = Math.min(i, this.frames.length - 1); j >= 0; j--) {
      if (this.frames[j].file === file && this.frames[j].kind !== "failure") {
        return this.frames[j].after;
      }
    }
    for (let j = Math.max(i + 1, 0); j < this.frames.length; j++) {
      if (this.frames[j].file === file && this.frames[j].kind !== "failure") {
        return this.frames[j].before;
      }
    }
    return undefined;
  }
}

// Lines of a text the way the server counts them: "" is no lines, and a final "\n" does not start another one.
export function splitLines(text: string): string[] {
  if (text === "") {
    return [];
  }
  return (text.endsWith("\n") ? text.slice(0, -1) : text).split("\n");
}

// Compares two line lists and returns the changed lines (1-based) on each side. The common head and tail are dropped
// first and the rest is compared with a longest common subsequence. A rest that is too big counts as all changed.
export function changedLines(before: string[], after: string[]): { before: number[]; after: number[] } {
  let head = 0;
  while (head < before.length && head < after.length && before[head] === after[head]) {
    head++;
  }
  let tail = 0;
  while (tail < before.length - head && tail < after.length - head && before[before.length - 1 - tail] === after[after.length - 1 - tail]) {
    tail++;
  }
  const a = before.slice(head, before.length - tail);
  const b = after.slice(head, after.length - tail);
  const rows = (n: number, keep: Set<number>): number[] =>
    Array.from({ length: n }, (_, k) => k)
      .filter((k) => !keep.has(k))
      .map((k) => head + k + 1);
  if (a.length * b.length > MAX_LCS_CELLS) {
    return { before: rows(a.length, new Set()), after: rows(b.length, new Set()) };
  }
  // lcs[i][j] is the length of the longest common subsequence of a[i..] and b[j..]
  const lcs: Uint32Array[] = Array.from({ length: a.length + 1 }, () => new Uint32Array(b.length + 1));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const keepA = new Set<number>();
  const keepB = new Set<number>();
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      keepA.add(i++);
      keepB.add(j++);
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      i++;
    } else {
      j++;
    }
  }
  return { before: rows(a.length, keepA), after: rows(b.length, keepB) };
}

export const MAX_LCS_CELLS = 4_000_000;

// The frame to go to after the kinds that are shown were changed: the one whose seq is nearest to seq (the earlier one on a tie).
// -1 when there is no frame.
export function nearestBySeq(frames: Frame[], seq: number): number {
  let best = -1;
  for (const f of frames) {
    if (best < 0 || Math.abs((f.seq ?? 0) - seq) < Math.abs((frames[best].seq ?? 0) - seq)) {
      best = f.index;
    }
  }
  return best;
}
