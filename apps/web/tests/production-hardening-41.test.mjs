import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const readAPI = (path) => readFile(new URL(`../../api/${path}`, import.meta.url), "utf8");

test("admin reasons use rich editor and safe rich rendering", async () => {
  const ui = await read("app/x7m4q9k2/admin-ui.tsx");
  assert.match(ui, /AdminReasonEditor autoFocus/);
  assert.match(ui, /FormattedText value=\{content\}/);
  for (const path of [
    "app/x7m4q9k2/matching/page.tsx",
    "app/x7m4q9k2/finance/fees/page.tsx",
    "app/x7m4q9k2/content/page.tsx",
  ]) assert.match(await read(path), /AdminReasonEditor/, `${path} must use the rich reason editor`);
  const monetization = await read("app/x7m4q9k2/monetization/page.tsx");
  assert.doesNotMatch(monetization, /AdminReasonEditor/, "PRO plan/grant audit reasons are intentionally plain fields");
});

test("admin operational entity columns link to their targets", async () => {
  assert.match(await read("app/x7m4q9k2/reports/page.tsx"), /AdminEntityLink/);
  assert.match(await read("app/x7m4q9k2/fraud/page.tsx"), /AdminEntityLink/);
  assert.match(await read("app/x7m4q9k2/audit/page.tsx"), /AdminEntityLink/);
  const content = await read("app/x7m4q9k2/content-admin.tsx");
  assert.match(content, /entityType="USER"/);
  assert.match(content, /moderation/);
});

test("safe deal project opens separately and only unfunded deal exposes delete", async () => {
  const source = await read("app/x7m4q9k2/safe-deals/page.tsx");
  assert.match(source, /target="_blank"/);
  assert.match(source, /d\.status==="AWAITING_FUNDING"&&!d\.payment/);
  assert.match(source, /\/delete/);
});

test("presence is batched instead of one network request per avatar", async () => {
  const media = await read("app/media-components.tsx");
  const main = await readAPI("cmd/api/main.go");
  const presence = await readAPI("internal/auth/presence.go");
  assert.match(media, /\/api\/v1\/presence\/batch/);
  assert.match(media, /presencePending/);
  assert.match(main, /\/api\/v1\/presence\/batch/);
  assert.match(presence, /func \(h PresenceHandler\) Batch/);
  assert.match(presence, /len\(input\.IDs\) > 100/);
});

test("local media survives container rebuilds and production retains object storage", async () => {
  const compose = await readFile(new URL("../../../docker-compose.yml", import.meta.url), "utf8");
  const main = await readAPI("cmd/api/main.go");
  assert.match(compose, /naimio_media:\/var\/lib\/naimio-media/);
  assert.match(compose, /naimio_media:/);
  assert.match(main, /OBJECT_STORAGE_ENDPOINT/);
  assert.match(main, /DEV_MEDIA_ROOT/);
});

test("chat supports quote scrolling and browser voice recording", async () => {
  const source = await read("app/messages/page.tsx");
  assert.match(source, /MediaRecorder/);
  assert.match(source, /getUserMedia/);
  assert.match(source, /scrollIntoView/);
  assert.match(source, /AUDIO/);
});

test("admin provider setup makes configuration visible and exposes sandbox production mode", async () => {
  const source = await read("app/x7m4q9k2/payment-routing/page.tsx");
  assert.match(source, /Настроить/);
  assert.match(source, /Sandbox — тестовые платежи/);
  assert.match(source, /Production — реальные платежи/);
  assert.match(source, /scrollIntoView/);
  assert.match(source, /\/config/);
});

test("legal documents are selected by admin and surfaced in footer", async () => {
  const settings = await read("app/x7m4q9k2/settings/page.tsx");
  const footer = await read("app/site-footer.tsx");
  assert.match(settings, /privacy_policy_slug/);
  assert.match(settings, /terms_slug/);
  assert.match(footer, /Политика конфиденциальности/);
  assert.match(footer, /Условия соглашения/);
});

