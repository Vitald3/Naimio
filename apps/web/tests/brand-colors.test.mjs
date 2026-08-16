import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const css = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");

test("Naimio keeps green primary and adds favicon-derived blue semantic tokens", () => {
  assert.match(css, /--brand:\s*#15956a/i);
  assert.match(css, /--secondary:\s*#335e7a/i);
  assert.match(css, /--secondary-dark:\s*#164f78/i);
  assert.match(css, /--secondary-deep:\s*#103b54/i);
  assert.match(css, /--secondary-soft:\s*#d4e8ef/i);
  assert.match(css, /--secondary-border:\s*#bed5e0/i);
});

test("blue is applied to hero, categories, trust and informational UI", () => {
  assert.match(css, /\.hero--premium[\s\S]*var\(--secondary-deep\)/);
  assert.match(css, /\.category-card[\s\S]*var\(--secondary-surface\)/);
  assert.match(css, /\.trust-story--premium[\s\S]*#103b54/i);
  assert.match(css, /\.deal-timeline li\.is-active[\s\S]*var\(--secondary-dark\)/);
  assert.match(css, /\.review-card__role[\s\S]*var\(--secondary-soft-2\)/);
});
