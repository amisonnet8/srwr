// The replay screen, on the fake vscode and a fake server that answers from the golden data.
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { ServerError } from "../src/server";
import { FakeServer } from "./fakeserver";
import { state } from "./fakevscode";
import { goldenFrames, loadGolden } from "./helpers";
import { start } from "./harness";

// These tests look at the Japanese texts (english.test.ts looks at the English ones).
process.env.SRWR_TEST_LANG = "ja";

const WHY = "20260930-0054-why-basic";
const EXT = "20260930-0949-external";
const NOWHY = "20260930-0053-no-why";

function serverWithTapes(): FakeServer {
  const s = new FakeServer();
  for (const [id, name] of [
    [WHY, "ui-check-why-basic"],
    [EXT, "ui-check-external"],
    [NOWHY, "ui-check-no-why"],
  ]) {
    const frames = goldenFrames(loadGolden(name));
    s.frames.set(id, frames);
    s.tapes.push({ tapeId: id, startedAt: "2026-09-30T00:54:08+09:00", ops: frames.length, files: [...new Set(frames.map((f) => f.file))] });
  }
  return s;
}

let app: ReturnType<typeof start> | undefined;
afterEach(() => {
  app?.dispose();
  app = undefined;
});

async function openTape(server: FakeServer, id: string) {
  app = start(server);
  state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === id);
  await app.run("srwr.openTape");
  return app;
}

const colorsOf = (tab: { decorations: Array<{ opts: Record<string, unknown> }> }): unknown[] => tab.decorations.map((d) => d.opts.backgroundColor ?? "numbers");

test("a look frame: why row in blue above the range in light blue, file's own line numbers", async () => {
  const a = await openTape(serverWithTapes(), WHY);
  const s = a.screen();
  assert.equal(s.tabs.length, 1);
  const tab = s.tabs[0];
  assert.equal(tab.uri, `srwr-replay:/${WHY}/text.go`);
  assert.equal(tab.options.lineNumbers, 0, "the standard numbers are off while a why row is in");
  const lines = tab.text.split("\n");
  // golden frame 0: text.go lines 37-37, one why row
  assert.ok(lines[36].startsWith("◆ Truncate が幅ちょうど"));
  assert.ok(lines[37].startsWith("\tif DisplayWidth(s) < w"));
  // Listed in the order they were first painted: the range, the why row, then the numbers.
  assert.deepEqual(colorsOf(tab), ["#1d3a5c", "#0b61a4", "numbers"]);
  const [range, why, numbers] = tab.decorations;
  assert.deepEqual(range.ranges, [{ line: 37 }], "the range moved down by the why rows");
  assert.deepEqual(why.ranges, [{ line: 36 }]);
  assert.equal(why.opts.color, "#ffffff");
  assert.equal(why.opts.fontWeight, "bold");
  assert.deepEqual((range.opts.light as { backgroundColor: string }).backgroundColor, "#cfe3fb");
  // The why row has blank space, the row after it has the file's number 37.
  const nbsp = "\u00a0";
  // The width is the digits of the file's last line number (3 here), then two spaces.
  assert.equal(numbers.ranges[36].before, nbsp.repeat(3 + 2), "a why row is blank");
  assert.equal(numbers.ranges[37].before, nbsp + "37" + nbsp.repeat(2));
  assert.equal(numbers.ranges[0].before, nbsp.repeat(2) + "1" + nbsp.repeat(2));
  assert.equal(numbers.ranges.length, lines.length, "one number per row, the trailing empty row too");
  assert.equal(tab.reveal, 38);
});

test("an edit frame is orange", async () => {
  const a = await openTape(serverWithTapes(), WHY);
  await a.run("srwr.goto", 4);
  const tab = a.screen().tabs[0];
  assert.deepEqual(colorsOf(tab), ["numbers", "#583c27", "#b45f06"]);
  assert.ok(tab.text.includes("DisplayWidth(s) <= w"), "the content after the replace is shown");
});

test("a frame without why: no why row, standard numbers stated explicitly, range only", async () => {
  const a = await openTape(serverWithTapes(), NOWHY);
  const tab = a.screen().tabs[0];
  assert.equal(tab.options.lineNumbers, 1);
  assert.deepEqual(colorsOf(tab), ["#1d3a5c"]);
  assert.ok(!tab.text.includes("◆"));
});

