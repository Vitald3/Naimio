import { writeFileSync } from 'node:fs';
import { BrowserController, sleep } from './browser-controller.mjs';

const VIEWPORTS = [390, 430, 768, 1024, 1280, 1440];

const ADMIN_PAGES = [
  { name: 'admin_dashboard', path: '/x7m4q9k2' },
  { name: 'admin_users', path: '/x7m4q9k2/users' },
  { name: 'admin_projects', path: '/x7m4q9k2/projects' },
  { name: 'admin_services', path: '/x7m4q9k2/services' },
  { name: 'admin_vacancies', path: '/x7m4q9k2/vacancies' },
  { name: 'admin_safe_deals', path: '/x7m4q9k2/safe-deals' },
  { name: 'admin_disputes', path: '/x7m4q9k2/disputes' },
  { name: 'admin_payments', path: '/x7m4q9k2/payments' },
  { name: 'admin_payment_routing', path: '/x7m4q9k2/payment-routing' },
  { name: 'admin_reputation', path: '/x7m4q9k2/reputation' },
  { name: 'admin_reviews', path: '/x7m4q9k2/reviews' },
  { name: 'admin_reports', path: '/x7m4q9k2/reports' },
  { name: 'admin_monetization', path: '/x7m4q9k2/monetization' },
  { name: 'admin_taxonomy', path: '/x7m4q9k2/taxonomy' },
  { name: 'admin_referral_rules', path: '/x7m4q9k2/referral-rules' },
  { name: 'admin_matching', path: '/x7m4q9k2/matching' },
  { name: 'admin_fraud', path: '/x7m4q9k2/fraud' },
  { name: 'admin_feature_flags', path: '/x7m4q9k2/feature-flags' },
  { name: 'admin_support', path: '/x7m4q9k2/support' },
  { name: 'admin_audit', path: '/x7m4q9k2/audit' },
  { name: 'admin_settings', path: '/x7m4q9k2/settings' },
  { name: 'admin_finance', path: '/x7m4q9k2/finance' },
  { name: 'admin_calculators', path: '/x7m4q9k2/calculators' },
  { name: 'admin_content', path: '/x7m4q9k2/content' },
];

async function run() {
  const browser = new BrowserController();
  await browser.init();

  console.log('=== LOGGING INTO STAFF CONTROL CENTER (/x7m4q9k2/login) ===');
  await browser.navigate('/x7m4q9k2/login');
  await sleep(500);

  const loginRes = await browser.eval(`(async () => {
    const res = await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: "admin@example.test", password: "LocalDemo2026!", portal: "admin" }),
    });
    return { ok: res.ok, status: res.status };
  })()`);

  console.log('Staff login response:', loginRes);

  const results = {
    testedViewports: VIEWPORTS,
    overflows: [],
    details: []
  };

  for (const page of ADMIN_PAGES) {
    console.log(`Auditing Admin Route: ${page.name} (${page.path})`);
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp, 900);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `authenticated_${page.name}_${vp}px.png`;
      
      if (vp === 390 || vp === 1440 || vp === 768) {
        await browser.captureScreenshot(filename);
      }

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] ${page.name} at ${vp}px: docW=${layout.docW}, winW=${layout.winW}`);
        results.overflows.push({ page: page.name, path: page.path, viewport: vp, layout });
      }
    }
  }

  console.log('\n=== STAFF VISUAL QA COMPLETE ===');
  console.log(`Total Overflows in Staff Control Center: ${results.overflows.length}`);
  writeFileSync('./screenshots/admin_qa_results.json', JSON.stringify(results, null, 2));

  await browser.close();
}

run().catch(console.error);
