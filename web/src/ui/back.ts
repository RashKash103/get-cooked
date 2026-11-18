// The top-left back control on recipe and list pages. Coming from inside the
// site it acts like the browser's back button (keeping search, filters and
// scroll position); otherwise it links to the recipe index.
import { t } from "./i18n";

export function initBack(): void {
  const link = document.getElementById("back") as HTMLAnchorElement | null;
  if (!link) return;
  let ref: URL | null = null;
  try {
    ref = document.referrer ? new URL(document.referrer) : null;
  } catch {
    ref = null;
  }
  const index = new URL(link.href, location.href);
  const internal = ref !== null && ref.origin === location.origin && ref.href !== location.href && history.length > 1;
  if (!internal || !ref) return;
  const toIndex = ref.pathname === index.pathname;
  const label = link.querySelector("span");
  if (label) label.textContent = toIndex ? t("all_recipes") : t("back");
  link.addEventListener("click", (e) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    history.back();
  });
}
