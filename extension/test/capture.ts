// Captures what the screen shows for every frame, in the shape of baseline/*.json (the shape of the first baseline, taken from the previous implementation):
// the real extension and the real bin/srwr, on the fake vscode, in a copy of the fixed workspace.
import fs from "node:fs";
import path from "node:path";
import { state, Screen, screen } from "./fakevscode";
import { start } from "./harness";
import { ServerProcess } from "../src/server";
import { binPath, copyWorkspace, tapeLines, until } from "./helpers";

export interface Shot extends Screen {
  frame: number;
  label?: string;
  quickPick?: unknown;
  viewMessage?: string; // the line under the heading of the list; only in captureKinds, so the older baselines are unchanged
}

function newApp(root: string) {
  const servers: ServerProcess[] = [];
  const app = start(() => {
    const s = new ServerProcess({ root, command: () => binPath });
    servers.push(s);
    return s;
  }, root);
  state.config.path = binPath;
  return {
    app,
    close: () => {
      app.dispose();
      for (const s of servers) {
        s.dispose();
      }
    },
  };
}

function position(): number {
  const t = screen().status.find((s) => /^\d+\/\d+$/.test(s.text));
  return t ? Number(t.text.split("/")[0]) - 1 : -1;
}

// Opens a tape and shows every frame in turn (frame 0 is shown by opening it).
export async function captureReplay(tapeId: string, extra?: string): Promise<Shot[]> {
  const { app, close } = newApp(copyWorkspace(extra));
  try {
    state.pickQuickPick = (items) => items.find((i) => i.tape.tapeId === tapeId);
    await app.run("srwr.openTape");
    const shots: Shot[] = [];
    const n = screen().tree.length;
    for (let i = 0; i < n; i++) {
      if (i > 0) {
        await app.run("srwr.goto", i);
      }
      shots.push({ frame: position(), ...screen(), quickPick: state.quickPick });
    }
    return shots;
  } finally {
    close();
  }
}

// Opens a tape (failure is left out), takes every frame; then turns failure on with the funnel button (the same command, all four
// kinds picked) and takes every frame again. The labels are d1... for the first, f1... for the second.
export async function captureKinds(tapeId: string, extra?: string): Promise<Shot[]> {
  const { app, close } = newApp(copyWorkspace(extra));
  try {
    state.pickQuickPick = (items) => items.find((i) => i.tape?.tapeId === tapeId);
    await app.run("srwr.openTape");
    const shots: Shot[] = [];
    const take = async (prefix: string): Promise<void> => {
      const n = screen().tree.length;
      for (let i = 0; i < n; i++) {
        await app.run("srwr.goto", i);
        shots.push({ frame: position(), label: `${prefix}${i + 1}`, ...screen(), viewMessage: viewMessage() });
      }
    };
    await take("d");
    state.pickQuickPick = (items) => items.filter((i) => i.shown !== undefined); // all four kinds
    await app.run("srwr.chooseKinds");
    await take("f");
    return shots;
  } finally {
    close();
  }
}

// What the list says under its heading ("Hiding: failure (2)").
function viewMessage(): string | undefined {
  return state.trees.get("srwr.ops")?.view.message;
}

type Stage = { label: string; do: (h: Live) => Promise<void> };

class Live {
  appended = 0; // lines of the tape written so far
  frames = 0; // frames those lines make
  constructor(
    readonly lines: string[],
    readonly tape: string,
    readonly app: ReturnType<typeof start>,
  ) {}
  // Appends lines, one at a time, until the tape holds `n` frames, and waits until the view has them all.
  async appendUntil(n: number): Promise<void> {
    while (this.frames < n) {
      const line = this.lines[this.appended++];
      fs.appendFileSync(this.tape, line + "\n");
      if (isFrameLine(line)) {
        this.frames++;
        const want = this.frames;
        await until(`frame ${want} to reach the list`, () => screen().tree.length === want);
      }
    }
    await this.app.api.settled();
  }
}

function isFrameLine(line: string): boolean {
  const t = (JSON.parse(line) as { type: string }).type;
  return t === "select" || t === "replace" || t === "external";
}

