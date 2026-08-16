import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
const read=(p)=>fs.readFile(new URL(`../${p}`,import.meta.url),'utf8');

test('notification open marks read and proposal notification targets proposals',async()=>{const s=await read('app/notifications/page.tsx');assert.match(s,/proposal_received/);assert.match(s,/dashboard\/projects\/\$\{item\.payload\?\.project_id\|\|item\.entity_id\}\/proposals/);assert.match(s,/openNotification/);assert.match(s,/notification-read/)});
test('notification badge consumes read events without polling',async()=>{const s=await read('app/notification-badge.tsx');assert.match(s,/notification-read/);assert.doesNotMatch(s,/setInterval/)});
test('proposal selection displays authoritative customer total with commissions',async()=>{const s=await read('app/dashboard/projects/[id]/proposals/page.tsx');assert.match(s,/safe-deals\/quote/);assert.match(s,/К оплате с комиссиями/);assert.match(s,/quotes\[proposal\.price_kopecks\]/);assert.match(s,/Комиссия платформы/);assert.match(s,/Комиссия провайдера/)});
test('staff support inbox is realtime over admin websocket',async()=>{
    const s=await read('app/x7m4q9k2/support/page.tsx');

    assert.match(s,/\/api\/v1\/admin\/support\/conversations/);
    assert.match(s,/\/api\/v1\/admin\/support\/ws/);
    assert.match(s,/message\.created/);
    assert.match(s,/support\.conversation\.updated/);
    assert.match(s,/new WebSocket/);
    assert.doesNotMatch(
        s,
        /setInterval\s*\(\s*(?:async\s*)?\(\)\s*=>\s*\{?[^}]*loadConversations/
    );
    assert.doesNotMatch(
        s,
        /setInterval\s*\(\s*(?:async\s*)?\(\)\s*=>\s*\{?[^}]*loadMessages/
    );
    assert.match(s,/Ответ поддержки/);
});
test('calendar period menus highlight only the selected label',async()=>{const s=await read('app/pretty-date-input.tsx');const css=await read('app/globals.css');assert.match(s,/date-picker-v2__period-menu--months/);assert.match(s,/date-picker-v2__period-menu--years/);assert.match(s,/className=\{index===month\.getMonth\(\)\?"is-selected":""\}/);assert.match(css,/period-menu button\.is-selected span\{background:var\(--brand-soft\)/);assert.match(css,/period-menu button:hover\{background:transparent!important/)});
test('message delivery persists notification intent atomically and pushes it over websocket',async()=>{const pg=await fs.readFile(new URL('../../api/internal/communication/postgres.go',import.meta.url),'utf8');const svc=await fs.readFile(new URL('../../api/internal/communication/communication.go',import.meta.url),'utf8');assert.match(pg,/INSERT INTO notifications/);assert.match(pg,/message_id/);assert.match(pg,/tx\.Commit\(\)/);assert.match(svc,/MessageNotificationEvents/);assert.match(svc,/notification\.created/)});
test('admin support websocket is separately routed through nginx and admin session',async()=>{const page=await read('app/x7m4q9k2/support/page.tsx');const main=await fs.readFile(new URL('../../api/cmd/api/main.go',import.meta.url),'utf8');const nginx=await fs.readFile(new URL('../../../infra/nginx/nginx.conf',import.meta.url),'utf8');assert.match(page,/api\/v1\/admin\/support\/ws/);assert.match(main,/ServeSupportHTTP/);assert.match(nginx,/location = \/api\/v1\/admin\/support\/ws/);assert.match(nginx,/proxy_set_header Upgrade \$http_upgrade/)});
test('legacy untouched safe deal fee default charges customer platform commission',async()=>{const migration=await fs.readFile(new URL('../../../db/migrations/000049_safe_deal_legacy_customer_fee_default.sql',import.meta.url),'utf8');assert.match(migration,/version=1/);assert.match(migration,/platform_fee_payer_mode='CUSTOMER'/);assert.match(migration,/platform_fee_payer_mode='FREELANCER'/)});

test('freelancer proposal preview shows platform and provider commissions',async()=>{const s=await read('app/project/[id]/proposal-form.tsx');assert.match(s,/Расчёт выплаты и комиссий/);assert.match(s,/Комиссия сервиса/);assert.match(s,/Комиссия платёжного провайдера/);assert.match(s,/platform_fee\.total_kopecks/);assert.match(s,/provider_fee\.total_kopecks/)});
test('accepted proposal exposes Safe Deal payment action',async()=>{const s=await read('app/dashboard/projects/[id]/proposals/page.tsx');assert.match(s,/safe_deal_id/);assert.match(s,/Перейти в Безопасную сделку/);assert.match(s,/Исполнитель выбран\. Безопасная сделка создана и ожидает оплаты/)});
test('awaiting funding project status is human readable',async()=>{const s=await read('app/dashboard/projects/[id]/page.tsx');assert.match(s,/AWAITING_FUNDING:\s*"Ожидает оплаты по Безопасной сделке"/)});
test('Safe Deal action failures use top-right toast instead of inline action error',async()=>{const s=await read('app/dashboard/safe-deals/[id]/page.tsx');assert.match(s,/useToast/);assert.match(s,/PAYMENT_PROVIDER_UNAVAILABLE/);assert.match(s,/push\(\{kind:"error"/);assert.doesNotMatch(s,/<p role="status" className="notice">\{state\}<\/p><\/aside>/)});
