import assert from "node:assert/strict";
import { test } from "node:test";
import { insertBanner, wrapWhy } from "../src/lines";

test("wrapWhy: one short row starts with the mark", () => {
  assert.deepEqual(wrapWhy("理由です", 100), ["◆ 理由です"]);
});

test("wrapWhy: wide characters count 2, rows are indented, and the whole why is kept", () => {
  const why = "あ".repeat(10); // 20 cells
  const rows = wrapWhy(why, 12); // room = 10 cells = 5 characters
  assert.deepEqual(rows, ["◆ あああああ", "  あああああ"]);
  assert.equal(rows.join("").replace(/[◆ ]/g, ""), why);
});

test("wrapWhy: ASCII counts 1", () => {
  assert.deepEqual(wrapWhy("abcdefghij", 8 + 2), ["◆ abcdefgh", "  ij"]);
});

test("wrapWhy: a newline starts a new row, an empty line stays", () => {
  assert.deepEqual(wrapWhy("a\n\nb", 100), ["◆ a", "  ", "  b"]);
});

test("wrapWhy: a very narrow width still makes progress", () => {
  const rows = wrapWhy("abcdefghijklmnop", 1);
  assert.ok(rows.length >= 2);
  assert.equal(rows.join("").replace(/[◆ ]/g, ""), "abcdefghijklmnop");
});

test("insertBanner: before line `at`, keeping the final newline", () => {
  assert.equal(insertBanner("a\nb\nc\n", 2, ["◆ x"]), "a\n◆ x\nb\nc\n");
  assert.equal(insertBanner("a\nb\nc", 2, ["◆ x"]), "a\n◆ x\nb\nc");
});

test("insertBanner: a line outside the text goes to the start or the end", () => {
  assert.equal(insertBanner("a\nb\n", 0, ["R"]), "R\na\nb\n");
  assert.equal(insertBanner("a\nb\n", 99, ["R"]), "a\nb\nR\n");
  assert.equal(insertBanner("", 1, ["R"]), "R\n");
});
