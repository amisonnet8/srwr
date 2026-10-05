import assert from "node:assert/strict";
import { test } from "node:test";
import { setJapanese } from "../src/lang";
import { PROTOCOL_VERSION, resolveCommand } from "../src/server";
import { Timeline, basename, changedLines, formatRange, splitLines, toFrame, MAX_LCS_CELLS } from "../src/timeline";
import { goldenFrames, goldenKind, goldenNames, loadGolden } from "./helpers";

// The Japanese text of "before 12" (english.test.ts looks at the English one).
setJapanese(true);

test("formatRange: one line, many lines, empty range", () => {
  assert.equal(formatRange({ start: 37, end: 37 }), "37");
  assert.equal(formatRange({ start: 39, end: 41 }), "39-41");
  assert.equal(formatRange({ start: 12, end: 11 }), "12の前");
});

test("basename drops the directory", () => {
  assert.equal(basename("a/b/c.go"), "c.go");
  assert.equal(basename("c.go"), "c.go");
});

test("splitLines: '' is no lines and a final newline does not start another", () => {
  assert.deepEqual(splitLines(""), []);
  assert.deepEqual(splitLines("a\nb\n"), ["a", "b"]);
  assert.deepEqual(splitLines("a\nb"), ["a", "b"]);
  assert.deepEqual(splitLines("a\n\n"), ["a", ""]);
});

test("changedLines", async (t) => {
  const cases: Array<{ name: string; a: string[]; b: string[]; want: { before: number[]; after: number[] } }> = [
    { name: "same", a: ["x", "y"], b: ["x", "y"], want: { before: [], after: [] } },
    { name: "one line changed", a: ["a", "b", "c"], b: ["a", "B", "c"], want: { before: [2], after: [2] } },
    { name: "only added", a: ["a", "c"], b: ["a", "b", "c"], want: { before: [], after: [2] } },
    { name: "only removed", a: ["a", "b", "c"], b: ["a", "c"], want: { before: [2], after: [] } },
    { name: "moved apart (LCS, not just head and tail)", a: ["a", "x", "b", "y", "c"], b: ["a", "b", "z", "c"], want: { before: [2, 4], after: [3] } },
    { name: "from empty", a: [], b: ["a", "b"], want: { before: [], after: [1, 2] } },
    { name: "to empty", a: ["a"], b: [], want: { before: [1], after: [] } },
    { name: "repeated lines", a: ["a", "a", "a"], b: ["a", "a"], want: { before: [3], after: [] } },
  ];
  for (const c of cases) {
    await t.test(c.name, () => assert.deepEqual(changedLines(c.a, c.b), c.want));
  }
});

test("changedLines: a rest that is too big counts as all changed", () => {
  const n = Math.floor(Math.sqrt(MAX_LCS_CELLS)) + 1;
  const a = Array.from({ length: n }, (_, i) => `a${i}`);
  const b = Array.from({ length: n }, (_, i) => (i === 0 ? "first" : `a${i}`)).reverse();
  const r = changedLines(a, b);
  // Even lines that did not change are reported, because the rest was not compared.
  assert.ok(r.before.length > 1 && r.after.length > 1);
  assert.equal(r.before.length, n);
});

test("toFrame fills in what is missing and ignores the rest", () => {
  const f = toFrame({ index: 3, kind: "look", file: "a.go", range: { start: 1, end: 2 }, why: null, seq: 9, jumpLabel: "x" });
  assert.deepEqual(f, { index: 3, seq: 9, kind: "look", file: "a.go", range: { start: 1, end: 2 }, why: null, before: "", after: "" });
  assert.equal(toFrame({ index: 0, kind: "final", file: "a", range: { start: 1, end: 0 }, why: null, deleted: true }).deleted, true);
});

test("resolveCommand: setting, then SRWR_PATH, then PATH", () => {
  assert.equal(resolveCommand("/opt/srwr", { SRWR_PATH: "/dev/srwr" }), "/opt/srwr");
  assert.equal(resolveCommand("srwr", { SRWR_PATH: "/dev/srwr" }), "/dev/srwr");
  assert.equal(resolveCommand("", { SRWR_PATH: " " }), "srwr");
  assert.equal(resolveCommand("srwr", {}), "srwr");
  assert.equal(PROTOCOL_VERSION, 2);
});

// The golden data holds the content of each file at i = -1, 0, 1 ... per frame. contentAt must give the same.
for (const name of goldenNames()) {
  test(`golden ${name}: contentAt and the fields the editor uses`, () => {
    const g = loadGolden(name);
    const tl = new Timeline(goldenFrames(g));
    assert.equal(tl.length, g.frames.length);
    for (const [file, list] of Object.entries(g.contentAt)) {
      // The list starts at i = -1: before any frame.
      list.forEach((want, k) => assert.equal(tl.contentAt(file, k - 1), want ?? undefined, `${file} at ${k - 1}`));
    }
    g.frames.forEach((raw, i) => {
      const f = tl.frames[i];
      assert.equal(f.index, i);
      assert.equal(f.kind, goldenKind(String(raw.kind)));
      assert.deepEqual(f.range, raw.range);
      assert.equal(f.why, raw.why ?? null);
      assert.equal(f.before, raw.before);
      assert.equal(f.after, raw.after);
    });
  });
}
