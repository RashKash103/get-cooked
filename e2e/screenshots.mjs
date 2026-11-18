// Regenerates the README screenshots (light and dark) from the example site.
// Usage: make screenshots   (or: node e2e/screenshots.mjs, after `make build`)
import { spawn } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";
import { chromium } from "@playwright/test";

const root = new URL("..", import.meta.url).pathname;
const out = `${root}docs/screenshots`;
const port = 4199;
const base = `http://127.0.0.1:${port}`;
const server = spawn(`${root}bin/get-cooked`, ["serve", "-s", `${root}examples/site`, "-a", `127.0.0.1:${port}`, "--watch=false"], {
  stdio: "ignore",
});
await sleep(1200);

const mobile = { viewport: { width: 390, height: 780 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true };
const desktop = { viewport: { width: 1280, height: 860 }, deviceScaleFactor: 1.5 };
const hideToast = ".toast{display:none!important}";

const browser = await chromium.launch();
try {
  for (const scheme of ["light", "dark"]) {
    const shot = async (opts, name, fn) => {
      const ctx = await browser.newContext({ ...opts, colorScheme: scheme });
      const page = await ctx.newPage();
      await fn(page);
      await page.addStyleTag({ content: hideToast });
      await page.screenshot({ path: `${out}/${name}-${scheme}.png` });
      await ctx.close();
    };
    await shot(desktop, "desktop", async (p) => {
      await p.goto(`${base}/recipes/cranberry-baked-oatmeal/?s=6&o.walnuts=1`);
      await p.evaluate(() => {
        const head = document.querySelector(".recipe-head");
        if (head) window.scrollTo(0, head.getBoundingClientRect().top + window.scrollY - 64);
      });
    });
    await shot(mobile, "index", async (p) => {
      await p.goto(`${base}/`);
    });
    await shot(mobile, "recipe", async (p) => {
      await p.goto(`${base}/recipes/crispy-chicken-thighs/?o.method=air_fryer`);
      await p.evaluate(() => window.scrollTo(0, 90));
    });
    await shot(mobile, "customize", async (p) => {
      await p.goto(`${base}/recipes/crispy-chicken-thighs/?o.method=air_fryer`);
      await p.locator("#customize").click();
      await sleep(300);
    });
    await shot(mobile, "cook", async (p) => {
      await p.goto(`${base}/recipes/cranberry-baked-oatmeal/`);
      await p.locator("#cook-mode").click();
      await p.locator("#next-step").click();
      await p.locator("#next-step").click();
      await sleep(700);
      await p.locator(".timer-chip").first().click();
      await sleep(300);
    });
    await shot(mobile, "list", async (p) => {
      for (const u of ["/recipes/buttermilk-pancakes/?s=6&o.blueberries=1", "/recipes/cranberry-baked-oatmeal/"]) {
        await p.goto(base + u);
        await p.locator("#add-to-list").click();
      }
      await p.goto(`${base}/list/`);
    });
  }
} finally {
  await browser.close();
  server.kill();
}
console.log(`screenshots written to ${out}`);
