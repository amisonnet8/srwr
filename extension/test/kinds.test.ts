// Which kinds of frames are shown (the funnel button), and the frame of a failure.
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { failureText } from "../src/replay";
import { hiddenMessage } from "../src/sidebar";
import { Frame, Hidden, nearestBySeq, ShownKind, Timeline } from "../src/timeline";
import { FakeServer } from "./fakeserver";
import { state } from "./fakevscode";
import { until } from "./helpers";
import { start } from "./harness";

process.env.SRWR_TEST_LANG = "ja";

const TAPE = "20261004-1530-kinds";

// select a.go, a failure about an absolute path, replace a.go, a failure about a stale token, an external change of b.go.
function allFrames(): Frame[] {
  const text = "one\ntwo\nthree\n";
  const f = (i: number, seq: number, kind: Frame["kind"], extra: Partial<Frame> = {}): Frame => ({
    index: i, seq, kind, file: "a.go", range: { start: 2, end: 2 }, why: "reason", before: text, after: text, ...extra,
  });
  return [
    f(0, 2, "look"),
    f(1, 3, "failure", { file: "", range: { start: 9, end: 9 }, before: "", after: "", tool: "look", code: "invalid_range", message: "The path is absolute. Give a path relative to the workspace" }),
    f(2, 4, "edit", { after: "one\nTWO\nthree\n" }),
    f(3, 5, "failure", { range: { start: 0, end: -1 }, before: "", after: "", tool: "edit", code: "selection_stale", message: "stale", why: null }),
    f(4, 6, "external", { file: "b.go", range: { start: 1, end: 1 }, why: null, before: "x\n", after: "y\n" }),
  ];
}

// What the server does with the kinds: sends those frames, numbers them again, and counts the rest. final follows external.
function filterLikeTheServer(frames: Frame[], kinds: ShownKind[]): { frames: Frame[]; hidden: Hidden } {
  const hidden: Hidden = {};
  const out: Frame[] = [];
  for (const f of frames) {
    const k = (f.kind === "final" ? "external" : f.kind) as ShownKind;
    if (kinds.includes(k)) {
      out.push({ ...f, index: out.length });
    } else {
      hidden[k] = (hidden[k] ?? 0) + 1;
    }
  }
  return { frames: out, hidden };
}

function server(): FakeServer {
  const s = new FakeServer();
  s.frames.set(TAPE, allFrames());
  s.tapes.push({ tapeId: TAPE, startedAt: "2026-10-04T15:30:00+09:00", ops: 3, files: ["a.go", "b.go"] });
  s.filter = filterLikeTheServer;
  return s;
}

let app: ReturnType<typeof start> | undefined;
afterEach(() => {
  app?.dispose();
  app = undefined;
});

async function open(s: FakeServer) {
  app = start(s);
  state.pickQuickPick = (items) => items.find((i) => i.tape?.tapeId === TAPE);
  await app.run("srwr.openTape");
  return app;
}

const treeLabels = (): string[] => (state.trees.get("srwr.ops")!.provider.getChildren() as Frame[]).map((el) => String(state.trees.get("srwr.ops")!.provider.getTreeItem(el).label));
const message = (): string | undefined => state.trees.get("srwr.ops")!.view.message;

test("a tape is opened with the default kinds: failure is left out, and the list says how many", async () => {
  const s = server();
  s.hiddenOf.set(TAPE, {});
  await open(s);
  assert.deepEqual(s.openKinds[0], ["look", "edit", "external"]);
  assert.equal(treeLabels().length, 3);
  assert.equal(message(), "隠している：failure (2)");
});

test("hiddenMessage", () => {
  assert.equal(hiddenMessage({}, 3), "");
  assert.equal(hiddenMessage({ failure: 2, look: 1 }, 3), "隠している：failure (2)、look (1)");
  assert.equal(hiddenMessage({ failure: 2 }, 0), "表示するコマがありません · 隠している：failure (2)");
  assert.equal(hiddenMessage({}, 0), "表示するコマがありません");
});

test("the funnel button: the chosen kinds open the tape again, at the frame nearest to the one on the screen", async () => {
  const s = server();
  const a = await open(s);
  await a.run("srwr.goto", 1); // edit (seq 4) among look, edit, external
  state.pickQuickPick = (items) => items.filter((i) => i.shown === "edit" || i.shown === "failure");
  await a.run("srwr.chooseKinds");
  assert.deepEqual(s.openKinds.at(-1), ["edit", "failure"]);
  assert.equal(state.quickPick?.placeHolder, "表示するコマ");
  assert.deepEqual(state.quickPick?.items.map((i) => i.label), ["look", "edit", "external", "failure"]);
  // The frames are now failure, replace, failure; the replace (seq 4) is still on the screen.
  assert.equal(treeLabels().length, 3);
  assert.equal(a.api && a.screen().status.find((x) => /\d+\/\d+/.test(x.text))?.text, "2/3");
  assert.equal(message(), "隠している：look (1)、external (1)");
  // The choice is kept for the next tape that is opened.
  state.pickQuickPick = (items) => items.find((i) => i.tape?.tapeId === TAPE);
  await a.run("srwr.closeTape");
  await a.run("srwr.openTape");
  assert.deepEqual(s.openKinds.at(-1), ["edit", "failure"]);
});

