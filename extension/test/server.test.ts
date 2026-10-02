// ServerProcess against the real bin/srwr (built by `qsoku ext` / `qsoku bin`), and against small stand-in programs.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { afterEach, test } from "node:test";
import { ServerError, ServerProcess } from "../src/server";
import { Frame } from "../src/timeline";
import { binPath, copyWorkspace, goldenFrames, loadGolden, tapeLines, until } from "./helpers";

const servers: ServerProcess[] = [];
afterEach(() => {
  for (const s of servers.splice(0)) {
    s.dispose();
  }
});

function real(root: string): ServerProcess {
  assert.ok(fs.existsSync(binPath), `${binPath} does not exist: run qsoku bin`);
  const s = new ServerProcess({ root, command: () => binPath });
  servers.push(s);
  return s;
}

// A stand-in for the server: runs a node script that gets lines on stdin.
function standIn(script: string, extra: { timeoutMs?: number } = {}): ServerProcess {
  const s = new ServerProcess({ root: "/", command: () => process.execPath, args: ["-e", script], ...extra });
  servers.push(s);
  return s;
}

const WHY = "20260930-0054-why-basic";
const EXT = "20260930-0949-external";
const NOWHY = "20260930-0053-no-why";

test("tapes/list: the three fixed tapes, newest first, with their operations", async () => {
  const s = real(copyWorkspace());
  const tapes = await s.listTapes();
  assert.deepEqual(
    tapes.map((t) => [t.tapeId, t.ops]),
    [
      [EXT, 9],
      [WHY, 7],
      [NOWHY, 10],
    ],
  );
  assert.deepEqual(tapes[1].files, ["text.go", "parse.go"]);
});

for (const [id, golden] of [
  [WHY, "ui-check-why-basic"],
  [EXT, "ui-check-external"],
  [NOWHY, "ui-check-no-why"],
] as const) {
  test(`tape/open ${id}: every frame equals the golden data (with the final diff against the real files)`, async () => {
    const s = real(copyWorkspace());
    const got = await s.openTape(id);
    const want = goldenFrames(loadGolden(golden));
    assert.equal(got.length, want.length);
    got.forEach((f, i) => assert.deepEqual(f, want[i], `frame ${i}`));
  });
}

test("live: the frames so far, then the appended ones in order", async () => {
  const root = copyWorkspace();
  const tapes = path.join(root, ".srwr", "tapes");
  for (const f of fs.readdirSync(tapes)) {
    fs.rmSync(path.join(tapes, f));
  }
  const lines = tapeLines(WHY);
  const tape = path.join(tapes, "20260930-0000-live.tape.jsonl");
  fs.writeFileSync(tape, lines.slice(0, 3).join("\n") + "\n"); // header, snapshot, first select
  const s = real(root);
  const got: Array<[string, Frame]> = [];
  const r = await s.liveStart((id, f) => got.push([id, f]));
  assert.equal(r.tapeId, "20260930-0000-live");
  assert.equal(r.frames.length, 1);
  for (const l of lines.slice(3)) {
    fs.appendFileSync(tape, l + "\n");
  }
  const want = goldenFrames(loadGolden("ui-check-why-basic"));
  await until("the appended frames", () => got.length === want.length - 1);
  got.forEach(([id, f], i) => {
    assert.equal(id, "20260930-0000-live");
    assert.deepEqual(f, want[i + 1]);
  });
  await s.liveStop();
});

test("tape/open of a tape that is not there", async () => {
  const s = real(copyWorkspace());
  await assert.rejects(s.openTape("20200101-0000-none"), (e: ServerError) => e.code === "tape_not_found");
  await assert.rejects(s.openTape("../x"), (e: ServerError) => e.code === "invalid_params");
});

test("the binary does not exist", async () => {
  const s = new ServerProcess({ root: "/", command: () => "/no/such/srwr" });
  servers.push(s);
  await assert.rejects(s.listTapes(), (e: ServerError) => e.code === "binary_not_found" && e.message.includes("/no/such/srwr"));
  // and again: it can be tried again, say after the setting was fixed
  await assert.rejects(s.listTapes(), (e: ServerError) => e.code === "binary_not_found");
});

test("the server exits: waiting requests fail with the end of its error output", async () => {
  const s = standIn("console.error('line1');console.error('boom');process.stdin.once('data',()=>process.exit(3))");
  await assert.rejects(s.listTapes(), (e: ServerError) => e.code === "server_exited" && e.message.includes("コード 3") && e.message.includes("boom"));
});

test("a server that says the protocol does not match", async () => {
  const s = standIn(`
    require('readline').createInterface({input: process.stdin}).on('line', (l) => {
      const m = JSON.parse(l);
      console.log(JSON.stringify({jsonrpc:'2.0', id:m.id, error:{code:-32000, message:'x', data:{code:'protocol_mismatch'}}}));
    });`);
  await assert.rejects(
    s.listTapes(),
    (e: ServerError) => e.code === "protocol_mismatch" && e.message.includes("バージョンが合っていません") && e.message.includes("protocolVersion 1"),
  );
});

test("a server that does not answer: request_timeout", async () => {
  const s = standIn("setInterval(()=>{},1000)", { timeoutMs: 50 });
  await assert.rejects(s.listTapes(), (e: ServerError) => e.code === "request_timeout" && e.message.includes("initialize"));
});

test("a line that is not JSON is skipped; replies are matched by id", async () => {
  const s = standIn(`
    require('readline').createInterface({input: process.stdin}).on('line', (l) => {
      const m = JSON.parse(l);
      console.log('not json');
      console.log(JSON.stringify({jsonrpc:'2.0', id:m.id, result: m.method === 'tapes/list' ? {tapes: []} : {}}));
    });`);
  assert.deepEqual(await s.listTapes(), []);
});

test("initialize is sent once, with the client name, protocol version and options", async () => {
  const s = standIn(`
    const seen = [];
    require('readline').createInterface({input: process.stdin}).on('line', (l) => {
      const m = JSON.parse(l);
      seen.push(m);
      const result = m.method === 'tapes/list' ? {tapes: [{tapeId: JSON.stringify(seen.map(x => [x.method, x.params])), startedAt: '', ops: 1, files: []}]} : {};
      console.log(JSON.stringify({jsonrpc:'2.0', id:m.id, result}));
    });`);
  await s.listTapes();
  const second = await s.listTapes();
  assert.deepEqual(JSON.parse(second[0].tapeId), [
    ["initialize", { client: "vscode", protocolVersion: 1, options: { diffFrames: true } }],
    ["tapes/list", {}],
    ["tapes/list", {}],
  ]);
});
