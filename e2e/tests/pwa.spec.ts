import { expect, test } from "@playwright/test";

test.describe("installable app", () => {
  test("Chrome reports the site as installable, with PNG and maskable icons", async ({
    page,
    context,
    browserName,
  }) => {
    test.skip(browserName !== "chromium", "installability is reported by Chromium");
    await page.goto("/");
    await page.evaluate(() => navigator.serviceWorker.ready);
    const cdp = await context.newCDPSession(page);
    const { installabilityErrors } = await cdp.send("Page.getInstallabilityErrors");
    expect(installabilityErrors).toEqual([]);
    const manifest = await (await page.request.get("/manifest.webmanifest")).json();
    expect(manifest.display).toBe("standalone");
    expect(manifest.id).toBe("/");
    const purposes = manifest.icons.map((i: { sizes: string; purpose: string }) => `${i.sizes} ${i.purpose}`);
    expect(purposes).toEqual(expect.arrayContaining(["192x192 any", "512x512 any", "512x512 maskable"]));
    for (const icon of manifest.icons) {
      const res = await page.request.get(icon.src);
      expect(res.ok(), icon.src).toBe(true);
    }
    await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute("href", "/apple-touch-icon.png");
  });

  test("works offline: visited pages load, others show the offline page", async ({ page, context }) => {
    await page.goto("/recipes/cranberry-baked-oatmeal/");
    await page.evaluate(() => navigator.serviceWorker.ready);
    // Reload once so the page is controlled by the service worker and cached.
    await page.reload();
    await expect.poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller))).toBe(true);
    await page.reload();
    await context.setOffline(true);
    await page.goto("/recipes/cranberry-baked-oatmeal/");
    await expect(page.locator("h1")).toHaveText("Cranberry Baked Oatmeal");
    await page.goto("/recipes/sandwich-bread/");
    await expect(page.locator("h1")).toHaveText("You're offline");
    await context.setOffline(false);
  });
});
