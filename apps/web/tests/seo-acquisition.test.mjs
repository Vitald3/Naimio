import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("SEO foundation has canonical metadata, bounded sitemap and private noindex", () => {
  assert.match(read("seo.ts"), /alternates:\s*\{\s*canonical/);
  assert.match(read("robots.ts"), /\/dashboard\//);
  assert.match(read("sitemap.ts"), /\/api\/v1\/seo\/sitemap/);
  assert.match(read("dashboard/layout.tsx"), /index:\s*false/);
  assert.match(read("messages/layout.tsx"), /index:\s*false/);
});

test("public entity pages render safe structured data from public fields", () => {
  for (const path of ["categories/[slug]/page.tsx", "freelancers/[username]/page.tsx", "services/[id]/page.tsx", "projects/[id]/page.tsx", "vacancies/[id]/page.tsx"]) {
    const source = read(path);
    assert.match(source, /application\/ld\+json/);
    assert.match(source, /jsonLD/);
    assert.doesNotMatch(source, /evidence|source_snapshot|moderation_reason/);
  }
});

test("calculator acquisition is limited to three validated definitions and preserves a draft", () => {
  const index = read("price/page.tsx");
  const detail = read("price/[slug]/page.tsx");
  const form = read("price/[slug]/calculator.tsx");
  for (const slug of ["telegram-bot", "landing-page", "seo"]) assert.match(index, new RegExp(slug));
  assert.match(detail, /generateStaticParams/);
  assert.match(form, /\/api\/v1\/calculators\//);
  assert.match(form, /create-project\?draft=/);
  assert.match(form, /CALCULATOR_STARTED/);
});

test("analytics sends only allowlisted attribution metadata, never acquisition content", () => {
  const analytics = read("analytics.ts");
  assert.match(analytics, /allowedMetadata/);
  assert.match(analytics, /utm_source/);
  assert.doesNotMatch(analytics, /text|prompt|description/);
  assert.match(read("check-offer/page.tsx"), /COMMERCIAL_OFFER_ANALYZED/);
  assert.match(read("create-project/page.tsx"), /attribution\(\)/);
});