test("an empty range paints nothing", async () => {
  const s = serverWithTapes();
  const frames = s.frames.get(NOWHY)!;
  frames[0] = { ...frames[0], range: { start: 3, end: 2 } };
  const a = await openTape(s, NOWHY);
  assert.deepEqual(a.screen().tabs[0].decorations, []);
});

test("stepping: always a back and a forward button, dimmed at the ends, position n/total", async () => {
  const a = await openTape(serverWithTapes(), WHY);
  const text = () => a.screen().status.map((s) => s.text);
  const dimmed = () => a.screen().status.filter((s) => s.color).map((s) => s.text);
  assert.deepEqual(text(), ["$(chevron-left) 戻る", "進む $(chevron-right)", "1/7", "$(close)"]);
  assert.deepEqual(dimmed(), ["$(chevron-left) 戻る"]);
  await a.run("srwr.stepBack");
  assert.equal(text()[2], "1/7", "stays at the first frame");
  for (let i = 0; i < 10; i++) {
    await a.run("srwr.stepForward");
  }
  assert.equal(text()[2], "7/7", "stays at the last frame");
  assert.deepEqual(dimmed(), ["進む $(chevron-right)"]);
  await a.run("srwr.stepBack");
  assert.equal(text()[2], "6/7");
  assert.deepEqual(dimmed(), []);
  assert.equal(a.screen().status.find((s) => s.text.endsWith("戻る"))!.color, undefined);
});

test("quick pick: placeholder and items", async () => {
  const a = await openTape(serverWithTapes(), WHY);
  assert.equal(a.screen().viewDescription, WHY);
  assert.equal(state.quickPick?.placeHolder, "再生するテープを選ぶ");
  assert.deepEqual(state.quickPick?.items[0], { label: "2026-09-30 00:54:08", description: "7操作 · text.go, parse.go", detail: `${WHY}.tape.jsonl` });
});

test("the list: numbered from 1, kinds, dots and a click goes to the frame", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  const tree = a.screen().tree;
  assert.equal(tree.length, 11);
  assert.deepEqual(tree[0], { label: "1  look  entry.go:29", description: "秒からミリ秒への倍率が間違っていて 1.5s が 150ms になっているため", color: "charts.blue", kind: "look" });
  assert.deepEqual([tree[3].color, tree[5].color, tree[9].color], ["charts.orange", "charts.purple", "charts.purple"]);
  assert.equal(tree[5].label, "6  外部変更  stats.go:1-126");
  assert.equal(tree[5].description, "srwr の外でファイルが変わった");
  assert.equal(tree[9].label, "10  録画後  entry.go:1-100");
  assert.equal(tree[9].description, "");
  const provider = state.trees.get("srwr.ops")!.provider;
  const item = provider.getTreeItem(provider.getChildren()[2]);
  assert.deepEqual(item.command, { command: "srwr.goto", title: "この操作へ移動", arguments: [2] });
  assert.equal(state.contexts.get("srwr.hasTape"), true);
});

test("a diff frame: two editors, changed lines only, headings, standard numbers", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  await a.run("srwr.goto", 5);
  const s = a.screen();
  assert.equal(s.tabs.length, 2);
  assert.equal(s.tabs[0].uri, `srwr-replay:/${EXT}/diff5/前 ⚠ srwrの外で変更：stats.go?diff=5&side=before`);
  assert.equal(s.tabs[1].uri, `srwr-replay:/${EXT}/diff5/後 stats.go?diff=5&side=after`);
  assert.deepEqual(s.tabs.map((t) => t.column), [1, 2]);
  assert.deepEqual(s.tabs.map((t) => t.options.lineNumbers), [1, 1]);
  assert.deepEqual(colorsOf(s.tabs[0]), ["#1d3a5c"], "left is blue");
  assert.deepEqual(colorsOf(s.tabs[1]), ["#583c27"], "right is orange");
  assert.deepEqual(s.tabs[0].decorations[0].ranges, [{ line: 23 }, { line: 29 }, { line: 32 }]);
  assert.equal(s.tabs[0].reveal, 12, "the first changed line is about 30% from the top");
  assert.equal(s.tabs[1].reveal, 12);
});

test("a final frame: headings for a changed file and for a file that is gone", async () => {
  const a = await openTape(serverWithTapes(), NOWHY);
  await a.run("srwr.goto", 10);
  assert.equal(a.screen().tabs[0].uri, `srwr-replay:/${NOWHY}/diff10/前 ⚠ 録画のあとで変更（今は存在しない）：TASK.md?diff=10&side=before`);
  await a.run("srwr.goto", 11);
  assert.ok(a.screen().tabs[0].uri.includes("前 ⚠ 録画のあとで変更（今のファイルとの差分）：go.mod"));
});

