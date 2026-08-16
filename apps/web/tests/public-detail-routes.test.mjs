import test from "node:test";
import assert from "node:assert/strict";
import { readFile, access } from "node:fs/promises";

const routeFiles = [
  "../app/projects/[id]/page.tsx",
  "../app/services/[id]/page.tsx",
  "../app/vacancies/[id]/page.tsx",
  "../app/freelancers/[username]/page.tsx",
];

test("canonical marketplace detail routes exist", async () => {
  for (const file of routeFiles) await access(new URL(file, import.meta.url));
});

test("catalogs and homepage link to canonical plural detail routes", async () => {
  const files = ["../app/page.tsx","../app/projects/page.tsx","../app/services/page.tsx","../app/vacancies/page.tsx","../app/freelancers/page.tsx","../app/education/page.tsx"];
  const source = (await Promise.all(files.map((file) => readFile(new URL(file, import.meta.url), "utf8")))).join("\n");
  assert.match(source, /`\/projects\/\$\{/);
  assert.match(source, /`\/services\/\$\{/);
  assert.match(source, /`\/vacancies\/\$\{/);
  assert.match(source, /`\/freelancers\/\$\{/);
  assert.doesNotMatch(source, /`\/(?:project|service|vacancy|profile)\/\$\{/);
});

test("legacy singular detail URLs permanently redirect", async () => {
  for (const [file,target] of [
    ["../app/project/[id]/page.tsx", "/projects/"],
    ["../app/service/[id]/page.tsx", "/services/"],
    ["../app/vacancy/[id]/page.tsx", "/vacancies/"],
    ["../app/profile/[username]/page.tsx", "/freelancers/"],
  ]) {
    const source=await readFile(new URL(file, import.meta.url),"utf8");
    assert.match(source,/permanentRedirect/);
    assert.ok(source.includes(target));
  }
});

test("canonical project route never redirects a private project back to the dashboard", async () => {
  const source = await readFile(new URL("../app/projects/[id]/page.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(source, /ownerCanOpenProject/);
  assert.doesNotMatch(source, /redirect\(`\/dashboard\/projects\/\$\{/);
  assert.match(source, /if \(!project\) notFound\(\)/);
});

test("new projects default to public catalogue visibility and private published projects can be made public", async () => {
  const createSource = await readFile(new URL("../app/dashboard/projects/new/page.tsx", import.meta.url), "utf8");
  const detailSource = await readFile(new URL("../app/dashboard/projects/[id]/page.tsx", import.meta.url), "utf8");
  assert.match(createSource, /useState\("PUBLIC"\)/);
  assert.match(createSource, /Публичный — показывать в каталоге/);
  assert.match(detailSource, /make-public/);
  assert.match(detailSource, /Опубликовать в каталоге/);
  assert.match(detailSource, /item\.visibility === "PUBLIC"/);
});

test("payout setup explains missing provider configuration without exposing raw backend text", async () => {
  const source = await readFile(new URL("../app/dashboard/payouts/page.tsx", import.meta.url), "utf8");
  assert.match(source, /PAYOUT_UNAVAILABLE/);
  assert.match(source, /Платёжный провайдер для выплат ещё не настроен/);
  assert.doesNotMatch(source, /payout setup is temporarily unavailable/);
});
