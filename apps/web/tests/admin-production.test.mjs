import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
const read = (path) => readFile(new URL(`../app/x7m4q9k2/${path}`, import.meta.url), "utf8");
test("production admin routes are backed by real admin APIs", async () => {
  const files = {
    users: await read("users/page.tsx"), reputation: await read("reputation/page.tsx"), reports: await read("reports/page.tsx"), fraud: await read("fraud/page.tsx"),
    projects: await read("projects/page.tsx"), services: await read("services/page.tsx"), vacancies: await read("vacancies/page.tsx"), reviews: await read("reviews/page.tsx"),
    disputes: await read("disputes/page.tsx"), flags: await read("feature-flags/page.tsx"), audit: await read("audit/page.tsx"), safe: await read("safe-deals/page.tsx"), taxonomy: await read("taxonomy/page.tsx")
  };
  assert.match(files.users,/\/api\/v1\/admin\/users/); assert.match(files.reputation,/external-reputations/); assert.match(files.reports,/\/api\/v1\/admin\/reports/);
  assert.match(files.fraud,/fraud-signals/); assert.match(await read("content-admin.tsx"),/\/api\/v1\/admin\/\$\{endpoint\}/); assert.match(files.reviews,/\/api\/v1\/admin\/reviews/);
  assert.match(files.disputes,/\/api\/v1\/admin\/disputes/); assert.match(files.flags,/feature-flags/); assert.match(files.audit,/\/api\/v1\/admin\/audit/); assert.match(files.safe,/safe-deals/); assert.match(files.taxonomy,/admin\/categories/); assert.match(files.taxonomy,/admin\/skills/);
});
test("mandatory admin UI has no unavailable-section placeholder", async () => {
  const nav = await read("admin-nav.tsx");
  const home = await read("page.tsx");
  for (const source of [nav,home]) assert.doesNotMatch(source,/Недоступные разделы|coming soon|not implemented/i);
  assert.match(nav,/STAFF_BASE_PATH/); for (const route of ["users","taxonomy","reputation","reports","fraud","projects","services","vacancies","reviews","matching","safe-deals","disputes","feature-flags","audit"]) assert.match(nav,new RegExp(`STAFF_BASE_PATH}\/${route}`));
});
test("sensitive admin actions require reason and audited domain routes", async () => {
  const ui = await read("admin-ui.tsx"); const users = await read("users/page.tsx"); const content = await read("content-admin.tsx"); const disputes = await read("disputes/page.tsx");
  assert.match(ui,/Причина/); assert.match(users,/status/); assert.match(content,/moderation/); assert.match(disputes,/Idempotency-Key/);
  assert.doesNotMatch(await read("safe-deals/page.tsx"),/method:\s*"PATCH"/);
});
