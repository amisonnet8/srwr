import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { Frame, toFrame } from "../src/timeline";

// out/test/helpers.js -> extension/
export const extDir = path.resolve(__dirname, "..", "..");
export const repoDir = path.resolve(extDir, "..");
export const goldenDir = path.join(extDir, "test", "golden");
export const fixtureDir = path.join(extDir, "test", "fixtures");
export const uiCheckDir = path.join(fixtureDir, "ui-check");
export const baselineDir = path.join(extDir, "test", "baseline");
export const binPath = path.join(repoDir, "bin", "srwr" + (process.platform === "win32" ? ".exe" : ""));

export interface Golden {
  name: string;
  tape?: string;
  current: { kind: string; files?: Record<string, string>; dir?: string };
  frames: Array<Record<string, unknown>>;
  files: string[];
  contentAt: Record<string, Array<string | null>>;
}

export function goldenNames(): string[] {
  return fs
    .readdirSync(goldenDir)
    .filter((f) => f.endsWith(".json"))
    .map((f) => f.slice(0, -5))
    .sort();
}

export function loadGolden(name: string): Golden {
  return JSON.parse(fs.readFileSync(path.join(goldenDir, `${name}.json`), "utf8")) as Golden;
}

// The golden data was written for the tape of version 1, which said select and replace for what are look and edit now.
const GOLDEN_KINDS: Record<string, string> = { select: "look", replace: "edit", sub: "replace" };

export function goldenKind(kind: string): string {
  return GOLDEN_KINDS[kind] ?? kind;
}

export function goldenFrames(g: Golden): Frame[] {
  return g.frames.map((f) => toFrame({ ...f, kind: goldenKind(f.kind as string) }));
}

// Copies the fixed workspace (three tapes and the real files) to a new temporary directory.
// `extra` is another directory copied over it (qsoku ui-check adds the tape it made on the spot).
export function copyWorkspace(extra?: string): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "srwr-ext-"));
  fs.cpSync(uiCheckDir, dir, { recursive: true });
  if (extra) {
    fs.cpSync(extra, dir, { recursive: true });
  }
  return dir;
}

export function tapeLines(tapeId: string): string[] {
  return fs
    .readFileSync(path.join(uiCheckDir, ".srwr", "tapes", `${tapeId}.tape.jsonl`), "utf8")
    .split("\n")
    .filter((l) => l !== "");
}

// Waits until cond() is true. No fixed sleep: it polls, and fails with `what` at the timeout.
export async function until(what: string, cond: () => boolean | Promise<boolean>, timeoutMs = 10000): Promise<void> {
  const end = Date.now() + timeoutMs;
  while (!(await cond())) {
    if (Date.now() > end) {
      throw new Error(`timed out waiting for ${what}`);
    }
    await new Promise((r) => setTimeout(r, 5));
  }
}
