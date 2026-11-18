import { t } from "./i18n";
import { load, save } from "./storage";

type Theme = "system" | "light" | "dark";
const ORDER: Theme[] = ["system", "light", "dark"];

function apply(theme: Theme): void {
  const dark = theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  const btn = document.getElementById("theme-toggle");
  if (btn) {
    btn.dataset.theme = theme;
    btn.setAttribute("aria-label", `${t("theme")}: ${t(`theme_${theme}`)}`);
  }
}

export function initTheme(): void {
  let theme = load<Theme>("gc:theme", "system");
  if (!ORDER.includes(theme)) theme = "system";
  apply(theme);
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => apply(theme));
  document.getElementById("theme-toggle")?.addEventListener("click", () => {
    theme = ORDER[(ORDER.indexOf(theme) + 1) % ORDER.length] as Theme;
    save("gc:theme", theme === "system" ? undefined : theme);
    apply(theme);
  });
}
