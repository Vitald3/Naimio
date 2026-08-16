import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) =>
  readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("PRO checkout is provider-backed and browser return is not payment authority", () => {
  const page = read("pro/page.tsx");
  const returned = read("pro/payment-return/page.tsx");
  assert.match(page, /\/api\/v1\/monetization/);
  assert.match(page, /\/api\/v1\/me\/subscription/);
  assert.match(page, /\/api\/v1\/me\/pro-billing\/checkout/);
  assert.match(page, /\/api\/v1\/me\/pro-billing\/recover/);
  assert.match(page, /Повторить оплату \/ сменить способ/);
  assert.match(page, /Платёжный маршрут не настроен/);
  assert.match(page, /Сравнить возможности/);
  assert.match(returned, /\/api\/v1\/me\/pro-billing\/status/);
  assert.match(returned, /Проверяем оплату/);
  assert.doesNotMatch(returned, /success=true/);
});

test("effective PRO is rendered consistently on profiles and catalog cards", () => {
  assert.match(read("pro-badge.tsx"), /PRO/);
  assert.match(read("freelancers/[username]/page.tsx"), /effective_pro/);
  assert.match(
    read("freelancers/page.tsx"),
    /item\.effective_pro\s*\?\s*<ProBadge/,
  );
});

test("PRO analytics records supported public views and has a gated account area", () => {
  const tracker = read("engagement-tracker.tsx");
  const analytics = read("dashboard/analytics/page.tsx");
  assert.match(tracker, /\/api\/v1\/engagement\/events/);
  assert.match(read("freelancers/[username]/page.tsx"), /PROFILE_VIEW/);
  assert.match(read("freelancers/[username]/portfolio-gallery.tsx"), /PORTFOLIO_VIEW/);
  assert.match(read("services/[id]/page.tsx"), /SERVICE_VIEW/);
  assert.match(analytics, /\/api\/v1\/me\/analytics/);
  assert.match(analytics, /Расширенная аналитика доступна в PRO/);
});

test("blog pages expose SEO metadata and structured data", () => {
  const article = read("blog/[slug]/page.tsx");
  assert.match(article, /generateMetadata/);
  assert.match(article, /<Breadcrumbs/);
  assert.match(article, /Article/);
  assert.match(article, /sanitizeHtml/);
  assert.match(read("sitemap.ts"), /blogResponse\.ok/);
});

test("staff tools manage monetization and CMS through audited APIs", () => {
  const monetization = read("x7m4q9k2/monetization/page.tsx");
  const cms = read("x7m4q9k2/content/page.tsx");
  assert.match(monetization, /\/api\/v1\/admin\/monetization/);
  assert.match(monetization, /Причина изменения/);
  const payments = read("x7m4q9k2/payments/page.tsx");
  assert.match(payments, /OTHER_PLATFORM_PAYMENT/);
  assert.match(payments, /operation/);
  assert.match(payments, /reference/);
  assert.match(payments, /action\("cancel"/);
  assert.match(cms, /\/api\/v1\/admin\/blog/);
  assert.match(cms, /Причина изменения для аудита/);
  assert.match(read("blog-editor.tsx"), /@tiptap\/react/);
});
