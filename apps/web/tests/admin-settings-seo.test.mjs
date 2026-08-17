import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../app/${path}`, import.meta.url), "utf8");

test("admin settings page contains all mandatory sections: appearance, SEO general, SEO pages, SEO templates, IndexNow, and storage", async () => {
  const page = await read("x7m4q9k2/settings/page.tsx");
  assert.match(page, /Внешний вид & Проект/);
  assert.match(page, /SEO Общие/);
  assert.match(page, /SEO Страницы/);
  assert.match(page, /SEO Шаблоны/);
  assert.match(page, /IndexNow & Индексация/);
  assert.match(page, /Хранилище файлов/);
  assert.match(page, /Система & Каталоги/);

  // Storage tests
  assert.match(page, /testS3Connection/);
  assert.match(page, /\/api\/v1\/admin\/storage-settings\/test/);

  // IndexNow batch submitter
  assert.match(page, /submitIndexNowBatch/);
  assert.match(page, /\/api\/v1\/admin\/indexnow\/submit/);

  // Feature flags saving
  assert.match(page, /site_appearance/);
  assert.match(page, /seo_settings/);
});

test("site-settings exports full SEO types and defaults", async () => {
  const settings = await read("site-settings.tsx");
  assert.match(settings, /export const defaultSEOSettings/);
  assert.match(settings, /default_title/);
  assert.match(settings, /title_template/);
  assert.match(settings, /indexnow/);
});
