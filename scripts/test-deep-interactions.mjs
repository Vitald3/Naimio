import { writeFileSync } from 'node:fs';
import { BrowserController, sleep } from './browser-controller.mjs';

async function run() {
  const browser = new BrowserController();
  await browser.init();

  console.log('=== TESTING DEEP COMPONENT INTERACTIONS ===\n');

  // 1. CUSTOM SELECT / COMBOBOX IN CATALOG FILTERS
  console.log('1. Testing CustomSelect in Catalog Filters...');
  await browser.setViewport(1440, 900);
  await browser.navigate('/projects');
  await sleep(600);

  // Click on a filter select
  const selectOpened = await browser.eval(`(() => {
    const trigger = document.querySelector('.custom-select, select, [role="combobox"], .custom-select__trigger');
    if (trigger) {
      trigger.click();
      return true;
    }
    return false;
  })()`);
  await sleep(300);
  await browser.captureScreenshot('deep_custom_select_desktop.png');

  // Test custom select on 390px
  await browser.setViewport(390, 844);
  await browser.navigate('/projects');
  await sleep(600);
  await browser.captureScreenshot('deep_custom_select_mobile.png');

  // 2. MODALS / DIALOGS
  console.log('2. Testing Modals...');
  await browser.setViewport(1440, 900);
  await browser.navigate('/x7m4q9k2/login');
  await sleep(400);
  await browser.eval(`(async () => {
    await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: "admin@example.test", password: "LocalDemo2026!", portal: "admin" }),
    });
  })()`);
  await browser.navigate('/x7m4q9k2/disputes');
  await sleep(600);
  await browser.captureScreenshot('deep_admin_disputes_1440px.png');

  await browser.setViewport(390, 844);
  await browser.navigate('/x7m4q9k2/disputes');
  await sleep(600);
  await browser.captureScreenshot('deep_admin_disputes_390px.png');

  // 3. SAFE DEAL
  console.log('3. Testing Safe Deal Lifecycle...');
  await browser.setViewport(1440, 900);
  await browser.navigate('/x7m4q9k2/safe-deals');
  await sleep(600);
  await browser.captureScreenshot('deep_safe_deals_admin_1440px.png');

  await browser.setViewport(390, 844);
  await browser.navigate('/x7m4q9k2/safe-deals');
  await sleep(600);
  await browser.captureScreenshot('deep_safe_deals_admin_390px.png');

  // 4. MESSENGER
  console.log('4. Testing Messenger...');
  await browser.setViewport(1440, 900);
  await browser.navigate('/login');
  await sleep(400);
  await browser.eval(`(async () => {
    await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: "freelancer@example.test", password: "LocalDemo2026!" }),
    });
  })()`);
  await browser.navigate('/messages');
  await sleep(600);
  await browser.captureScreenshot('deep_messenger_1440px.png');

  await browser.setViewport(390, 844);
  await browser.navigate('/messages');
  await sleep(600);
  await browser.captureScreenshot('deep_messenger_390px.png');

  // 5. SKELETONS UNDER NETWORK DELAY
  console.log('5. Testing Skeletons & Shimmers...');
  // Emulate network latency
  await browser.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 2000,
    downloadThroughput: 50 * 1024,
    uploadThroughput: 20 * 1024
  });

  await browser.navigate('/projects');
  await sleep(500); // capture while loading
  await browser.captureScreenshot('deep_projects_skeleton_loading.png');

  await browser.navigate('/freelancers');
  await sleep(500);
  await browser.captureScreenshot('deep_freelancers_skeleton_loading.png');

  // Reset network conditions
  await browser.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1
  });

  console.log('=== DEEP INTERACTION VERIFICATION COMPLETE ===');
  await browser.close();
}

run().catch(console.error);
