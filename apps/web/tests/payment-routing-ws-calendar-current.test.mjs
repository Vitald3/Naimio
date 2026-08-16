import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';

const read = (path) => fs.readFile(new URL(`../${path}`, import.meta.url), 'utf8');

test('payment routes serialize lowercase fields and PRO admin uses live routing', async () => {
  const routing = await fs.readFile(new URL('../../api/internal/payments/routing.go', import.meta.url), 'utf8');
  const pro = await read('app/x7m4q9k2/monetization/page.tsx');
  assert.match(routing, /`json:"domain"`/);
  assert.match(routing, /`json:"provider"`/);
  assert.match(pro, /routes\/PRO_SUBSCRIPTION/);
  assert.match(pro, /Провайдер PRO-платежей/);
  assert.doesNotMatch(pro, /<strong>Не подключён<\/strong>/);
});

test('access logger preserves websocket hijacking', async () => {
  const main = await fs.readFile(new URL('../../api/cmd/api/main.go', import.meta.url), 'utf8');
  assert.match(main, /func \(w \*statusRecorder\) Hijack\(\)/);
  assert.match(main, /w\.ResponseWriter\.\(http\.Hijacker\)/);
});

test('safe deal provider pricing follows active SAFE_DEAL route', async () => {
  const pg = await fs.readFile(new URL('../../api/internal/safedeal/postgres.go', import.meta.url), 'utf8');
  const finance = await read('app/x7m4q9k2/finance/fees/page.tsx');
  assert.match(pg, /payment_provider_routes WHERE domain='SAFE_DEAL'/);
  assert.match(pg, /provider=\$1 AND payment_method=\$2/);
  assert.match(finance, /тариф PSP, выбранного текущим маршрутом SAFE_DEAL/);
});

test('calendar header uses compact month and year menus', async () => {
  const picker = await read('app/pretty-date-input.tsx');
  const css = await read('app/globals.css');
  assert.match(picker, /date-picker-v2__period-trigger/);
  assert.match(picker, /date-picker-v2__period-menu--months/);
  assert.match(picker, /date-picker-v2__period-menu--years/);
  assert.match(css, /period-menu button\.is-selected span/);
});
