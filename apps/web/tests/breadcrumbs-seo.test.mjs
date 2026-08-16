import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("breadcrumbs emit canonical BreadcrumbList structured data", async () => {
  const source = await read("app/breadcrumbs.tsx");
  assert.match(source, /"@type": "BreadcrumbList"/);
  assert.match(source, /position: index \+ 1/);
  assert.match(source, /canonical\(item\.href\)/);
  assert.match(source, /aria-label="Хлебные крошки"/);
});

test("specialists are a top-level catalog, not a child of categories", async () => {
  const list = await read("app/freelancers/page.tsx");
  const detail = await read("app/freelancers/[username]/page.tsx");
  assert.match(list, /label:"Специалисты"/);
  assert.doesNotMatch(list, /label:"Категории"/);
  assert.match(detail, /label:"Специалисты",href:"\/freelancers"/);
  assert.doesNotMatch(detail, /label:"Категории"/);
});

test("nested categories preserve parent hierarchy in breadcrumbs", async () => {
  const source = await read("app/categories/[slug]/page.tsx");
  assert.match(source, /function categoryPath/);
  assert.match(source, /trail\.slice\(0,-1\)\.map/);
  assert.match(source, /href:`\/categories\/\$\{item\.slug\}`/);
});

test("education is represented as a service subcatalog", async () => {
  const source = await read("app/education/page.tsx");
  assert.match(source, /label:\s*"Услуги",\s*href:\s*"\/services"/);
  assert.match(source, /label:\s*"Обучение и наставничество"/);
});

test("private project editor breadcrumbs do not jump through the public project catalog", async () => {
  const source = await read("app/dashboard/projects/[id]/page.tsx");
  assert.match(source, /label: "Кабинет", href: "\/dashboard"/);
  assert.doesNotMatch(source, /label: "Проекты", href: "\/projects"/);
});