test("logout replaces history entry and registration captures gender", async () => {
  assert.match(await read("app/auth-state.tsx"), /window\.location\.replace\("\/"\)/);
  const register = await read("app/register/page.tsx");
  assert.match(register, /gender/);
  assert.match(register, /Мужской/);
  assert.match(register, /Женский/);
});

test("project proposal counts use Russian declension", async () => {
  for (const path of ["app/projects/page.tsx", "app/projects/[id]/page.tsx"]) {
    const source = await read(path);
    assert.match(source, /proposalLabel/);
    assert.match(source, /отклик/);
    assert.match(source, /отклика/);
    assert.match(source, /откликов/);
  }
});

test("portfolio grid removes wrapper border and empty states have a semantic icon", async () => {
  const css = await read("app/globals.css");
  assert.match(css, /\.portfolio-media-grid>li\{[^}]*border:0!important/);
  assert.match(css, /\.empty:before\s*\{[^}]*data:image\/svg\+xml/);
});


test("admin reason editor has the full project-grade toolbar", async () => {
  const editor = await read("app/x7m4q9k2/admin-reason-editor.tsx");
  for (const control of ["Обычный текст", "Заголовок второго уровня", "Заголовок третьего уровня", "Жирный", "Курсив", "Маркированный список", "Нумерованный список", "Цитата", "Добавить или изменить ссылку", "Отменить последнее действие", "Вернуть отменённое действие", "Очистить форматирование", "Предпросмотр"]) {
    assert.match(editor, new RegExp(control));
  }
  assert.match(editor, /FormattedText/);
  assert.match(editor, /project-rich-editor__toolbar/);
});

test("hardening routes stay inside admin and provider audit accepts non-UUID provider names", async () => {
  const ui = await read("app/x7m4q9k2/admin-ui.tsx");
  const provider = await readAPI("internal/payments/provider_config.go");
  assert.match(ui, /case "PROJECT":return `\/x7m4q9k2\/projects\/\$\{/);
  assert.match(ui, /case "SERVICE":return `\/x7m4q9k2\/services\/\$\{/);
  assert.match(ui, /case "VACANCY":return `\/x7m4q9k2\/vacancies\/\$\{/);
  assert.match(ui, /case "USER":return `\/x7m4q9k2\/users\/\$\{/);
  assert.match(provider, /'PAYMENT_PROVIDER',NULL,jsonb_build_object\('provider'/);
});

test("nginx config is image-built so websocket changes are applied on make dev", async () => {
  const compose = await readFile(new URL("../../../docker-compose.yml", import.meta.url), "utf8");
  const dockerfile = await readFile(new URL("../../../infra/nginx/Dockerfile", import.meta.url), "utf8");
  const devNginx = await readFile(new URL("../../../infra/nginx/nginx.conf", import.meta.url), "utf8");
  const prodNginx = await readFile(new URL("../../../infra/nginx/nginx.production.conf", import.meta.url), "utf8");
  assert.match(compose, /nginx:\s*\n\s*build: \.\/infra\/nginx/);
  assert.match(dockerfile, /COPY nginx\.conf/);
  assert.match(dockerfile, /COPY nginx\.production\.conf/);
  for (const source of [devNginx, prodNginx]) {
    assert.match(source, /location = \/api\/v1\/ws/);
    assert.match(source, /proxy_set_header Upgrade \$http_upgrade/);
    assert.match(source, /microphone=\(self\)/);
  }
});

test("freelancer registration requires professional availability context", async () => {
  const register = await read("app/register/page.tsx");
  const auth = await readAPI("internal/auth/http.go");
  assert.match(register, /Опыт, лет<input required type="number"/);
  assert.match(register, /Ставка, ₽\/час<input required type="number"/);
  assert.match(register, /availability/);
  assert.match(auth, /in\.ExperienceYears == nil \|\| in\.HourlyRateKopecks == nil/);
});
