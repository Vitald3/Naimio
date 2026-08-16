import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

async function read(path) {
  return readFile(new URL(path, import.meta.url), "utf8");
}

test("display typography uses rounder Nunito for large headings", async () => {
  const css = await read("../app/globals.css");
  assert.doesNotMatch(css, /fonts\.(?:googleapis|gstatic)\.com/);
  assert.match(css, /--font-display:\s*ui-rounded,\s*"SF Pro Rounded",\s*"Nunito"/);
  assert.match(css, /--font-display:\s*ui-rounded,\s*"SF Pro Rounded",\s*"Nunito"/);
});

test("frontend media uses next image instead of raw img tags", async () => {
  const media = await read("../app/media-components.tsx");
  const home = await read("../app/page.tsx");
  assert.match(media, /from "next\/image"/);
  assert.match(home, /from "next\/image"/);
  assert.doesNotMatch(media, /<img\b/);
  assert.doesNotMatch(home, /<img\b/);
});

test("web toolchain is on maintained security-patched line", async () => {
  const pkg = JSON.parse(await read("../package.json"));
  assert.equal(pkg.dependencies.next, "16.3.0");
  assert.equal(pkg.dependencies.react, "19.2.8");
  assert.equal(pkg.devDependencies.eslint, "9.39.5");
  assert.equal(pkg.scripts.lint, "eslint . --max-warnings=0");
});
