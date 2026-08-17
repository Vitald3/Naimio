import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) =>
  readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("universal media serving endpoint is used across frontend components", () => {
  const blogEditor = read("blog-editor.tsx");
  assert.match(blogEditor, /\/api\/v1\/media\/\$\{x\.media_id\}/);
  assert.doesNotMatch(blogEditor, /\/api\/v1\/blog\/media\//);

  const cms = read("x7m4q9k2/content/page.tsx");
  assert.match(cms, /\/api\/v1\/media\/\$\{x\.media_id\}/);
  assert.match(cms, /\/api\/v1\/media\/\$\{value\.cover_media_object_id\}/);
  assert.doesNotMatch(cms, /\/api\/v1\/blog\/media\//);

  const portfolio = read("freelancers/[username]/portfolio-gallery.tsx");
  assert.match(portfolio, /\/api\/v1\/media\/\$\{mediaID\}/);
});
