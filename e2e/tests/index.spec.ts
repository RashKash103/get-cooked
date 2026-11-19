import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("lists recipes and filters by search and category", async ({ page }) => {
  await page.goto("/");
  const cards = page.locator(".card:visible");
  await expect(cards).toHaveCount(7);
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

test("theme toggle cycles and is remembered", async ({ page }) => {
  await page.goto("/");
  const html = page.locator("html");
  const toggle = page.locator("#theme-toggle");
  const startDark = await html.evaluate((el) => el.classList.contains("dark"));
  await toggle.click(); // system → light
  await expect(html).not.toHaveClass(/dark/);
  await toggle.click(); // light → dark
  await expect(html).toHaveClass(/dark/);
  await page.reload();
  await expect(html).toHaveClass(/dark/);
  await toggle.click(); // back to system
  expect(await html.evaluate((el) => el.classList.contains("dark"))).toBe(startDark);
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
