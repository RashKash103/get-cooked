// The "Aa" display menu: theme (system/light/dark) and text size, both
// remembered on this device. The page applies both before it first paints
// (inline script in base.html); this module keeps them in sync afterwards.
import { num, t } from "./i18n";
import { load, save } from "./storage";

type Theme = "system" | "light" | "dark";
const THEMES: Theme[] = ["system", "light", "dark"];

/** Text sizes as a percentage of the browser's default. */
export const TEXT_SIZES = [87.5, 100, 112.5, 125, 137.5, 150];

function applyTheme(theme: Theme): void {
  const dark = theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  for (const b of document.querySelectorAll<HTMLElement>("[data-theme-set]")) {
    b.setAttribute("aria-pressed", String(b.dataset.themeSet === theme));
  }
}

/** Steps through TEXT_SIZES from the current size (0 resets to 100%). */
export function nextTextSize(current: number, dir: number): number {
  if (dir === 0) return 100;
  let i = TEXT_SIZES.findIndex((s) => s >= current - 0.01);
  if (i < 0) i = TEXT_SIZES.length - 1;
  if (dir > 0 && (TEXT_SIZES[i] as number) <= current + 0.01) i++;
  if (dir < 0) i--;
  return TEXT_SIZES[Math.min(TEXT_SIZES.length - 1, Math.max(0, i))] as number;
}

function applyText(size: number): void {
  // Only the reading content scales (see .zoomable in styles.css); the top
  // bar and controls keep their size so they never clip on small screens.
  if (size === 100) document.documentElement.style.removeProperty("--gc-zoom");
  else document.documentElement.style.setProperty("--gc-zoom", String(size / 100));
  const out = document.getElementById("text-size-value");
  if (out) {
    out.textContent = `${num(size)}%`;
    out.setAttribute("aria-label", `${t("text_size")}: ${num(size)}%`);
  }
  const first = TEXT_SIZES[0] as number;
  const last = TEXT_SIZES[TEXT_SIZES.length - 1] as number;
  for (const b of document.querySelectorAll<HTMLButtonElement>("[data-text-size]")) {
    const d = Number(b.dataset.textSize);
    b.disabled = (d < 0 && size <= first) || (d > 0 && size >= last);
  }
}

export function initTheme(): void {
  let theme = load<Theme>("gc:theme", "system");
  if (!THEMES.includes(theme)) theme = "system";
  let size = load<number>("gc:text", 100);
  if (typeof size !== "number" || size < 50 || size > 200) size = 100;
  applyTheme(theme);
  applyText(size);
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => applyTheme(theme));
  const menu = document.getElementById("display-menu") as HTMLDetailsElement | null;
  menu?.addEventListener("click", (e) => {
    const el = e.target as Element;
    const th = el.closest<HTMLElement>("[data-theme-set]");
    if (th) {
      theme = th.dataset.themeSet as Theme;
      save("gc:theme", theme === "system" ? undefined : theme);
      applyTheme(theme);
      return;
    }
    const ts = el.closest<HTMLElement>("[data-text-size]");
    if (ts) {
      size = nextTextSize(size, Number(ts.dataset.textSize));
      save("gc:text", size === 100 ? undefined : size);
      applyText(size);
    }
  });
  document.addEventListener("click", (e) => {
    if (menu?.open && !menu.contains(e.target as Node)) menu.open = false;
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && menu?.open) menu.open = false;
  });
}
