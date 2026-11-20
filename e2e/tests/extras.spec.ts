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
  await page.locator(".ing[data-id=eggs] .qty").click();
  const sheet = page.locator("dialog.convert-sheet");
  await sheet.locator("#scale-have").fill("3");
  await sheet.locator("#scale-go").click();
  await expect(sheet).toBeHidden();
  await expect(page.locator("#yield")).toHaveText("6 servings");
  await expect(page.locator(".ing[data-id=eggs] .ing-check")).not.toBeChecked();
});

test("tapping an amount shows it in other units", async ({ page }) => {
  await page.goto("/recipes/buttermilk-pancakes/");
  await page.locator(".ing[data-id=all_purpose_flour] .qty").click();
  const sheet = page.locator("dialog.convert-sheet");
  await expect(sheet.locator("h2")).toHaveText("all-purpose flour");
  await expect(sheet.locator(".convert-now")).toContainText("2 cups");
  await expect(sheet.locator(".convert-list dt")).toHaveText([
    "US · by weight",
    "Metric · by volume",
    "Metric · by weight",
  ]);
  await expect(sheet.locator(".convert-list dd")).toHaveText(["8 ¾ oz", "473 ml", "250 g"]);
  await sheet.getByRole("button", { name: "Close" }).click();
  // Eggs convert to their weight.
  await page.locator(".ing[data-id=eggs] .qty").click();
  await expect(sheet.locator(".convert-list dd").first()).toHaveText(/oz$/);
});

test("substitutes swap an ingredient everywhere it's used", async ({ page }) => {
  await page.goto("/recipes/french-toast/");
  const milk = page.locator(".ing[data-id=milk]");
  await milk.locator(".swap-btn").click();
  const sheet = page.locator("dialog.swap-sheet");
  await expect(sheet.locator("h2")).toHaveText("Swap milk");
  await expect(sheet.locator(".swap-choice")).toHaveText(["milk", "coconut milk", "heavy cream + water"]);
  await sheet.getByText("heavy cream + water").click();
  await expect(sheet).toBeHidden();
  await expect(milk).toContainText("½ cup heavy cream");
  await expect(page.locator(".ing[data-id=milk_water]")).toContainText("½ cup water");
  await expect(page.locator("#steps")).toContainText("½ cup heavy cream and ½ cup water");
  await expect(page.locator("#changes")).toBeVisible();
  // The choice is in the share link and survives a reload.
  await expect(page).toHaveURL(/o\.swap_milk=heavy_cream_water/);
  await page.reload();
  await expect(milk).toContainText("heavy cream");
});

test("tapping an ingredient in a step ticks it off", async ({ page }) => {
  await page.goto("/recipes/french-toast/");
  const ref = page.locator('.step .ref[data-ing="eggs"]').first();
  await ref.click();
  await expect(ref).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".ing[data-id=eggs] .ing-check")).toBeChecked();
  // The step itself isn't marked done by that tap.
  await expect(page.locator(".step.done")).toHaveCount(0);
  await page.locator(".ing[data-id=eggs] .ing-check").uncheck();
  await expect(ref).toHaveAttribute("aria-pressed", "false");
});

