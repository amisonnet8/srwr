// The live view, on the fake vscode and a fake server that the test pushes frames through.
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { Frame } from "../src/timeline";
import { FakeServer } from "./fakeserver";
import { state } from "./fakevscode";
import { start } from "./harness";
import { goldenFrames, loadGolden } from "./helpers";

// These tests look at the Japanese texts (english.test.ts looks at the English ones).
process.env.SRWR_TEST_LANG = "ja";

const WHY = goldenFrames(loadGolden("ui-check-why-basic"));
const EXT = goldenFrames(loadGolden("ui-check-external"));

let app: ReturnType<typeof start> | undefined;
afterEach(() => {
  app?.dispose();
  app = undefined;
});

async function live(server: FakeServer) {
  app = start(server);
  await app.run("srwr.liveStart");
  return app;
}

const position = (a: ReturnType<typeof start>): string => a.screen().status.find((s) => /^\d+\/\d+$/.test(s.text))!.text;
const liveItem = (a: ReturnType<typeof start>) => a.screen().status.find((s) => s.text.includes("LIVE"));

test("start: the frames so far are listed, nothing is shown", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  const a = await live(s);
  const sc = a.screen();
  assert.equal(sc.tabs.length, 0);
  assert.equal(sc.tree.length, 1);
  assert.equal(sc.viewDescription, "ライブ視聴");
  assert.equal(position(a), "1/1");
  assert.equal(liveItem(a)!.text, "● LIVE");
  assert.equal(liveItem(a)!.bg, undefined);
});

test("following: each new frame is shown at once, also when they come in a burst", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  const a = await live(s);
  s.emit("t1", WHY[1]);
  await a.api.settled();
  assert.equal(a.screen().tabs[0].uri, "srwr-replay:/live/text.go");
  assert.equal(position(a), "2/2");
  // frames 2..6 arrive one after the other without the screen having caught up
  for (const f of WHY.slice(2)) {
    s.emit("t1", f);
  }
  await a.api.settled();
  assert.equal(position(a), "7/7");
  assert.equal(liveItem(a)!.text, "● LIVE");
  assert.equal(a.screen().tabs[0].uri, "srwr-replay:/live/parse.go");
  assert.ok(a.screen().tabs[0].text.includes("◆ 区切りの数+1"), "the last frame (a replace of parse.go) is on the screen");
});

test("stepping back stops following: the screen stays, new frames are counted", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  const a = await live(s);
  s.emit("t1", WHY[1]);
  s.emit("t1", WHY[2]);
  await a.api.settled();
  await a.run("srwr.stepBack");
  assert.equal(position(a), "2/3");
  assert.deepEqual([liveItem(a)!.text, liveItem(a)!.bg], ["LIVE に戻る（新着 1）", "statusBarItem.warningBackground"]);
  const before = JSON.stringify(a.screen().tabs);
  s.emit("t1", WHY[3]);
  s.emit("t1", WHY[4]);
  await a.api.settled();
  assert.equal(liveItem(a)!.text, "LIVE に戻る（新着 3）");
  assert.equal(position(a), "2/5");
  assert.equal(JSON.stringify(a.screen().tabs), before, "the screen did not move");
  assert.equal(a.screen().tree.length, 5, "but the list grew");
  // back to the latest, and follow again
  await a.run("srwr.liveLatest");
  assert.equal(position(a), "5/5");
  assert.equal(liveItem(a)!.text, "● LIVE");
  s.emit("t1", WHY[5]);
  await a.api.settled();
  assert.equal(position(a), "6/6");
});

test("clicking an older frame also stops following; stepping forward to the end follows again", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 3);
  const a = await live(s);
  await a.run("srwr.goto", 0);
  s.emit("t1", WHY[3]);
  await a.api.settled();
  assert.equal(liveItem(a)!.text, "LIVE に戻る（新着 3）");
  await a.run("srwr.stepForward");
  await a.run("srwr.stepForward");
  await a.run("srwr.stepForward");
  assert.equal(liveItem(a)!.text, "● LIVE");
  s.emit("t1", WHY[4]);
  await a.api.settled();
  assert.equal(position(a), "5/5", "caught up, so it follows");
});