// Follows a fixed tape that is written into the workspace stage by stage.
export async function captureLive(tapeId: string, stages: Stage[], firstFrames: number): Promise<Shot[]> {
  const root = copyWorkspace();
  const tapes = path.join(root, ".srwr", "tapes");
  for (const f of fs.readdirSync(tapes)) {
    fs.rmSync(path.join(tapes, f));
  }
  const tape = path.join(tapes, "20260930-0000-live.tape.jsonl");
  fs.writeFileSync(tape, "");
  const { app, close } = newApp(root);
  try {
    const live = new Live(tapeLines(tapeId), tape, app);
    // The frames before the start: written first, so the server hands them over with live/start.
    while (live.frames < firstFrames) {
      const line = live.lines[live.appended++];
      fs.appendFileSync(tape, line + "\n");
      if (isFrameLine(line)) {
        live.frames++;
      }
    }
    await app.run("srwr.liveStart");
    await until("the list", () => screen().tree.length === firstFrames);
    const shots: Shot[] = [];
    for (const st of stages) {
      await st.do(live);
      await app.api.settled();
      shots.push({ label: st.label, frame: position(), ...screen() });
    }
    return shots;
  } finally {
    close();
  }
}

export const liveBasic = (): Promise<Shot[]> => {
  const append = (n: number) => async (h: Live) => h.appendUntil(n);
  return captureLive(
    "20260930-0054-why-basic",
    [
      { label: "L1_waiting", do: async () => undefined },
      { label: "L2_following_1", do: append(2) },
      { label: "L3_following_3", do: append(3) },
      { label: "L4_stepped_back", do: async (h) => void (await h.app.run("srwr.stepBack")) },
      { label: "L5_behind_2", do: append(5) },
      { label: "L6_behind_4", do: append(7) },
      { label: "L7_back_to_live", do: async (h) => void (await h.app.run("srwr.liveLatest")) },
    ],
    1,
  );
};

export const liveExt = (): Promise<Shot[]> => {
  const append = (n: number) => async (h: Live) => h.appendUntil(n);
  return captureLive(
    "20260930-0949-external",
    [
      { label: "E0_waiting", do: async () => undefined },
      { label: "E1_following", do: append(5) },
      { label: "E2_external", do: append(6) },
      { label: "E3_after_external", do: append(7) },
    ],
    1,
  );
};

// `node --require ./out/test/setup.js out/test/capture.js <dir> [<extra workspace>]` writes the five files, to compare with baseline/ or to make a new
// baseline. With an extra workspace (qsoku ui-check makes one with the tape of a long why), long_why.json is written too.
async function main(): Promise<void> {
  const dir = process.argv[2];
  if (!dir) {
    throw new Error("usage: node out/test/capture.js <output dir>");
  }
  fs.mkdirSync(dir, { recursive: true });
  const out = (name: string, v: unknown): void => fs.writeFileSync(path.join(dir, `${name}.json`), JSON.stringify(v, null, 1) + "\n");
  const liveOnly = process.env.SRWR_CAPTURE_ONLY === "live"; // qsoku ui-live
  if (!liveOnly) {
    out("all_basic", await captureReplay("20260930-0054-why-basic"));
    out("all_ext", await captureReplay("20260930-0949-external"));
    out("all_nowhy", await captureReplay("20260930-0053-no-why"));
  }
  out("live_basic", await liveBasic());
  out("live_ext", await liveExt());
  const extra = process.argv[3];
  if (extra && extra !== "-" && !liveOnly) {
    out("long_why", await captureReplay(longWhyTape, extra));
  }
  const extraFailure = process.argv[4];
  if (extraFailure && !liveOnly) {
    out("with_failure", await captureKinds(failureTape, extraFailure));
  }
}

// The tape qsoku ui-check makes on the spot (tools/ui-check/maketape.go).
export const longWhyTape = "20260101-0000-long-why";
export const failureTape = "20260101-0001-with-failure";

if (require.main === module) {
  require("./setup");
  main().catch((e) => {
    console.error(e);
    process.exit(1);
  });
}
