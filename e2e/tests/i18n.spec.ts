import { expect, test } from "@playwright/test";

test("language picker switches, remembers and redirects", async ({ page, context }) => {
  await page.goto("/recipes/cranberry-baked-oatmeal/");
  await page.locator("#lang-picker").selectOption({ label: "Español" });
  await expect(page).toHaveURL(/\/es\/recipes\/cranberry-baked-oatmeal\//);
  await expect(page.locator("h1")).toHaveText("Avena horneada con arándanos");
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.locator(".ing[data-id=oats]")).toContainText("180 g de copos de avena");
  await expect(page.locator("#customize")).toContainText("Ajustar");
  // A fresh visit to an English page goes to the remembered language.
  const fresh = await context.newPage();
  await fresh.goto("/");
  await expect(fresh).toHaveURL(/\/es\/$/);
  await expect(fresh.locator("#search")).toHaveAttribute("placeholder", "Buscar recetas o ingredientes");
});

test("untranslated recipes fall back to the original language", async ({ page }) => {
  await page.goto("/es/recipes/crispy-chicken-thighs/");
  await expect(page.locator(".untranslated")).toHaveText("Esta receta aún no está traducida.");
  await expect(page.locator("#recipe")).toHaveAttribute("lang", "en");
  await expect(page.locator("h1")).toHaveText("Crispy Smoked-Paprika Chicken Thighs");
  await expect(page.locator(".h2").first()).toHaveText("Ingredientes");
});
