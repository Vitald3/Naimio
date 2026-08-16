import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("completed Safe Deal hides duplicate review form after an existing project review", async () => {
  const source = await read("app/dashboard/safe-deals/[id]/page.tsx");
  assert.match(source, /\/api\/v1\/me\/reviews\/given/);
  assert.match(source, /alreadyReviewed/);
  assert.match(source, /Отзыв уже опубликован/);
  assert.match(source, /reviewState==="available"/);
});

test("My Team adds people through freelancer autocomplete instead of raw user IDs", async () => {
  const source = await read("app/dashboard/team/page.tsx");
  assert.match(source, /\/api\/v1\/freelancers\?q=/);
  assert.match(source, /role="combobox"/);
  assert.match(source, /role="option"/);
  assert.doesNotMatch(source, />ID исполнителя</);
});

test("project creation generates its URL internally and does not ask for an address or slug", async () => {
  const source = await read("app/dashboard/projects/new/page.tsx");
  assert.match(source, /generatedSlug/);
  assert.match(source, /Адрес страницы Naimio\s+сформирует автоматически/);
  assert.doesNotMatch(source, /Адрес страницы<input/);
  assert.doesNotMatch(source, /pattern=/);
});

test("project creation includes an editable description editor and TЗ attachments", async () => {
  const source = await read("app/dashboard/projects/new/page.tsx");
  const editor = await read("app/project-description-editor.tsx");
  assert.match(source, /ProjectDescriptionEditor/);
  assert.match(source, /type="file"/);
  assert.match(source, /\.pdf,\.doc,\.docx,\.txt/);
  assert.match(source, /media_ids: attachments\.map/);
  assert.match(source, /\/api\/v1\/uploads\/presign/);
  assert.match(editor, /Шаблон ТЗ/);
  assert.match(editor, /Предпросмотр/);
});

test("project refresh icon is exported for catalog auto-refresh UI", async () => {
  const icons = await read("app/icons.tsx");
  const projects = await read("app/projects/page.tsx");
  assert.match(icons, /export const IconRefresh/);
  assert.match(projects, /IconRefresh/);
});

test("seed FIXED economics demo project satisfies project budget constraint", async () => {
  const seed = await readFile(new URL("../../api/cmd/dev-seed/seed.sql", import.meta.url), "utf8");
  assert.match(seed, /'FIXED',w,NULL/);
  assert.doesNotMatch(seed, /'FIXED',w,w/);
});
