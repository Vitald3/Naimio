import { writeFileSync, mkdirSync } from 'node:fs';

const PORT = 9222;
const BASE_URL = 'http://localhost:8088';

export async function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

export class BrowserController {
  constructor(port = PORT) {
    this.port = port;
    this.ws = null;
    this.pageId = null;
    this.msgId = 1;
    this.pending = new Map();
  }

  async init(url = `${BASE_URL}/`) {
    const newPageRes = await fetch(`http://127.0.0.1:${this.port}/json/new?${encodeURIComponent(url)}`, { method: 'PUT' });
    const page = await newPageRes.json();
    this.pageId = page.id;

    this.ws = new WebSocket(page.webSocketDebuggerUrl);
    this.ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.id && this.pending.has(data.id)) {
        const { resolve, reject } = this.pending.get(data.id);
        this.pending.delete(data.id);
        if (data.error) reject(data.error);
        else resolve(data.result);
      }
    };

    await new Promise(r => { this.ws.onopen = r; });

    await this.send('Page.enable');
    await this.send('Runtime.enable');
    await this.send('DOM.enable');
    await this.send('Network.enable');
  }

  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = this.msgId++;
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }

  async setViewport(width, height = 900) {
    await this.send('Emulation.setDeviceMetricsOverride', {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: width < 768
    });
  }

  async navigate(path) {
    const url = path.startsWith('http') ? path : `${BASE_URL}${path}`;
    await this.send('Page.navigate', { url });
    await sleep(1000);
  }

  async eval(expression) {
    const res = await this.send('Runtime.evaluate', {
      expression,
      returnByValue: true,
      awaitPromise: true
    });
    return res.result?.value;
  }

  async captureScreenshot(filename) {
    const screenshot = await this.send('Page.captureScreenshot', { format: 'png' });
    const buf = Buffer.from(screenshot.data, 'base64');
    mkdirSync('./screenshots', { recursive: true });
    writeFileSync(`./screenshots/${filename}`, buf);
    return `./screenshots/${filename}`;
  }

  async checkLayout() {
    return await this.eval(`(() => {
      const docW = document.documentElement.scrollWidth;
      const winW = window.innerWidth;
      const hasOverflow = docW > winW;
      
      const overflowingElements = [];
      if (hasOverflow) {
        const all = document.querySelectorAll('*');
        for (const el of all) {
          const rect = el.getBoundingClientRect();
          if (rect.right > winW + 1) {
            overflowingElements.push({
              tag: el.tagName,
              className: el.className?.toString?.() || '',
              id: el.id,
              right: rect.right,
              width: rect.width,
              text: el.innerText ? el.innerText.slice(0, 50) : ''
            });
            if (overflowingElements.length >= 5) break;
          }
        }
      }

      // Check for zero-sized or clipped critical interactive elements
      const hiddenButtons = [];
      const buttons = document.querySelectorAll('button, a.btn');
      for (const btn of buttons) {
        const rect = btn.getBoundingClientRect();
        const style = window.getComputedStyle(btn);
        if (style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0') {
          if (rect.width === 0 && rect.height === 0 && btn.innerText.trim()) {
            hiddenButtons.push({
              text: btn.innerText.trim().slice(0, 30),
              className: btn.className?.toString?.() || ''
            });
          }
        }
      }

      return {
        docW,
        winW,
        hasOverflow,
        overflowingElements,
        hiddenButtons,
        title: document.title
      };
    })()`);
  }

  async loginViaForm(email, password, redirectUrl = '/') {
    await this.navigate('/login');
    await sleep(500);
    const loginOk = await this.eval(`(async () => {
      const emailInput = document.querySelector('input[type="email"], input[name="email"], input[name="login"]');
      const passInput = document.querySelector('input[type="password"], input[name="password"]');
      const form = document.querySelector('form');
      if (emailInput && passInput && form) {
        emailInput.value = ${JSON.stringify(email)};
        emailInput.dispatchEvent(new Event('input', { bubbles: true }));
        emailInput.dispatchEvent(new Event('change', { bubbles: true }));
        passInput.value = ${JSON.stringify(password)};
        passInput.dispatchEvent(new Event('input', { bubbles: true }));
        passInput.dispatchEvent(new Event('change', { bubbles: true }));
        form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        return true;
      }
      return false;
    })()`);
    await sleep(1500);
    if (redirectUrl) {
      await this.navigate(redirectUrl);
    }
  }

  async close() {
    if (this.ws) {
      this.ws.close();
    }
    if (this.pageId) {
      try {
        await fetch(`http://127.0.0.1:${this.port}/json/close/${this.pageId}`);
      } catch (e) {}
    }
  }
}
