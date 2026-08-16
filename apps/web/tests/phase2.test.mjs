import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("proposal UI sends bounded canonical fields", async () => {
  const source = await readFile(new URL("../app/project/[id]/proposal-form.tsx", import.meta.url), "utf8");
  assert.match(source, /maxLength=\{5000\}/);
  assert.match(source, /price_kopecks/);
  assert.match(source, /currency:\s*"RUB"/);
  assert.doesNotMatch(source, /user_id/);
});

test("favorites UI exposes only Phase 2 entity tabs", async () => {
  const source = await readFile(new URL("../app/favorites/page.tsx", import.meta.url), "utf8");
  assert.match(source, /FREELANCER.*SERVICE.*PROJECT/);
  assert.doesNotMatch(source, /VACANCY/);
});

test("external reputation UI keeps public verified data separate and cannot forge state", async () => {
  const settings = await readFile(new URL("../app/settings/reputation/page.tsx", import.meta.url), "utf8");
  const profile = await readFile(new URL("../app/freelancers/[username]/page.tsx", import.meta.url), "utf8");
  assert.match(settings, /Внешние оценки показываются отдельно/);
  assert.match(settings, /PROFILE_CODE/);
  assert.doesNotMatch(settings, /verification_status:\s*"VERIFIED"/);
  assert.doesNotMatch(settings, /source_snapshot|evidence.*input/i);
  assert.match(profile, /Подтверждённая внешняя репутация/);
  assert.doesNotMatch(profile, /source_snapshot|evidence/);
});
test("native reviews UI is project-bound and separate from external reputation",async()=>{const form=await readFile(new URL("../app/review-form.tsx",import.meta.url),"utf8");const profile=await readFile(new URL("../app/freelancers/[username]/page.tsx",import.meta.url),"utf8");assert.match(form,/projects\/\$\{encodeURIComponent\(projectId\)\}\/reviews/);assert.match(form,/maxLength=\{5000\}/);assert.doesNotMatch(form,/reviewee_user_id|reviewer_user_id/);assert.match(profile,/Рейтинг на платформе/);assert.match(profile,/Подтверждённая внешняя репутация/);assert.match(profile,/Новый исполнитель/)});
