// package.json and the code must name the same commands, views, settings and contexts.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { FakeServer } from "./fakeserver";
import { state } from "./fakevscode";
import { extDir } from "./helpers";
import { start } from "./harness";

const pkg = JSON.parse(fs.readFileSync(path.join(extDir, "package.json"), "utf8"));
const srcDir = path.join(extDir, "src");
const sources = fs
  .readdirSync(srcDir)
  .filter((f) => f.endsWith(".ts"))
  .map((f) => [f, fs.readFileSync(path.join(srcDir, f), "utf8")] as const);

// every "srwr.xxx" string literal in the sources
function srwrNames(): Set<string> {
  const out = new Set<string>();
  for (const [, text] of sources) {
    for (const m of text.matchAll(/"(srwr\.[A-Za-z]+)"/g)) {
      out.add(m[1]);
    }
  }
  return out;
}

const declared: string[] = pkg.contributes.commands.map((c: { command: string }) => c.command);

test("the commands the code registers are the commands package.json declares", () => {
  const a = start(new FakeServer());
  try {
    assert.deepEqual([...state.commands.keys()].sort(), [...declared].sort());
  } finally {
    a.dispose();
  }
});

test("every command the code or the UI refers to is declared", () => {
  const refs = new Set<string>();
  for (const n of srwrNames()) {
    if (n !== "srwr.ops" && n !== "srwr.hasTape" && n !== "srwr.path") {
      refs.add(n);
    }
  }
  for (const w of pkg.contributes.viewsWelcome) {
    for (const m of w.contents.matchAll(/command:(srwr\.[A-Za-z]+)/g)) {
      refs.add(m[1]);
    }
  }
  for (const list of Object.values(pkg.contributes.menus) as Array<Array<{ command: string }>>) {
    for (const m of list) {
      refs.add(m.command);
    }
  }
  for (const r of refs) {
    assert.ok(declared.includes(r), `${r} is used but not declared`);
  }
});

test("the view the code creates is the view package.json declares", () => {
  const a = start(new FakeServer());
  try {
    const ids = pkg.contributes.views.srwr.map((v: { id: string }) => v.id);
    assert.deepEqual([...state.trees.keys()], ids);
    for (const w of pkg.contributes.viewsWelcome) {
      assert.ok(ids.includes(w.view));
    }
    for (const m of pkg.contributes.menus["view/title"]) {
      assert.ok(m.when.includes(`view == ${ids[0]}`));
    }
    assert.equal(pkg.contributes.viewsContainers.activitybar[0].id, "srwr");
    assert.ok(fs.existsSync(path.join(extDir, pkg.contributes.viewsContainers.activitybar[0].icon)), "the icon exists");
  } finally {
    a.dispose();
  }
});

test("the context key in the menus and the welcome text is the one the code sets", async () => {
  const s = new FakeServer();
  s.liveTape = null;
  const a = start(s);
  try {
    await a.run("srwr.liveStart");
    assert.equal(state.contexts.get("srwr.hasTape"), true);
    const whens: string[] = [...pkg.contributes.menus["view/title"].map((m: { when: string }) => m.when), ...pkg.contributes.viewsWelcome.map((w: { when: string }) => w.when)];
    const keys = new Set(whens.flatMap((w) => [...w.matchAll(/srwr\.[A-Za-z]+/g)].map((m) => m[0])).filter((k) => k !== "srwr.ops"));
    assert.deepEqual([...keys], ["srwr.hasTape"]);
  } finally {
    a.dispose();
  }
});

test("the only setting is srwr.path, and the code reads it", () => {
  const props = Object.keys(pkg.contributes.configuration.properties);
  assert.deepEqual(props, ["srwr.path"]);
  assert.equal(pkg.contributes.configuration.properties["srwr.path"].default, "srwr");
  assert.ok(sources.find(([f]) => f === "config.ts")![1].includes('getConfiguration("srwr")'));
});

test("the goto command is not in the command palette", () => {
  assert.deepEqual(pkg.contributes.menus.commandPalette, [{ command: "srwr.goto", when: "false" }]);
});

