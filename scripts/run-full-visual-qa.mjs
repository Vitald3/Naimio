import { writeFileSync } from 'node:fs';
import { BrowserController, sleep } from './browser-controller.mjs';

const VIEWPORTS = [390, 430, 768, 1024, 1280, 1440];

const PUBLIC_PAGES = [
  { name: 'home', path: '/' },
  { name: 'projects_catalog', path: '/projects' },
  { name: 'project_detail', path: '/project/9664eaa8-9632-5973-aa23-cc6991cef751' },
  { name: 'freelancers_catalog', path: '/freelancers' },
  { name: 'freelancer_profile', path: '/freelancers/freelancer' },
  { name: 'services_catalog', path: '/services' },
  { name: 'vacancies_catalog', path: '/vacancies' },
  { name: 'education_catalog', path: '/education' },
  { name: 'categories_catalog', path: '/categories' },
  { name: 'blog_catalog', path: '/blog' },
  { name: 'pro_page', path: '/pro' },
];

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

const CUSTOMER_PAGES = [
  { name: 'customer_dashboard', path: '/dashboard' },
  { name: 'customer_create_project', path: '/create-project' },
  { name: 'customer_messages', path: '/messages' },
  { name: 'customer_notifications', path: '/notifications' },
  { name: 'customer_favorites', path: '/favorites' },
  { name: 'customer_settings', path: '/settings' },
  { name: 'customer_proposals', path: '/project/9664eaa8-9632-5973-aa23-cc6991cef751' },
];

const FREELANCER_PAGES = [
  { name: 'freelancer_dashboard', path: '/dashboard' },
  { name: 'freelancer_profile', path: '/profile' },
  { name: 'freelancer_messages', path: '/messages' },
  { name: 'freelancer_notifications', path: '/notifications' },
  { name: 'freelancer_settings', path: '/settings' },
];

async function run() {
  const browser = new BrowserController();
  await browser.init();

  const results = {
    testedViewports: VIEWPORTS,
    pages: [],
    overflows: [],
    errors: [],
  };

  console.log('=== STARTING VISUAL QA ACROSS ALL VIEWPORTS ===\n');

  // 1. PUBLIC PAGES
  console.log('--- TESTING PUBLIC PAGES ---');
  for (const page of PUBLIC_PAGES) {
    console.log(`Checking ${page.name} (${page.path})...`);
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp, 900);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `public_${page.name}_${vp}px.png`;
      
      // Save screenshot for mandatory evidence
      if (vp === 390 || vp === 1440 || vp === 768) {
        await browser.captureScreenshot(filename);
      }

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] ${page.name} at ${vp}px: docW=${layout.docW}, winW=${layout.winW}`);
        results.overflows.push({ page: page.name, path: page.path, viewport: vp, layout });
      }
    }
  }

  // 2. CUSTOMER SESSION
  console.log('\n--- TESTING CUSTOMER SESSION ---');
  await browser.setViewport(1440, 900);
  await browser.loginViaForm('customer@example.test', 'LocalDemo2026!', '/dashboard');
  for (const page of CUSTOMER_PAGES) {
    console.log(`Checking Customer ${page.name} (${page.path})...`);
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp, 900);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `customer_${page.name}_${vp}px.png`;
      if (vp === 390 || vp === 1440 || vp === 768) {
        await browser.captureScreenshot(filename);
      }

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Customer ${page.name} at ${vp}px: docW=${layout.docW}, winW=${layout.winW}`);
        results.overflows.push({ role: 'customer', page: page.name, path: page.path, viewport: vp, layout });
      }
    }
  }

  // 3. FREELANCER SESSION
  console.log('\n--- TESTING FREELANCER SESSION ---');
  await browser.setViewport(1440, 900);
  await browser.loginViaForm('freelancer@example.test', 'LocalDemo2026!', '/dashboard');
  for (const page of FREELANCER_PAGES) {
    console.log(`Checking Freelancer ${page.name} (${page.path})...`);
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp, 900);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `freelancer_${page.name}_${vp}px.png`;
      if (vp === 390 || vp === 1440 || vp === 768) {
        await browser.captureScreenshot(filename);
      }

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Freelancer ${page.name} at ${vp}px: docW=${layout.docW}, winW=${layout.winW}`);
        results.overflows.push({ role: 'freelancer', page: page.name, path: page.path, viewport: vp, layout });
      }
    }
  }

  // 4. ADMIN SESSION
  console.log('\n--- TESTING ADMIN SESSION ---');
  await browser.setViewport(1440, 900);
  await browser.navigate('/x7m4q9k2/login');
  await sleep(400);
  await browser.eval(`(async () => {
    const res = await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: "admin@example.test", password: "LocalDemo2026!", portal: "admin" }),
    });
    return res.ok;
  })()`);
  for (const page of ADMIN_PAGES) {
    console.log(`Checking Admin ${page.name} (${page.path})...`);
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp, 900);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `admin_${page.name}_${vp}px.png`;
      if (vp === 390 || vp === 1440 || vp === 768) {
        await browser.captureScreenshot(filename);
      }

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Admin ${page.name} at ${vp}px: docW=${layout.docW}, winW=${layout.winW}`);
        results.overflows.push({ role: 'admin', page: page.name, path: page.path, viewport: vp, layout });
      }
    }
  }

  console.log('\n=== VISUAL QA RUN COMPLETE ===');
  console.log(`Total Overflows Detected: ${results.overflows.length}`);
  writeFileSync('./screenshots/qa_results.json', JSON.stringify(results, null, 2));

  await browser.close();
}

run().catch(console.error);