test("a frame that arrives before the answer of live/start is played after it", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 2);
  let release!: () => void;
  s.liveGate = new Promise<void>((r) => (release = r));
  s.beforeAnswer = () => s.emit("t1", WHY[2]);
  app = start(s);
  const starting = app.run("srwr.liveStart");
  release();
  await starting;
  await app.api.settled();
  assert.equal(position(app), "3/3");
  assert.equal(app.screen().tree.length, 3);
  assert.equal(app.screen().tabs[0].uri, "srwr-replay:/live/parse.go", "the early frame is the one on the screen");
});

test("a frame of another tape starts the list over, from its first frame", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 3);
  const a = await live(s);
  s.emit("t2", EXT[0]);
  await a.api.settled();
  assert.equal(a.screen().tree.length, 1);
  assert.equal(position(a), "1/1");
  assert.equal(a.screen().tabs[0].uri, "srwr-replay:/live/entry.go");
  s.emit("t2", EXT[1]);
  await a.api.settled();
  assert.equal(position(a), "2/2");
});

test("an external frame is shown as two editors, and the next frame closes the right one", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = EXT.slice(0, 5);
  const a = await live(s);
  s.emit("t1", EXT[5]);
  await a.api.settled();
  assert.equal(a.screen().tabs.length, 2);
  assert.equal(a.screen().tabs[0].uri, "srwr-replay:/live/diff5/前 ⚠ srwrの外で変更：stats.go?diff=5&side=before");
  s.emit("t1", EXT[6]);
  await a.api.settled();
  assert.equal(a.screen().tabs.length, 1);
  assert.equal(a.screen().tabs[0].uri, "srwr-replay:/live/stats.go");
});

test("closing: the server is told to stop and the view goes away", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  const a = await live(s);
  assert.equal(s.watching, true);
  await a.run("srwr.closeTape");
  assert.ok(s.calls.includes("liveStop"));
  assert.equal(s.watching, false);
  assert.deepEqual(a.screen().status, []);
  assert.deepEqual(a.screen().tree, []);
  s.emit("t1", WHY[1]); // a late frame is ignored
  assert.deepEqual(a.screen().tabs, []);
});

test("closed while waiting for the answer: the server is asked to stop", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  let release!: () => void;
  s.liveGate = new Promise<void>((r) => (release = r));
  app = start(s);
  const starting = app.run("srwr.liveStart");
  await app.run("srwr.liveStop");
  release();
  await starting;
  assert.ok(s.calls.includes("liveStop"));
  assert.deepEqual(app.screen().tree, []);
});

test("the server cannot be started: the view is closed and the error is shown", async () => {
  const s = new FakeServer();
  s.liveStart = async () => {
    throw new Error("boom");
  };
  app = start(s);
  await app.run("srwr.liveStart");
  assert.deepEqual(app.screen().status, []);
  assert.deepEqual(state.messages.map((m) => m.text), ["srwr: boom"]);
});

test("opening a tape closes the live view, and starting live closes the tape", async () => {
  const s = new FakeServer();
  s.liveTape = "t1";
  s.liveFrames = WHY.slice(0, 1);
  s.tapes = [{ tapeId: "20260930-0054-why-basic", startedAt: "2026-09-30T00:54:08+09:00", ops: 7, files: ["text.go"] }];
  s.frames.set("20260930-0054-why-basic", WHY as Frame[]);
  const a = await live(s);
  await a.run("srwr.openTape");
  assert.ok(s.calls.includes("liveStop"));
  assert.equal(a.screen().viewDescription, "20260930-0054-why-basic");
  await a.run("srwr.liveStart");
  assert.ok(s.calls.includes("close 20260930-0054-why-basic"));
  assert.equal(a.screen().viewDescription, "ライブ視聴");
});
