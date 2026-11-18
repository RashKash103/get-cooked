// Language picker. The choice is remembered on the device; arriving at the
// site in another language redirects to the remembered one.
import { load, save } from "./storage";

export function initLang(): void {
  const picker = document.getElementById("lang-picker") as HTMLSelectElement | null;
  if (!picker) return;
  const current = document.documentElement.lang;
  picker.addEventListener("change", () => {
    const opt = picker.selectedOptions[0];
    if (!opt) return;
    save("gc:lang", opt.dataset.lang);
    location.href = opt.value + location.search;
  });
  const wanted = load<string | null>("gc:lang", null);
  const internal = document.referrer && new URL(document.referrer).origin === location.origin;
  if (wanted && wanted !== current && !internal) {
    const alt = document.querySelector<HTMLLinkElement>(`link[rel="alternate"][hreflang="${CSS.escape(wanted)}"]`);
    if (alt) location.replace(alt.href + location.search + location.hash);
  }
}
