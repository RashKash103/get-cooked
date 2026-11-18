import { expect, type Page, test } from "@playwright/test";

const oatmeal = "/recipes/cranberry-baked-oatmeal/";

async function setServings(page: Page, n: number) {
  for (let i = 0; i < 20; i++) {
    const text = (await page.locator("#yield").textContent()) ?? "";
    const cur = Number.parseFloat(text);
    if (cur === n) return;
    await page.locator(`[data-servings="${cur > n ? -1 : 1}"]`).click();
  }
  throw new Error(`could not reach ${n} servings`);
}

async function openSheet(page: Page) {
  await page.locator("#customize").click();
  await expect(page.locator("#sheet")).toBeVisible();
}

test("is fully rendered without JavaScript", async ({ browser }) => {
  const ctx = await browser.newContext({ javaScriptEnabled: false });
  const page = await ctx.newPage();
  await page.goto(oatmeal);
  await expect(page.locator(".ing")).toHaveCount(10);
  await expect(page.locator("#steps")).toContainText("Heat the oven to 375°F");
  await expect(page.locator("#yield")).toHaveText("4 servings");
  await ctx.close();
});

test("servings change pans, times, amounts and notes", async ({ page }) => {
  await page.goto(oatmeal);
  await expect(page.locator("#equipment")).toContainText("13×9 inch baking dish");
  await setServings(page, 3);
  await expect(page.locator("#times")).toContainText("36 min"); // 30 + 3 × 2
  await expect(page.locator("#warnings")).not.toContainText("softer");
  await expect(page.locator("#warnings")).toContainText("Example only");
  await setServings(page, 2);
  await expect(page.locator("#equipment")).toContainText("8×8 inch baking dish");
  await expect(page.locator(".ing[data-id=oats]")).toContainText("1 cup rolled oats");
  await setServings(page, 1);
  await expect(page.locator("#warnings")).toContainText("softer");
  await expect(page.locator(".ing[data-id=salt]")).toContainText("pinch");
  await expect(page).toHaveURL(/\?s=1/);
  // State survives a reload (URL) and a fresh visit (local storage).
  await page.reload();
  await expect(page.locator("#yield")).toHaveText("1 serving");
  await page.goto(oatmeal.replace(/\/$/, "/?"));
  await page.goto(oatmeal);
  await expect(page.locator("#yield")).toHaveText("1 serving");
  await openSheet(page);
  await page.locator("#reset").click();
  await expect(page.locator("#yield")).toHaveText("4 servings");
});

test("options add and rewrite steps; units convert", async ({ page }) => {
  await page.goto(oatmeal);
  const steps = page.locator(".step");
  await expect(steps).toHaveCount(7);
  await expect(page.locator(".ing[data-id=cranberries] .ing-uses")).toContainText("for step 3");
  await openSheet(page);
  await page.getByLabel("Add dried cranberries").uncheck();
  await expect(steps).toHaveCount(6);
  await expect(page.locator("#steps")).not.toContainText("cranberries");
  await page.getByLabel("Add walnuts").check();
  await expect(page.locator("#steps")).toContainText("fold in ⅓ cup walnuts");
  await page.locator("#sheet").getByText("Brown sugar").click();
  await expect(page.locator(".ing[data-id=brown_sugar]")).toBeVisible();
  await expect(page.locator(".ing[data-id=maple]")).toHaveCount(0);
  await page.locator("#sheet").getByText("Metric").click();
  await expect(page.locator(".ing[data-id=oats]")).toContainText("180 g");
  await expect(page.locator("#steps")).toContainText("190°C");
  await page.keyboard.press("Escape");
  await expect(page.locator("#changes")).toBeVisible();
});

test("ingredient checklist and step progress are remembered", async ({ page }) => {
  await page.goto(oatmeal);
  await page.locator(".ing[data-id=oats] .ing-check").check();
  await page.locator(".step[data-n='2'] .step-num").click();
  await expect(page.locator(".step[data-n='2']")).toHaveClass(/done/);
  await page.reload();
  await expect(page.locator(".ing[data-id=oats] .ing-check")).toBeChecked();
  await expect(page.locator(".step[data-n='2']")).toHaveClass(/done/);
});

test("timers count down and ring", async ({ page }) => {
  await page.clock.install();
  await page.goto(oatmeal);
  await page.locator(".timer-chip").first().click();
  const timer = page.locator(".timer");
  await expect(timer).toHaveCount(1);
  await expect(timer).toContainText("Step 4");
  await page.clock.runFor(60_000);
  await expect(timer.locator(".timer-left")).toHaveText("4:00");
  await timer.getByRole("button", { name: "Add one minute" }).click();
  await page.clock.runFor(5 * 60_000);
  await expect(timer).toHaveClass(/done/);
  await expect(timer).toContainText("Done!");
  // Timers persist across pages.
  await page.goto("/");
  await expect(page.locator(".timer")).toHaveCount(1);
  await page.locator(".timer").getByRole("button", { name: "Dismiss timer" }).click();
  await expect(page.locator(".timer")).toHaveCount(0);
});

test("cook mode keeps the screen awake", async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { __locks: number };
    w.__locks = 0;
    Object.defineProperty(navigator, "wakeLock", {
      value: {
        request: async () => {
          w.__locks++;
          return { release: async () => {}, addEventListener() {} };
        },
      },
    });
  });
  await page.goto(oatmeal);
  await page.locator("#cook-mode").click();
  await expect(page.locator("#cook-mode")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("body")).toHaveClass(/cooking/);
  await expect(page.locator(".step.current")).toHaveCount(1);
  expect(await page.evaluate(() => (window as unknown as { __locks: number }).__locks)).toBe(1);
  await page.locator(".step.current .step-num").click();
  await expect(page.locator(".step[data-n='1']")).toHaveClass(/done/);
  await expect(page.locator(".step[data-n='2']")).toHaveClass(/current/);
});

test("recipe references open the linked recipe with the right state", async ({ page }) => {
  await page.goto("/recipes/french-toast/?s=6&o.wheat_bread=1");
  const link = page.locator(".ing[data-id=bread] a");
  await expect(link).toContainText("sandwich bread");
  await expect(page.locator(".ing[data-id=bread]")).toContainText("¾ loaf");
  await link.click();
  await expect(page).toHaveURL(/\/recipes\/sandwich-bread\/.*o\.wheat=25/);
  await expect(page.locator(".ing[data-id=ww_flour]")).toBeVisible();
});

test("divided ingredients list each step's share", async ({ page }) => {
  await page.goto("/recipes/crispy-chicken-thighs/");
  await expect(page.locator(".ing[data-id=salt] .ing-uses")).toHaveText("1 ¼ tsp for step 2 · ¼ tsp for step 6");
});