test("several timers collapse and link back to their step", async ({ page }) => {
  await page.clock.install();
  await page.goto("/recipes/buttermilk-pancakes/");
  await page.locator(".timer-chip").first().click();
  await page.locator(".timer-chip").nth(1).click();
  await page.locator("dialog.timer-picker #timer-picker-start").click();
  const timers = page.locator("#timers .timer");
  await expect(timers).toHaveCount(1);
  await page.locator(".timers-more").click();
  await expect(timers).toHaveCount(2);
  await page.locator(".timers-more").click();
  await expect(timers).toHaveCount(1);
  // From another page, the timer opens its recipe at the step.
  await page.goto("/");
  await page.locator('#timers .timer-info[data-act="go"]').click();
  await expect(page).toHaveURL(/buttermilk-pancakes\/#step-\d$/);
});

test("a shopping list can be shared as a link", async ({ page, context }) => {
  await page.goto("/recipes/french-toast/");
  await page.locator("#add-to-list").click();
  await page.goto("/list/");
  // Capture the copied link (the list is compressed asynchronously first).
  await page.evaluate(() => {
    Object.defineProperty(navigator, "share", { value: undefined });
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: async (s: string) => ((window as unknown as { copied: string }).copied = s) },
    });
  });
  await page.locator("#list-share").click();
  await expect(page.locator("#toast")).toHaveText("Shopping list link copied");
  const link = await page.evaluate(() => (window as unknown as { copied: string }).copied);
  expect(link).toMatch(/\/list\/#l=z/);
  // Someone else opens it.
  const other = await context.browser()?.newPage();
  if (!other) return;
  other.once("dialog", (d) => d.accept());
  await other.goto(link);
  await expect(other.locator(".list-recipe")).toHaveText(/French Toast/);
  await expect(other).toHaveURL(/\/list\/$/);
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

test("image variants follow the reader's choices", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('.card a[href$="/crispy-chicken-thighs/"] img')).toHaveAttribute(
    "src",
    /crispy-chicken-thighs\.svg$/,
  );
  await page.goto("/recipes/crispy-chicken-thighs/");
  const hero = page.locator("#hero");
  await expect(hero).toHaveAttribute("src", /crispy-chicken-thighs\.svg$/);
  await page.locator("#customize").click();
  await page.locator("#sheet").getByText("Air fryer").click();
  await expect(hero).toHaveAttribute("src", /crispy-chicken-thighs-air-fryer\.svg$/);
  await expect(hero).toHaveAttribute("alt", "Crispy chicken thighs in an air fryer basket");
  await page.locator("#sheet").getByText("Oven").click();
  await expect(hero).toHaveAttribute("src", /crispy-chicken-thighs\.svg$/);
  // A variant chosen through the URL is server-side independent but applied on load.
  await page.goto("/recipes/buttermilk-pancakes/?o.blueberries=1");
  await expect(hero).toHaveAttribute("src", /buttermilk-pancakes-blueberries\.svg$/);
});

test("time ranges open a picker with quick choices and a slider", async ({ page }) => {
  await page.clock.install();
  await page.goto("/recipes/creamy-tomato-soup/");
  const chip = page.locator('.timer-chip[data-max="25"]');
  await expect(chip).toContainText("20–25 min");
  await chip.click();
  const picker = page.locator("dialog.timer-picker");
  await expect(picker).toBeVisible();
  await expect(picker.locator("[data-sec]")).toHaveText(["20 min", "21 min", "23 min", "24 min", "25 min"]);
  // The middle choice is preselected; pick the top of the range instead.
  await expect(picker.locator('[data-sec="1380"]')).toHaveAttribute("aria-pressed", "true");
  await picker.locator('[data-sec="1500"]').click();
  await expect(picker.locator("#timer-picker-value")).toHaveText("25 min");
  // The slider can pick anything in between.
  await picker.locator("#timer-picker-range").fill("1320");
  await expect(picker.locator("#timer-picker-value")).toHaveText("22 min");
  await picker.locator("#timer-picker-start").click();
  await expect(picker).toBeHidden();
  await expect(page.locator(".timer .timer-left")).toHaveText("22:00");
  await page.locator(".timer").getByRole("button", { name: "Cancel timer" }).click();
});

test("start over clears ticked ingredients and finished steps", async ({ page }) => {
  await page.goto("/recipes/french-toast/");
  await expect(page.locator("#start-over")).toBeHidden();
  await page.locator(".ing[data-id=eggs] .ing-check").check();
  await page.locator(".step[data-n='1'] .step-num").click();
  await expect(page.locator("#start-over")).toBeVisible();
  // The recipes page shows it as in progress.
  await page.goto("/");
  const card = page.locator('.card[data-slug="french-toast"]');
  await expect(card.locator(".card-badge")).toHaveText("In progress");
  await page.locator('.chip[data-cat="__progress"]').click();
  await expect(page.locator(".card:visible")).toHaveCount(1);
  await page.goto("/recipes/french-toast/");
  await page.locator("#start-over").click();
  await expect(page.locator(".ing[data-id=eggs] .ing-check")).not.toBeChecked();
  await expect(page.locator(".step.done")).toHaveCount(0);
  await expect(page.locator("#start-over")).toBeHidden();
  await page.goto("/");
  await expect(page.locator('.chip[data-cat="__progress"]')).toBeHidden();
});

