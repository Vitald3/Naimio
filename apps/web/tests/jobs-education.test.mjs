import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("vacancy UI keeps applications distinct and exposes required views", () => {
  const catalog = read("vacancies/page.tsx");
  const detail = read("vacancies/[id]/page.tsx");
  const form = read("vacancy/[id]/application-form.tsx");
  const owner = read("dashboard/vacancies/page.tsx");
  const mine = read("dashboard/job-applications/page.tsx");
  const edit = read("dashboard/vacancies/[id]/edit/page.tsx");
  assert.match(catalog, /\/api\/v1\/vacancies/);
  assert.match(detail, /ApplicationForm/);
  assert.match(form, /Откликнуться/);
  assert.match(form, /не предложение по проекту/);
  assert.match(owner, /applications/);
  assert.match(mine, /Мои отклики/);
  assert.match(edit, /method:"PATCH"/);
});

test("education is a filtered service catalog with conditional creation fields", () => {
  const catalog = read("education/page.tsx");
  const create = read("dashboard/services/new/page.tsx");
  assert.match(catalog, /service_type/);
  assert.match(catalog, /audience/);
  assert.match(catalog, /format/);
  assert.match(catalog, /без LMS/);
  for (const type of ["CONSULTATION", "EDUCATION", "MENTORING"]) assert.match(create, new RegExp(type));
  for (const field of ["duration_minutes", "sessions_count", "audience_type", "group_size_max"]) assert.match(create, new RegExp(field));
});

test("vacancy moderation requires a reason", () => {
  const moderation = read("x7m4q9k2/vacancies/page.tsx");
  const shared = read("x7m4q9k2/content-admin.tsx");
  assert.match(moderation, /ContentAdmin/);
  assert.match(shared, /reason/);
  assert.match(shared, /\/moderation/);
});