test("the status bar items run declared commands", async () => {
  const a = start(new FakeServer());
  try {
    await a.run("srwr.liveStart");
    for (const it of state.statusItems) {
      if (it.command) {
        assert.ok(declared.includes(it.command), it.command);
      }
    }
  } finally {
    a.dispose();
  }
});

test("package.json: no runtime dependencies, main is where tsc puts it", () => {
  assert.equal(pkg.dependencies, undefined);
  assert.equal(pkg.main, "./out/src/extension.js");
  assert.ok(fs.existsSync(path.join(extDir, pkg.main)));
  assert.equal(pkg.name, "srwr-view");
});

test("the extension does not import anything but vscode and node's own modules", () => {
  for (const [f, text] of sources) {
    for (const m of text.matchAll(/from "([^"]+)"/g)) {
      const spec = m[1];
      assert.ok(spec.startsWith("./") || spec === "vscode" || spec.startsWith("node:"), `${f} imports ${spec}`);
    }
  }
});

test("server.ts and timeline.ts do not import vscode", () => {
  for (const f of ["server.ts", "timeline.ts", "lines.ts"]) {
    assert.ok(!sources.find(([n]) => n === f)![1].includes('from "vscode"'), f);
  }
});

// The texts of package.json are %keys%; each key has an English text (package.nls.json) and a Japanese one (package.nls.ja.json).
test("every %key% of package.json has an English and a Japanese text, and none is left over", () => {
  const used = new Set([...JSON.stringify(pkg).matchAll(/%([A-Za-z0-9_.-]+)%/g)].map((m) => m[1]));
  const read = (f: string): Record<string, string> => JSON.parse(fs.readFileSync(path.join(extDir, f), "utf8"));
  const en = read("package.nls.json");
  const ja = read("package.nls.ja.json");
  assert.deepEqual([...used].sort(), Object.keys(en).sort(), "English");
  assert.deepEqual([...used].sort(), Object.keys(ja).sort(), "Japanese");
  assert.ok(Object.values(en).every((v) => !/[぀-ヿ一-鿿]/.test(v)), "the English texts have no Japanese");
  assert.ok(!/[぀-ヿ一-鿿]/.test(JSON.stringify({ ...pkg, contributes: undefined })), "package.json itself has no Japanese");
  const vsix = fs.readFileSync(path.join(extDir, ".vscodeignore"), "utf8");
  assert.ok(vsix.includes("!package.nls*.json"), "the texts are in the .vsix");
});

// What the Marketplace asks of a listing (docs: .claude/rules/distribution.md).
test("the listing has an icon (a PNG of 128 pixels or more), keywords and a homepage", () => {
  assert.equal(pkg.icon, "media/icon.png");
  const png = fs.readFileSync(path.join(extDir, pkg.icon));
  assert.equal(png.subarray(0, 8).toString("hex"), "89504e470d0a1a0a", "not a PNG");
  assert.ok(png.readUInt32BE(16) >= 128 && png.readUInt32BE(20) >= 128, "the icon is smaller than 128 pixels");
  assert.ok(Array.isArray(pkg.keywords) && pkg.keywords.length > 0 && pkg.keywords.every((k: unknown) => typeof k === "string" && k !== ""));
  assert.match(pkg.homepage, /^https:\/\/github\.com\/amisonnet8\/srwr/);
  assert.match(pkg.bugs.url, /^https:\/\/github\.com\/amisonnet8\/srwr\/issues$/);
  assert.ok(pkg.categories.length > 0);
});

test("the pictures the README of the extension shows are packaged (and are PNG, which vsce accepts)", () => {
  const readme = fs.readFileSync(path.join(extDir, "README.md"), "utf8");
  const local = [...readme.matchAll(/(?:src="|\]\()(media\/[^")\s]+)/g)].map((m) => m[1]);
  assert.ok(local.length > 0, "the README shows no picture of its own");
  const ignore = fs.readFileSync(path.join(extDir, ".vscodeignore"), "utf8");
  assert.ok(ignore.includes("!media/**"), "media/ is not packaged");
  for (const f of local) {
    assert.ok(f.endsWith(".png"), `${f}: vsce does not accept an SVG in a README`);
    assert.ok(fs.existsSync(path.join(extDir, f)), `${f} is missing`);
  }
});
