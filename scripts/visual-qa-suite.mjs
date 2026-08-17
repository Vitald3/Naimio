import { BrowserController, sleep } from './browser-controller.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';

const VIEWPORTS = [
  { width: 390, height: 844, name: '390px' },
  { width: 430, height: 932, name: '430px' },
  { width: 768, height: 1024, name: '768px' },
  { width: 1024, height: 768, name: '1024px' },
  { width: 1280, height: 800, name: '1280px' },
  { width: 1440, height: 900, name: '1440px' },
];

const PUBLIC_ROUTES = [
  { name: 'home', path: '/' },
  { name: 'projects_catalog', path: '/projects' },
  { name: 'project_detail', path: '/projects/demo-project-1' },
  { name: 'services_catalog', path: '/services' },
  { name: 'service_detail', path: '/services/demo-service-1' },
  { name: 'freelancers_catalog', path: '/freelancers' },
  { name: 'freelancer_profile', path: '/freelancers/telegram-dev' },
  { name: 'vacancies_catalog', path: '/vacancies' },
  { name: 'categories_catalog', path: '/categories' }
];

async function runVisualQA() {
  const results = {
    viewportsTested: VIEWPORTS.map(v => v.name),
    publicRoutes: [],
    customerFlows: [],
    freelancerFlows: [],
    adminFlows: [],
    messengerFlows: [],
    safeDealsFlows: [],
    skeletonChecks: [],
    issues: []
  };

  mkdirSync('./screenshots/qa', { recursive: true });

  const browser = new BrowserController();
  await browser.init('http://localhost:8088/');

  console.log('--- 1. Testing Public Catalog & Routes Across All Viewports ---');
  for (const route of PUBLIC_ROUTES) {
    console.log(`Checking route: ${route.path} (${route.name})`);
    const routeResult = { route: route.path, name: route.name, viewportResults: {} };

    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(route.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `qa/${route.name}_${vp.name}.png`;
      await browser.captureScreenshot(filename);

      const hasCriticalError = await browser.eval(`(() => {
        const bodyText = document.body.innerText || '';
        return bodyText.includes('Application error') || bodyText.includes('Unhandled Runtime Error') || bodyText.includes('500 Internal Server Error');
      })()`);

      routeResult.viewportResults[vp.name] = {
        hasOverflow: layout.hasOverflow,
        docW: layout.docW,
        winW: layout.winW,
        overflowingElementsCount: layout.overflowingElements?.length || 0,
        hasCriticalError: !!hasCriticalError,
        screenshot: filename
      };

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] ${route.path} @ ${vp.name}: docW=${layout.docW} > winW=${layout.winW}`, layout.overflowingElements);
        results.issues.push({
          type: 'OVERFLOW',
          route: route.path,
          viewport: vp.name,
          details: layout.overflowingElements
        });
      }
      if (hasCriticalError) {
        console.error(`[ERROR] ${route.path} @ ${vp.name} shows application error!`);
        results.issues.push({
          type: 'APP_ERROR',
          route: route.path,
          viewport: vp.name
        });
      }
    }
    results.publicRoutes.push(routeResult);
  }

  console.log('--- 2. Testing Customer Dashboard & Safe Deals ---');
  await browser.loginViaForm('customer@example.test', 'LocalDemo2026!', '/dashboard');
  await sleep(1000);

  const customerPages = [
    { name: 'customer_dashboard', path: '/dashboard' },
    { name: 'customer_deals', path: '/deals' },
    { name: 'customer_messages', path: '/messages' },
    { name: 'customer_new_project', path: '/projects/new' }
  ];

  for (const page of customerPages) {
    console.log(`Checking customer route: ${page.path} (${page.name})`);
    const pageRes = { route: page.path, name: page.name, viewportResults: {} };
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `qa/${page.name}_${vp.name}.png`;
      await browser.captureScreenshot(filename);

      pageRes.viewportResults[vp.name] = {
        hasOverflow: layout.hasOverflow,
        docW: layout.docW,
        winW: layout.winW,
        screenshot: filename
      };

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Customer ${page.path} @ ${vp.name}`, layout.overflowingElements);
        results.issues.push({
          type: 'OVERFLOW',
          user: 'customer',
          route: page.path,
          viewport: vp.name,
          details: layout.overflowingElements
        });
      }
    }
    results.customerFlows.push(pageRes);
  }

  console.log('--- 3. Testing Freelancer Dashboard, Profile & Deals ---');
  // Logout and login as freelancer
  await browser.navigate('/login');
  await browser.eval(`document.cookie.split(";").forEach(c => { document.cookie = c.replace(/^ +/, "").replace(/=.*/, "=;expires=" + new Date().toUTCString() + ";path=/"); });`);
  await browser.loginViaForm('freelancer@example.test', 'LocalDemo2026!', '/dashboard');
  await sleep(1000);

  const freelancerPages = [
    { name: 'freelancer_dashboard', path: '/dashboard' },
    { name: 'freelancer_deals', path: '/deals' },
    { name: 'freelancer_profile_self', path: '/profile' },
    { name: 'freelancer_messages', path: '/messages' }
  ];

  for (const page of freelancerPages) {
    console.log(`Checking freelancer route: ${page.path} (${page.name})`);
    const pageRes = { route: page.path, name: page.name, viewportResults: {} };
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `qa/${page.name}_${vp.name}.png`;
      await browser.captureScreenshot(filename);

      pageRes.viewportResults[vp.name] = {
        hasOverflow: layout.hasOverflow,
        docW: layout.docW,
        winW: layout.winW,
        screenshot: filename
      };

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Freelancer ${page.path} @ ${vp.name}`, layout.overflowingElements);
        results.issues.push({
          type: 'OVERFLOW',
          user: 'freelancer',
          route: page.path,
          viewport: vp.name,
          details: layout.overflowingElements
        });
      }
    }
    results.freelancerFlows.push(pageRes);
  }

  console.log('--- 4. Testing Admin Panel at /x7m4q9k2 ---');
  await browser.navigate('/x7m4q9k2/login');
  await sleep(500);
  // Login to admin
  await browser.eval(`(async () => {
    const emailInput = document.querySelector('input[type="email"], input[name="email"], input[name="login"]');
    const passInput = document.querySelector('input[type="password"], input[name="password"]');
    const form = document.querySelector('form');
    if (emailInput && passInput && form) {
      emailInput.value = 'admin@example.test';
      emailInput.dispatchEvent(new Event('input', { bubbles: true }));
      passInput.value = 'LocalDemo2026!';
      passInput.dispatchEvent(new Event('input', { bubbles: true }));
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    }
  })()`);
  await sleep(1500);

  const adminPages = [
    { name: 'admin_dashboard', path: '/x7m4q9k2' },
    { name: 'admin_users', path: '/x7m4q9k2/users' },
    { name: 'admin_deals', path: '/x7m4q9k2/deals' },
    { name: 'admin_disputes', path: '/x7m4q9k2/disputes' },
    { name: 'admin_providers', path: '/x7m4q9k2/providers' },
    { name: 'admin_audit', path: '/x7m4q9k2/audit' }
  ];

  for (const page of adminPages) {
    console.log(`Checking admin route: ${page.path} (${page.name})`);
    const pageRes = { route: page.path, name: page.name, viewportResults: {} };
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(page.path);
      await sleep(400);

      const layout = await browser.checkLayout();
      const filename = `qa/${page.name}_${vp.name}.png`;
      await browser.captureScreenshot(filename);

      pageRes.viewportResults[vp.name] = {
        hasOverflow: layout.hasOverflow,
        docW: layout.docW,
        winW: layout.winW,
        screenshot: filename
      };

      if (layout.hasOverflow) {
        console.warn(`[OVERFLOW] Admin ${page.path} @ ${vp.name}`, layout.overflowingElements);
        results.issues.push({
          type: 'OVERFLOW',
          user: 'admin',
          route: page.path,
          viewport: vp.name,
          details: layout.overflowingElements
        });
      }
    }
    results.adminFlows.push(pageRes);
  }

  console.log('--- 5. Checking Skeletons and Loading States ---');
  // Check if skeleton classes exist and render properly
  const skeletonTest = await browser.eval(`(() => {
    const skeletonElements = document.querySelectorAll('[class*="skeleton"], [class*="animate-pulse"]');
    return {
      count: skeletonElements.length
    };
  })()`);
  results.skeletonChecks.push(skeletonTest);

  await browser.close();

  writeFileSync('./screenshots/qa_results.json', JSON.stringify(results, null, 2));
  console.log('\n--- Visual QA Pass Completed! Results saved to ./screenshots/qa_results.json ---');
  console.log(`Total issues detected: ${results.issues.length}`);
}

runVisualQA().catch(e => {
  console.error(e);
  process.exit(1);
});
