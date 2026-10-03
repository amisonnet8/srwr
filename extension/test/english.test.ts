// English is what the screen says unless VSCode itself is in Japanese (view.test.ts and live.test.ts look at the Japanese texts).
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { ServerError } from "../src/server";
import { setJapanese } from "../src/lang";
import { formatRange } from "../src/timeline";
import { localStamp } from "../src/times";
import { FakeServer } from "./fakeserver";
import { state } from "./fakevscode";
import { start } from "./harness";
import { goldenFrames, loadGolden } from "./helpers";

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
  delete process.env.SRWR_TEST_LANG;
  process.env.TZ = "Asia/Tokyo";
});

async function openTape(server: FakeServer, id: string) {
  delete process.env.SRWR_TEST_LANG; // reset() takes the language of VSCode from it; none is English
  app = start(server);
  state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === id);
  await app.run("srwr.openTape");
  return app;
}

test("the language follows VSCode: Japanese only when it is Japanese", async () => {
  for (const [language, want] of [["en", "Back"], ["en-US", "Back"], ["fr", "Back"], ["ja", "戻る"], ["ja-JP", "戻る"]] as const) {
    app = start(serverWithTapes());
    state.language = language; // what vscode.env.language says; the extension reads it when it starts
    app.dispose();
    app = start(serverWithTapes());
    app.dispose();
    process.env.SRWR_TEST_LANG = language;
    app = start(serverWithTapes());
    state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === WHY);
    await app.run("srwr.openTape");
    assert.ok(app.screen().status[0].text.includes(want), `${language}: ${app.screen().status[0].text}`);
    app.dispose();
    app = undefined;
  }
});

test("stepping, the picker and the list in English", async () => {
  const a = await openTape(serverWithTapes(), EXT);
  assert.deepEqual(
    a.screen().status.map((s) => s.text),
    ["$(chevron-left) Back", "Forward $(chevron-right)", "1/11", "$(close)"],
  );
  assert.equal(state.quickPick?.placeHolder, "Pick a tape to replay");
  assert.deepEqual(state.quickPick?.items[0], { label: "2026-09-30 00:54:08", description: "7 operations · text.go, parse.go", detail: `${WHY}.tape.jsonl` });
  const tree = a.screen().tree;
  assert.equal(tree[5].label, "6  external  stats.go:1-126");
  assert.equal(tree[5].description, "File changed outside srwr");
  assert.equal(tree[9].label, "10  final  entry.go:1-100");
});

test("the headings of a diff frame in English", async () => {
  const s = serverWithTapes();
  const frames = s.frames.get(NOWHY)!;
  const a = await openTape(s, EXT);
  await a.run("srwr.goto", 5);
  assert.equal(a.screen().tabs[0].uri, `srwr-replay:/${EXT}/diff5/Before ⚠ Changed outside srwr: stats.go?diff=5&side=before`);
  assert.equal(a.screen().tabs[1].uri, `srwr-replay:/${EXT}/diff5/After stats.go?diff=5&side=after`);
  await a.run("srwr.goto", 9);
  assert.ok(a.screen().tabs[0].uri.includes("Before ⚠ Changed after recording (diff from current file): entry.go"));
  assert.ok(frames.length > 0);
});

test("the live bar in English", async () => {
  const frames = goldenFrames(loadGolden("ui-check-why-basic"));
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = frames.slice(0, 1);
  delete process.env.SRWR_TEST_LANG;
  app = start(s);
  await app.run("srwr.liveStart");
  const live = () => app!.screen().status.find((x) => x.text.includes("LIVE"));
  assert.equal(app.screen().viewDescription, "Live view");
  assert.equal(live()!.text, "● LIVE");
  s.emit("t1", frames[1]);
  s.emit("t1", frames[2]);
  await app.api.settled();
  await app.run("srwr.stepBack");
  s.emit("t1", frames[3]);
  await app.api.settled();
  assert.equal(live()!.text, "Back to LIVE (2 new)");
});

test("messages in English", async () => {
  delete process.env.SRWR_TEST_LANG;
  app = start(serverWithTapes());
  state.workspaceFolders = undefined;
  await app.run("srwr.openTape");
  assert.deepEqual(
    state.messages.map((m) => [m.level, m.text]),
    [["warning", "srwr: Open a folder first"]],
  );
  app.dispose();

  const s = serverWithTapes();
  s.tapes = [];
  app = start(s);
  await app.run("srwr.openTape");
  assert.equal(state.messages[0].text, "srwr: No tape with recorded operations yet (.srwr/tapes/)");
  app.dispose();

  const missing = serverWithTapes();
  missing.failList = new ServerError("binary_not_found", "Cannot start srwr (srwr): ENOENT");
  app = start(missing);
  state.pickMessageButton = (_t, b) => b[0];
  await app.run("srwr.openTape");
  const m = state.messages[0];
  assert.ok(m.text.includes("Install srwr (go install github.com/amisonnet8/srwr/cmd/srwr@latest) or set its location in the setting"), m.text);
  assert.deepEqual(m.buttons, ["Open Settings"]);
  assert.deepEqual(state.executed.find((e) => e.command === "workbench.action.openSettings")?.args, ["srwr.path"]);
  app.dispose();

  const gone = serverWithTapes();
  gone.failOpen = new ServerError("tape_not_found", "no such tape: x");
  app = start(gone);
  await app.run("srwr.openTape");
  assert.deepEqual(state.messages, [{ level: "error", text: "srwr: Tape not found", buttons: [] }], "the server's English is for developers; the screen says it in its own language");
});

test("formatRange in English", () => {
  setJapanese(false);
  assert.equal(formatRange({ start: 12, end: 11 }), "before 12");
  assert.equal(formatRange({ start: 39, end: 41 }), "39-41");
});

// The tape holds UTC (or an offset, in an older tape); the picker shows the time in the zone of the machine.
test("times are shown in the time zone of the machine", () => {
  for (const [tz, input, want] of [
    ["Asia/Tokyo", "2026-10-03T08:12:10.000Z", "2026-10-03 17:12:10"],
    ["UTC", "2026-10-03T08:12:10.000Z", "2026-10-03 08:12:10"],
    ["America/Los_Angeles", "2026-10-03T08:12:10.000Z", "2026-10-03 01:12:10"],
    ["UTC", "2026-10-03T17:12:10.000+09:00", "2026-10-03 08:12:10"],
    ["Asia/Tokyo", "2026-10-03T17:12:10.000+09:00", "2026-10-03 17:12:10"],
  ] as const) {
    process.env.TZ = tz;
    assert.equal(localStamp(input), want, `${tz} ${input}`);
  }
  assert.equal(localStamp("not a time"), undefined);
  assert.equal(localStamp(""), undefined);
});

test("the picker shows the time in the time zone of the machine", async () => {
  process.env.TZ = "UTC";
  const s = serverWithTapes();
  s.tapes[0] = { ...s.tapes[0], startedAt: "2026-09-30T00:54:08.000Z" };
  const a = await openTape(s, WHY);
  assert.equal(state.quickPick?.items[0].label, "2026-09-30 00:54:08");
  a.dispose();
  app = undefined;
  process.env.TZ = "Asia/Tokyo";
  app = start(s);
  state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === WHY);
  await app.run("srwr.openTape");
  assert.equal(state.quickPick?.items[0].label, "2026-09-30 09:54:08");
});