test("the funnel button does nothing when no tape is open, and cancelling changes nothing", async () => {
  const s = server();
  app = start(s);
  await app.run("srwr.chooseKinds");
  assert.equal(s.openKinds.length, 0);
  const a = await open(s);
  state.pickQuickPick = () => undefined;
  await a.run("srwr.chooseKinds");
  assert.equal(s.openKinds.length, 1, "no new open");
});

test("a failure frame: a document that explains it, the first row in red, no file", async () => {
  const s = server();
  const a = await open(s);
  state.pickQuickPick = (items) => items;
  await a.run("srwr.chooseKinds"); // all four
  await a.run("srwr.goto", 1);
  const scr = a.screen();
  assert.equal(scr.tabs.length, 1);
  const tab = scr.tabs[0];
  assert.ok(tab.uri.includes("failure=1") && tab.uri.includes("失敗: look (invalid_range)"), tab.uri);
  const rows = tab.text.split("\n");
  assert.equal(rows[0], "✖ look が失敗しました (invalid_range)");
  assert.equal(rows[1], "The path is absolute. Give a path relative to the workspace");
  assert.ok(rows.some((r) => r === "why    reason"));
  assert.ok(rows.some((r) => r === "range  9 行"));
  assert.ok(rows.some((r) => r === "file   (not shown)"));
  const red = tab.decorations.find((d) => d.opts.backgroundColor === "#d50000");
  assert.ok(red, "the red row");
  assert.deepEqual(red.ranges, [{ line: 0 }]);
  assert.equal(red.opts.color, "#ffffff");
  assert.equal(red.opts.fontWeight, "bold");
  // The list: a red dot, the word 失敗, the error code in the place of file:range.
  const tree = scr.tree;
  assert.deepEqual(tree[1], { label: "2  失敗  invalid_range", description: "reason", color: "srwr.failureForeground", kind: "failure" });
  // A replace failure has no range.
  await a.run("srwr.goto", 3);
  assert.ok(a.screen().tabs[0].text.includes("range  -"));
});

test("failureText in English", () => {
  const f = allFrames()[1];
  assert.deepEqual(failureText(f).slice(0, 2), ["✖ look が失敗しました (invalid_range)", f.message]);
});

test("stepping skips nothing but the kinds that are off: the numbers are those of what is shown", async () => {
  const a = await open(server());
  assert.deepEqual(treeLabels().map((l) => l.split("  ")[0]), ["1", "2", "3"]);
  await a.run("srwr.stepForward");
  await a.run("srwr.stepForward");
  assert.equal(a.screen().status.find((x) => /\d+\/\d+/.test(x.text))?.text, "3/3");
});

test("Timeline.contentAt does not take a failure for a frame that touched the file", () => {
  const tl = new Timeline(allFrames());
  assert.equal(tl.contentAt("a.go", 3), "one\nTWO\nthree\n", "the failure at 3 names a.go but holds no text");
  assert.equal(tl.contentAt("a.go", 1), "one\ntwo\nthree\n");
});

test("nearestBySeq", () => {
  const fs = allFrames();
  assert.equal(nearestBySeq(fs, 4), 2);
  assert.equal(nearestBySeq(fs, 100), 4);
  assert.equal(nearestBySeq(fs, 1), 0);
  assert.equal(nearestBySeq([], 4), -1);
  // A tie goes to the earlier frame.
  assert.equal(nearestBySeq([{ ...fs[0], index: 0, seq: 2 }, { ...fs[0], index: 1, seq: 4 }], 3), 0);
});

test("live: the kinds are asked for, hidden counts follow, and the funnel button starts the live view again", async () => {
  const s = server();
  s.liveTape = TAPE;
  s.liveFrames = allFrames();
  s.liveHidden = { failure: 2 };
  app = start(s);
  await app.run("srwr.liveStart");
  await until("the live view", () => s.watching);
  assert.deepEqual(s.openKinds[0], ["look", "edit", "external"]);
  assert.equal(message(), "隠している：failure (2)");
  s.emitHidden(TAPE, { failure: 3 });
  await until("the new count", () => message() === "隠している：failure (3)");
  state.pickQuickPick = (items) => items;
  await app.run("srwr.chooseKinds");
  await until("restarted", () => s.calls.filter((c) => c === "liveStart").length === 2);
  assert.deepEqual(s.openKinds.at(-1), ["look", "edit", "external", "failure"]);
  assert.ok(s.calls.includes("liveStop"));
  assert.equal(treeLabels().length, 5);
});
