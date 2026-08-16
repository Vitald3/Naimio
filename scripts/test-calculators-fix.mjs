import { BrowserController, sleep } from './browser-controller.mjs';

const VIEWPORTS = [390, 430, 768, 1024, 1280, 1440];

async function run() {
  const browser = new BrowserController();
  await browser.init();

  await browser.navigate('/x7m4q9k2/login');
  await sleep(500);

  await browser.eval(`(async () => {
    const res = await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: "admin@example.test", password: "LocalDemo2026!", portal: "admin" }),
    });
    return res.ok;
  })()`);

  console.log('Testing /x7m4q9k2/calculators across all viewports...');
  let totalOverflows = 0;
  for (const vp of VIEWPORTS) {
    await browser.setViewport(vp, 900);
    await browser.navigate('/x7m4q9k2/calculators');
    await sleep(400);

    const layout = await browser.checkLayout();
    console.log(`Viewport ${vp}px: docW=${layout.docW}, winW=${layout.winW}, hasOverflow=${layout.hasOverflow}`);
    if (layout.hasOverflow) {
      totalOverflows++;
      console.error('Overflow details:', layout.overflowingElements);
    }
  }

  console.log(`Result: ${totalOverflows} overflows detected.`);
  await browser.close();
}

run().catch(console.error);
