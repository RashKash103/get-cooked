import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test("shopping list merges recipes and remembers checks", async ({ page }) => {
  await page.goto("/recipes/buttermilk-pancakes/");
  await page.locator("#add-to-list").click();
  await expect(page.locator("#add-to-list")).toContainText("Update list");
  await expect(page.locator("#list-count")).toHaveText("1");
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await page.locator("#add-to-list").click();
  await expect(page.locator("#list-count")).toHaveText("2");
  await page.locator(".list-link").click();
  await expect(page).toHaveURL(/\/list\/$/);
  await expect(page.locator(".list-recipe")).toHaveCount(2);
  // 2 eggs (pancakes) + 1 egg (oatmeal), merged under one line.
  await expect(page.locator(".ing[data-k=egg]")).toContainText("eggs");
  await expect(page.locator(".ing[data-k=egg] .list-qty")).toHaveText("3");
  // Grouped by aisle.
  await expect(page.locator(".ing-title").first()).toHaveText("Dairy & eggs");
  await page.locator(".ing[data-k=egg] .list-check").check();
  await page.reload();
  await expect(page.locator(".ing[data-k=egg] .list-check")).toBeChecked();
  await page.locator("#list-clear-checked").click();
  await expect(page.locator(".ing[data-k=egg]")).toHaveCount(0);
  await page.locator(".list-remove").first().click();
  await expect(page.locator(".list-recipe")).toHaveCount(1);
  page.once("dialog", (d) => d.accept());
  await page.locator("#list-clear").click();
  await expect(page.locator("#list-empty")).toBeVisible();
  await expect(page.locator("#list-count")).toBeHidden();
});

test("saved recipes appear as an index filter", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('.chip[data-cat="__saved"]')).toBeHidden();
  await page.goto("/recipes/creamy-tomato-soup/");
  await page.locator("#save").click();
  await expect(page.locator("#save")).toHaveAttribute("aria-pressed", "true");
  await page.goto("/");
  await page.locator('.chip[data-cat="__saved"]').click();
  await expect(page.locator(".card:visible")).toHaveCount(1);
  await expect(page.locator(".card:visible")).toContainText("Tomato");
});

test("tapping an amount scales the recipe to what you have", async ({ page }) => {
  await page.goto("/recipes/buttermilk-pancakes/");
  page.once("dialog", (d) => d.accept("3"));
  await page.locator(".ing[data-id=eggs] .qty").click();
  await expect(page.locator("#yield")).toHaveText("6 servings");
  await expect(page.locator(".ing[data-id=eggs] .ing-check")).not.toBeChecked();
});

test("cook mode steps forward and back", async ({ page }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await page.locator("#cook-mode").click();
  await expect(page.locator("#step-of")).toHaveText("Step 1 of 7");
  await page.locator("#next-step").click();
  await page.locator("#next-step").click();
  await expect(page.locator("#step-of")).toHaveText("Step 3 of 7");
  await expect(page.locator(".step[data-n='2']")).toHaveClass(/done/);
  await page.locator("#prev-step").click();
  await expect(page.locator("#step-of")).toHaveText("Step 2 of 7");
  await expect(page.locator(".step[data-n='2']")).not.toHaveClass(/done/);
  await page.locator("#exit-cook").click();
  await expect(page.locator("#customize")).toBeVisible();
});

test("exports Markdown for the current servings and options", async ({ page }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/?s=2&o.walnuts=1");
  await page.locator("#export-menu summary").click();
  const [dl] = await Promise.all([page.waitForEvent("download"), page.locator('[data-export="md"]').click()]);
  expect(dl.suggestedFilename()).toBe("cranberry-baked-oatmeal.md");
  const md = readFileSync((await dl.path()) as string, "utf8");
  expect(md).toContain("# Cranberry Baked Oatmeal");
  expect(md).toContain("**2 servings");
  expect(md).toContain("Add walnuts");
  expect(md).toContain("8×8 inch baking dish");
});

test("personal notes are kept per recipe", async ({ page }) => {
  await page.goto("/recipes/sandwich-bread/");
  await page.locator("#my-notes summary").click();
  await page.locator("#my-notes-text").fill("Use the darker pan.");
  await page.reload();
  await expect(page.locator("#my-notes")).toHaveAttribute("open", "");
  await expect(page.locator("#my-notes-text")).toHaveValue("Use the darker pan.");
});

test("ranges render and scale", async ({ page }) => {
  await page.goto("/recipes/creamy-tomato-soup/?s=12");
  await expect(page.locator(".ing[data-id=garlic]")).toContainText("6–8 cloves garlic");
});
