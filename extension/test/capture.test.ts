// Every frame's screen equals the baseline (test/baseline/<lang>/*.json; the Japanese ones were first taken from the previous
// implementation), in English and in Japanese.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { Shot, captureReplay, liveBasic, liveExt } from "./capture";
import { baselineDir } from "./helpers";

function baseline(lang: string, name: string): Array<Record<string, unknown>> {
  return JSON.parse(fs.readFileSync(path.join(baselineDir, lang, `${name}.json`), "utf8"));
}

function same(lang: string, name: string, got: Shot[]): void {
  const want = baseline(lang, name);
  assert.equal(got.length, want.length, `${name}: number of frames`);
  got.forEach((g, i) => {
    const actual = JSON.parse(JSON.stringify(g)) as Record<string, unknown>;
    const w = want[i];
    for (const key of ["label", "frame", "tabs", "tree", "status", "viewDescription", "quickPick"]) {
      assert.deepStrictEqual(actual[key], w[key], `${name} #${i + 1} (${(w.label as string) ?? `frame ${w.frame}`}): ${key}`);
    }
  });
}

for (const lang of ["en", "ja"]) {
  for (const [name, tape] of [
    ["all_basic", "20260930-0054-why-basic"],
    ["all_ext", "20260930-0949-external"],
    ["all_nowhy", "20260930-0053-no-why"],
  ] as const) {
    test(`[${lang}] every frame of ${tape} looks like the baseline ${name}`, async () => {
      process.env.SRWR_TEST_LANG = lang;
      same(lang, name, await captureReplay(tape));
    });
  }

  test(`[${lang}] every stage of the live view of why-basic looks like the baseline live_basic`, async () => {
    process.env.SRWR_TEST_LANG = lang;
    same(lang, "live_basic", await liveBasic());
  });

  test(`[${lang}] every stage of the live view of external looks like the baseline live_ext`, async () => {
    process.env.SRWR_TEST_LANG = lang;
    same(lang, "live_ext", await liveExt());
  });
}
