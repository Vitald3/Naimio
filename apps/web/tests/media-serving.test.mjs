import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) =>
  readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("universal media serving and Next.js remotePatterns configuration", () => {
  const nextConfig = readFileSync(new URL("../next.config.js", import.meta.url), "utf8");
  assert.match(nextConfig, /remotePatterns/);
  assert.match(nextConfig, /hostname:\s*["']naimio\.ru["']/);
  assert.match(nextConfig, /pathname:\s*["']\/api\/v1\/media\/\*\*["']/);

  const media = read("media.ts");
  assert.match(media, /export function mediaURL/);
  assert.match(media, /\/api\/v1\/media\//);

  const blogEditor = read("blog-editor.tsx");
  assert.match(blogEditor, /mediaURL\(x\.media_id\)/);

  const blogList = read("blog/page.tsx");
  assert.match(blogList, /mediaURL\(post\.cover_url\)/);

  const blogDetail = read("blog/[slug]/page.tsx");
  assert.match(blogDetail, /mediaURL\(p\.cover_url\)/);

  const cms = read("x7m4q9k2/content/page.tsx");
  assert.match(cms, /mediaURL\(x\.media_id\)/);

  const portfolio = read("freelancers/[username]/portfolio-gallery.tsx");
  assert.match(portfolio, /mediaURL/);
});
