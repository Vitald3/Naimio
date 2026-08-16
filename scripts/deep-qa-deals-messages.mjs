import { BrowserController, sleep } from './browser-controller.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';

const VIEWPORTS = [
  { width: 390, height: 844, name: '390px' },
  { width: 768, height: 1024, name: '768px' },
  { width: 1440, height: 900, name: '1440px' }
];

async function deepQA() {
  mkdirSync('./screenshots/deep_qa', { recursive: true });
  const browser = new BrowserController();
  await browser.init('http://localhost:8088/');

  console.log('--- Deep QA 1: Safe Deals in Various States ---');
  // Login as customer
  await browser.loginViaForm('customer@example.test', 'LocalDemo2026!', '/deals');
  await sleep(1000);

  // Check deals list
  for (const vp of VIEWPORTS) {
    await browser.setViewport(vp.width, vp.height);
    await browser.navigate('/deals');
    await sleep(500);
    const layout = await browser.checkLayout();
    await browser.captureScreenshot(`deep_qa/deals_list_${vp.name}.png`);
    console.log(`Deals list @ ${vp.name}: overflow=${layout.hasOverflow}, width=${layout.docW}/${layout.winW}`);
  }

  // Check individual deal states by clicking / navigating into project 15, 17, 19
  const dealProjects = ['demo-project-15', 'demo-project-17', 'demo-project-19'];
  for (const slug of dealProjects) {
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(`/projects/${slug}`);
      await sleep(500);
      const layout = await browser.checkLayout();
      await browser.captureScreenshot(`deep_qa/deal_project_${slug}_${vp.name}.png`);
      console.log(`Project Deal ${slug} @ ${vp.name}: overflow=${layout.hasOverflow}`);
    }
  }

  console.log('--- Deep QA 2: Messenger Active Conversation ---');
  for (const vp of VIEWPORTS) {
    await browser.setViewport(vp.width, vp.height);
    await browser.navigate('/messages');
    await sleep(800);
    // Click on the first conversation if available
    await browser.eval(`(() => {
      const conv = document.querySelector('[data-conversation-id], [class*="conversation-item"], [class*="chat-item"], a[href*="/messages/"], button[class*="chat"]');
      if (conv) conv.click();
    })()`);
    await sleep(500);
    const layout = await browser.checkLayout();
    await browser.captureScreenshot(`deep_qa/messenger_active_${vp.name}.png`);
    console.log(`Messenger @ ${vp.name}: overflow=${layout.hasOverflow}`);
  }

  console.log('--- Deep QA 3: Admin Panel Detail Views ---');
  await browser.navigate('/x7m4q9k2/login');
  await sleep(500);
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

  const adminSubPages = ['/x7m4q9k2/deals', '/x7m4q9k2/disputes', '/x7m4q9k2/providers', '/x7m4q9k2/audit'];
  for (const sub of adminSubPages) {
    for (const vp of VIEWPORTS) {
      await browser.setViewport(vp.width, vp.height);
      await browser.navigate(sub);
      await sleep(500);
      const layout = await browser.checkLayout();
      const name = sub.replace('/x7m4q9k2/', 'admin_sub_');
      await browser.captureScreenshot(`deep_qa/${name}_${vp.name}.png`);
      console.log(`Admin ${sub} @ ${vp.name}: overflow=${layout.hasOverflow}`);
    }
  }

  await browser.close();
  console.log('--- Deep QA Complete! ---');
}

deepQA().catch(e => {
  console.error(e);
  process.exit(1);
});
