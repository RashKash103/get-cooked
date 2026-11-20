import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("lists recipes and filters by search and category", async ({ page }) => {
  await page.goto("/");
  const cards = page.locator(".card:visible");
  await expect(cards).toHaveCount(8);
  await page.fill("#search", "cranberr");
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("Cranberry Baked Oatmeal");
  await expect(page).toHaveURL(/\?q=cranberr/);
  // Ingredient names are searchable too.
  await page.fill("#search", "paprika");
  await expect(cards).toHaveCount(1);
  await page.fill("#search", "zzz");
  await expect(page.locator("#empty")).toBeVisible();
  await page.fill("#search", "");
  await page.getByRole("button", { name: "Desserts" }).click();
  await expect(cards).toHaveCount(1);
  await expect(page.getByRole("button", { name: "Desserts" })).toHaveAttribute("aria-pressed", "true");
});

test("largest text size fits small screens without clipping the chrome", async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.setItem("gc:text", "150"));
  for (const path of ["/", "/recipes/crispy-chicken-thighs/", "/list/"]) {
    await page.goto(path);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth), path).toBe(false);
    // Every top-bar control is fully inside the viewport.
    const rights = await page
      .locator(".topbar a, .topbar summary, .topbar select")
      .evaluateAll((els) => els.map((e) => e.getBoundingClientRect().right));
    for (const r of rights) expect(r, path).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth));
  }
  await page.evaluate(() => localStorage.removeItem("gc:text"));
});

test("has no horizontal overflow and no serious accessibility issues", async ({ page }) => {
  for (const path of ["/", "/recipes/cranberry-baked-oatmeal/", "/es/recipes/crispy-chicken-thighs/"]) {
    await page.goto(path);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
    expect(overflow, `${path} overflows horizontally`).toBe(false);
    const results = await new AxeBuilder({ page }).analyze();
    const serious = results.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(", ")}`)).toEqual([]);
  }
});

test("display menu sets theme and text size, remembered across pages", async ({ page }) => {
  await page.goto("/");
  const html = page.locator("html");
  const menu = page.locator("#display-menu");
  await menu.locator("summary").click();
  await menu.locator('[data-theme-set="dark"]').click();
  await expect(html).toHaveClass(/dark/);
  await menu.locator('[data-theme-set="light"]').click();
  await expect(html).not.toHaveClass(/dark/);
  await expect(menu.locator('[data-theme-set="light"]')).toHaveAttribute("aria-pressed", "true");

  const titleHeight = () => page.locator(".index-title").evaluate((el) => el.getBoundingClientRect().height);
  const before = await titleHeight();
  const barBefore = await page.locator(".topbar").evaluate((el) => el.getBoundingClientRect().height);
  await menu.locator('[data-text-size="1"]').click();
  await menu.locator('[data-text-size="1"]').click();
  await expect(page.locator("#text-size-value")).toHaveText("125%");
  expect(await titleHeight()).toBeCloseTo(before * 1.25, 0);
  // The top bar keeps its size.
  expect(await page.locator(".topbar").evaluate((el) => el.getBoundingClientRect().height)).toBeCloseTo(barBefore, 0);

  // Remembered on another page, applied before first paint.
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await expect(html).not.toHaveClass(/dark/);
  const zoom = () => page.evaluate(() => document.documentElement.style.getPropertyValue("--gc-zoom"));
  expect(await zoom()).toBe("1.25");
  await page.locator("#display-menu summary").click();
  await expect(page.locator("#text-size-value")).toHaveText("125%");
  await page.locator("#text-size-value").click(); // reset
  await expect(page.locator("#text-size-value")).toHaveText("100%");
  expect(await zoom()).toBe("");
  // Limits disable the buttons.
  for (let i = 0; i < 6; i++) await page.locator('#display-menu [data-text-size="-1"]').click({ force: true });
  await expect(page.locator('#display-menu [data-text-size="-1"]')).toBeDisabled();
  await expect(page.locator("#text-size-value")).toHaveText("87.5%");
  await page.locator("#display-menu").locator('[data-theme-set="system"]').click();
  await page.locator("#text-size-value").click();
});

test("back control returns to the list with search kept, or links home", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("#back")).toHaveCount(0);
  await page.fill("#search", "bread");
  await page.locator(".card:visible a", { hasText: "Sandwich Bread" }).click();
  await expect(page).toHaveURL(/sandwich-bread/);
  await expect(page.locator("#back")).toHaveText("All recipes");
  await page.locator("#back").click();
  await expect(page).toHaveURL(/\/\?q=bread$/);
  await expect(page.locator("#search")).toHaveValue("bread");
  await expect(page.locator(".card:visible")).toHaveCount(2);

  // From another recipe it still goes to the recipes page.
  await page.goto("/recipes/french-toast/");
  await page.locator(".ing[data-id=bread] a").click();
  await expect(page).toHaveURL(/sandwich-bread/);
  await expect(page.locator("#back")).toHaveText("All recipes");
  await page.locator("#back").click();
  await expect(page).toHaveURL(/127\.0\.0\.1:\d+\/$/);

  // After switching language it goes to that language's recipes page.
  await page.goto("/");
  await page.locator(".card a").first().click();
  await page.locator("#lang-picker").selectOption({ label: "Español" });
  await expect(page).toHaveURL(/\/es\/recipes\//);
  await expect(page.locator("#back")).toHaveText("Todas las recetas");
  await page.locator("#back").click();
  await expect(page).toHaveURL(/\/es\/$/);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  // Reset the remembered language for the other tests' fresh contexts.
  await page.evaluate(() => localStorage.removeItem("gc:lang"));

  // Opened directly (shared link), it goes to the index.
  const fresh = await page.context().newPage();
  await fresh.goto("/recipes/creamy-tomato-soup/");
  await expect(fresh.locator("#back")).toHaveText("All recipes");
  await fresh.locator("#back").click();
  await expect(fresh).toHaveURL(/127\.0\.0\.1:\d+\/$/);
});
