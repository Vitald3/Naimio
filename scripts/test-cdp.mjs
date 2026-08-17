import { spawn } from 'node:child_process';
import { writeFileSync, mkdirSync } from 'node:fs';

const CHROME_PATH = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const PORT = 9222;

async function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

async function main() {
  console.log('Connecting to Chrome on port', PORT);
  const versionRes = await fetch(`http://127.0.0.1:${PORT}/json/version`);
  const version = await versionRes.json();
  console.log('Connected to Chrome:', version.Browser);

  // create a new target/page
  const newPageRes = await fetch(`http://127.0.0.1:${PORT}/json/new?http://localhost:8088/`, { method: 'PUT' });
  const page = await newPageRes.json();
  console.log('Page created:', page.id);

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  let msgId = 1;
  const pending = new Map();

  ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.id && pending.has(data.id)) {
      const { resolve, reject } = pending.get(data.id);
      pending.delete(data.id);
      if (data.error) reject(data.error);
      else resolve(data.result);
    }
  };

  await new Promise(r => { ws.onopen = r; });

  function send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = msgId++;
      pending.set(id, { resolve, reject });
      ws.send(JSON.stringify({ id, method, params }));
    });
  }

  await send('Page.enable');
  await send('Runtime.enable');
  await send('DOM.enable');

  // Set viewport to 1440x900
  await send('Emulation.setDeviceMetricsOverride', {
    width: 1440,
    height: 900,
    deviceScaleFactor: 1,
    mobile: false
  });

  await send('Page.navigate', { url: 'http://localhost:8088/' });
  await sleep(1500);

  const evalResult = await send('Runtime.evaluate', {
    expression: 'document.title'
  });
  console.log('Page Title:', evalResult.result.value);

  const screenshot = await send('Page.captureScreenshot', { format: 'png' });
  const buf = Buffer.from(screenshot.data, 'base64');
  mkdirSync('./screenshots', { recursive: true });
  writeFileSync('./screenshots/test_home_1440.png', buf);
  console.log('Screenshot saved to ./screenshots/test_home_1440.png (' + buf.length + ' bytes)');

  ws.close();
}

main().catch(e => {
  console.error(e);
  process.exit(1);
});