test("a deleted external frame says so", async () => {
  const s = serverWithTapes();
  const frames = s.frames.get(EXT)!;
  frames[5] = { ...frames[5], deleted: true, after: "" };
  const a = await openTape(s, EXT);
  await a.run("srwr.goto", 5);
  assert.ok(a.screen().tabs[0].uri.includes("前 ⚠ srwrの外で変更（削除）：stats.go"));
});

test("going back and forth over diff frames never piles up editors", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  for (let round = 0; round < 6; round++) {
    await a.run("srwr.goto", 5);
    assert.equal(a.screen().tabs.length, 2);
    await a.run("srwr.goto", 6);
    const s = a.screen();
    assert.equal(s.tabs.length, 1, "the right-hand editor is closed again");
    assert.equal(s.tabs[0].uri, `srwr-replay:/${EXT}/stats.go`);
    assert.equal(s.tabs[0].options.lineNumbers, 0);
    await a.run("srwr.goto", 9);
    assert.equal(a.screen().tabs.length, 2);
    await a.run("srwr.goto", 4);
    assert.equal(a.screen().tabs.length, 1);
  }
  assert.ok(state.documents.size < 30, `${state.documents.size} documents`);
});

test("a normal frame after a diff frame stated the standard numbers off again, and the numbers are cleared on the next", async () => {
  const a = await openTape(serverWithTapes(), WHY);
  await a.run("srwr.goto", 3);
  await a.run("srwr.goto", 4);
  const tab = a.screen().tabs[0];
  // Only the numbers, one range and one why row are painted; nothing is left from the previous frame.
  assert.equal(tab.decorations.length, 3);
  assert.equal(tab.decorations[1].ranges.length, 1);
});

test("quick successive steps end on the last one asked for", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  const done = [a.run("srwr.goto", 5), a.run("srwr.goto", 6), a.run("srwr.goto", 1), a.run("srwr.goto", 5), a.run("srwr.goto", 2)];
  await Promise.all(done);
  const s = a.screen();
  assert.equal(s.status.find((x) => /^\d+\/\d+$/.test(x.text))!.text, "3/11");
  assert.equal(s.tabs.length, 1);
  assert.equal(s.tabs[0].uri, `srwr-replay:/${EXT}/stats.go`);
});

test("closing: the list and the bar go away, the server is told", async () => {
  const server = serverWithTapes();
  const a = await openTape(server, WHY);
  await a.run("srwr.closeTape");
  const s = a.screen();
  assert.deepEqual(s.tree, []);
  assert.deepEqual(s.status, []);
  assert.equal(state.contexts.get("srwr.hasTape"), false);
  assert.ok(server.calls.includes(`close ${WHY}`));
  await a.run("srwr.stepForward"); // nothing is open; must not fail
});

test("opening another tape replaces the first", async () => {
  const server = serverWithTapes();
  const a = await openTape(server, WHY);
  state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === EXT);
  await a.run("srwr.openTape");
  assert.ok(server.calls.includes(`close ${WHY}`));
  assert.equal(a.screen().tree.length, 11);
});

test("no folder open: a warning", async () => {
  app = start(serverWithTapes());
  state.workspaceFolders = undefined;
  await app.run("srwr.openTape");
  await app.run("srwr.liveStart");
  assert.deepEqual(
    state.messages.map((m) => [m.level, m.text]),
    [
      ["warning", "srwr: フォルダを開いてから実行してください"],
      ["warning", "srwr: フォルダを開いてから実行してください"],
    ],
  );
});

test("no tape: an information message", async () => {
  const s = serverWithTapes();
  s.tapes = [];
  app = start(s);
  await app.run("srwr.openTape");
  assert.deepEqual(state.messages, [{ level: "info", text: "srwr: 操作を記録したテープがありません（.srwr/tapes/）", buttons: [] }]);
});

test("the binary is missing: how to install it, and a button to open the setting", async () => {
  const s = serverWithTapes();
  s.failList = new ServerError("binary_not_found", "srwr を起動できません（srwr）: ENOENT");
  app = start(s);
  state.pickMessageButton = (_t, b) => b[0];
  await app.run("srwr.openTape");
  const m = state.messages[0];
  assert.equal(m.level, "error");
  assert.ok(m.text.includes("srwr を起動できません（srwr）: ENOENT"));
  assert.ok(m.text.includes("go install github.com/amisonnet8/srwr/cmd/srwr@latest"));
  assert.ok(m.text.includes("srwr.path"));
  assert.deepEqual(m.buttons, ["設定を開く"]);
  assert.deepEqual(state.executed.find((e) => e.command === "workbench.action.openSettings")?.args, ["srwr.path"]);
});

