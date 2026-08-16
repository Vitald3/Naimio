import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("local-network pages have an insecure-context-compatible ID generator", async () => {
  const helper = await read("app/random-id.ts");
  const toast = await read("app/toast.tsx");
  assert.match(helper, /getRandomValues/);
  assert.match(helper, /randomUUID/);
  assert.match(toast, /createRandomID/);
  assert.doesNotMatch(toast, /crypto\.randomUUID/);
});

test("admin theme exposes the bright blue accent", async () => {
  const settings = await read("app/x7m4q9k2/settings/page.tsx");
  const provider = await read("app/site-settings.tsx");
  const css = await read("app/globals.css");
  assert.match(settings, /bright_blue_color/);
  assert.match(provider, /--bright-blue/);
  assert.match(css, /var\(--bright-blue\)/);
});
