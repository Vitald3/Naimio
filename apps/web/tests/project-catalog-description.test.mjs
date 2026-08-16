import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("project catalog renders rich descriptions as plain text excerpts", async () => {
  const source = await read("app/projects/page.tsx");
  assert.match(source, /const plainDescription =/);
  assert.match(source, /replace\(\/<\[\^>\]\+>\/g, " "\)/);
  assert.match(source, /plainDescription\(item\.description\)/);
  assert.doesNotMatch(source, /list-card__desc">\{item\.description\}/);
});