test("swiping in cook mode moves between steps", async ({ page }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await page.locator("#cook-mode").click();
  await expect(page.locator("#step-of")).toHaveText("Step 1 of 7");
  const swipe = (dx: number) =>
    page.locator("#steps").evaluate((el, dx) => {
      const r = el.getBoundingClientRect();
      const x = r.left + r.width / 2;
      const y = r.top + 40;
      const t0 = new Touch({ identifier: 1, target: el, clientX: x, clientY: y });
      const t1 = new Touch({ identifier: 1, target: el, clientX: x + dx, clientY: y + 5 });
      el.dispatchEvent(new TouchEvent("touchstart", { touches: [t0], changedTouches: [t0], bubbles: true }));
      el.dispatchEvent(new TouchEvent("touchend", { touches: [], changedTouches: [t1], bubbles: true }));
    }, dx);
  await swipe(-120);
  await expect(page.locator("#step-of")).toHaveText("Step 2 of 7");
  await swipe(120);
  await expect(page.locator("#step-of")).toHaveText("Step 1 of 7");
});

test("the largest text size never scrolls sideways", async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.setItem("gc:text", "150"));
  const urls = await page.locator(".card-link").evaluateAll((as) => as.map((a) => (a as HTMLAnchorElement).href));
  for (const u of [...urls, new URL("/list/", page.url()).href]) {
    await page.goto(u);
    const over = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(over, u).toBeLessThanOrEqual(0);
  }
});

test("the footer names the get-cooked version", async ({ page }) => {
  await page.goto("/");
  const footer = page.locator(".site-footer");
  await expect(footer.getByRole("link", { name: "get-cooked" })).toHaveAttribute(
    "href",
    "https://github.com/RashKash103/get-cooked",
  );
  // The e2e binary is a local build ("dev" or a Go pseudo-version), which
  // has no release page, so it links to the list of releases.
  const v = footer.locator(".gen-version");
  await expect(v).not.toHaveText("");
  await expect(v).toHaveAttribute("href", "https://github.com/RashKash103/get-cooked/releases");
});

test("printing keeps headings with their steps and drops controls", async ({ page }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await page.emulateMedia({ media: "print" });
  await expect(page.locator(".swap-btn").first()).toBeHidden();
  // Equipment moves into the header so it stays on page one.
  await expect(page.locator("#equipment-wrap")).toBeHidden();
  await expect(page.locator("#print-equip")).toBeVisible();
  // Grid items fragment badly across pages in some engines; print uses a float.
  expect(await page.locator(".recipe-grid").evaluate((el) => getComputedStyle(el).display)).toBe("block");
  const title = page.locator(".col-steps .h2");
  expect(await title.evaluate((el) => getComputedStyle(el).breakInside)).toBe("avoid");
  expect(await title.evaluate((el) => getComputedStyle(el, "::after").height)).not.toBe("auto");
});

test("a source credit links to its page in the built-in PDF viewer", async ({ page }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await expect(page.locator(".recipe-source")).toHaveText("Adapted from The Example Kitchen, with less sugar.");
  await page.goto("/recipes/chocolate-chip-cookies/");
  const credit = page.locator(".recipe-source");
  await expect(credit).toHaveText("Modified from An Example Cookbook, p. 2");
  await credit.getByRole("link").click();
  await expect(page).toHaveURL(/\/pdf\/\?f=example-cookbook\.[0-9a-f]+\.pdf&p=2/);
  await expect(page.locator("#pdf-canvas")).toBeVisible();
  await expect(page.locator("#pdf-num")).toHaveValue("2");
  await expect(page.locator("#pdf-total")).toHaveText(/of 3/);
  await page.locator("#pdf-next").click();
  await expect(page.locator("#pdf-num")).toHaveValue("3");
  await expect(page.locator("#pdf-next")).toBeDisabled();
  await expect(page).toHaveURL(/p=3/);
  await page.locator("#back").click();
  await expect(page).toHaveURL(/\/recipes\/chocolate-chip-cookies\/$/);
});

test("the PDF viewer refuses files outside the site's PDFs", async ({ page }) => {
  await page.goto("/pdf/?f=..%2F..%2Fsecret.pdf");
  await expect(page.locator("#pdf-status")).toHaveText("Couldn't show this PDF.");
});