test("any other error is shown with the server's message", async () => {
  const s = serverWithTapes();
  s.failOpen = new ServerError("tape_not_found", "テープが見つからない");
  app = start(s);
  await app.run("srwr.openTape");
  assert.deepEqual(state.messages, [{ level: "error", text: "srwr: テープが見つからない", buttons: [] }]);
  assert.deepEqual(app.screen().tree, [], "nothing is open after a failure");
});

test("the server is made when first used, and again when srwr.path changes", async () => {
  const server = serverWithTapes();
  app = start(server);
  assert.equal(app.serversMade, 0);
  await app.run("srwr.openTape");
  await app.run("srwr.openTape");
  assert.equal(app.serversMade, 1);
  state.config.path = "/other/srwr";
  await app.run("srwr.openTape");
  assert.equal(app.serversMade, 2);
  assert.ok(server.calls.includes("dispose"), "the old server is stopped");
});

test("a diff frame after a frame with why rows still gets the standard line numbers (the off setting is carried over otherwise)", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  assert.equal(a.screen().tabs[0].options.lineNumbers, 0);
  await a.run("srwr.goto", 5);
  assert.deepEqual(a.screen().tabs.map((t) => t.options.lineNumbers), [1, 1]);
  await a.run("srwr.goto", 7); // why rows again
  assert.equal(a.screen().tabs[0].options.lineNumbers, 0);
  await a.run("srwr.goto", 9);
  assert.deepEqual(a.screen().tabs.map((t) => t.options.lineNumbers), [1, 1]);
});

test("quick successive steps: the frames in between are never opened", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  const before = state.opens;
  await Promise.all([a.run("srwr.goto", 1), a.run("srwr.goto", 2), a.run("srwr.goto", 3), a.run("srwr.goto", 4)]);
  assert.ok(state.opens - before <= 1, `${state.opens - before} documents were opened for 4 quick steps`);
});

test("the line numbers are as wide as the file's last line number, not the document with the why rows", async () => {
  const s = new FakeServer();
  const text = "x\n".repeat(98);
  const why = "a".repeat(150); // two rows at width 100
  s.frames.set("t", [{ index: 0, kind: "look", file: "a.txt", range: { start: 1, end: 1 }, why, before: text, after: text }]);
  s.tapes.push({ tapeId: "t", startedAt: "", ops: 1, files: ["a.txt"] });
  const a = await openTape(s, "t");
  const numbers = a.screen().tabs[0].decorations.find((d) => "before" in d.opts)!;
  const nbsp = "\u00a0";
  assert.equal(numbers.ranges[0].before, nbsp.repeat(2 + 2), "98 lines + the empty last row = 99: two digits, then two spaces");
  assert.equal(numbers.ranges[2].before, nbsp + "1" + nbsp.repeat(2));
});

// A new frame: the whole file in one editor, painted like an edit, with its why in a line above.
function serverWithNew(): FakeServer {
  const s = new FakeServer();
  const id = "20261005-1100-new";
  const after = "package a\n\nvar X = 1\n";
  const frames = [{ index: 0, kind: "new" as const, seq: 1, file: "a.go", range: { start: 1, end: 3 }, why: "パッケージを作る", before: "", after }];
  s.frames.set(id, frames);
  s.tapes.push({ tapeId: id, startedAt: "2026-10-05T11:00:00+09:00", ops: 1, files: ["a.go"] });
  return s;
}

test("a new frame: one editor with the whole file painted like an edit, listed as new in orange", async () => {
  const a = await openTape(serverWithNew(), "20261005-1100-new");
  const s = a.screen();
  assert.equal(s.tabs.length, 1);
  assert.equal(s.tabs[0].text, "◆ パッケージを作る\npackage a\n\nvar X = 1\n");
  const colors = colorsOf(s.tabs[0]);
  assert.ok(colors.includes("#b45f06") && colors.includes("#583c27"), `${colors}`);
  const row = s.tree[0];
  assert.match(String(row.label), /^1  new  a\.go:1-3$/);
  assert.equal(row.color, "charts.orange");
});
