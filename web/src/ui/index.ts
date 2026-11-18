// Index page: instant client-side search and category filter.
import { load } from "./storage";
export function initIndex(): void {
  const input = document.getElementById("search") as HTMLInputElement | null;
  const cards = [...document.querySelectorAll<HTMLElement>(".card")];
  if (!input || cards.length === 0) return;
  const chips = [...document.querySelectorAll<HTMLButtonElement>(".chip")];
  const empty = document.getElementById("empty");
  const params = new URLSearchParams(location.search);
  input.value = params.get("q") ?? "";
  let cat = params.get("c") ?? "";
  const saved = new Set(load<string[]>("gc:saved", []));
  const savedChip = chips.find((c) => c.dataset.cat === "__saved");
  if (savedChip) savedChip.hidden = saved.size === 0;

  function apply(): void {
    const terms = input?.value.toLowerCase().trim().split(/\s+/).filter(Boolean) ?? [];
    let shown = 0;
    for (const c of cards) {
      const hay = c.dataset.search ?? "";
      const inCat = cat === "__saved" ? saved.has(c.dataset.slug ?? "") : !cat || c.dataset.cat === cat;
      const ok = inCat && terms.every((t) => hay.includes(t));
      c.hidden = !ok;
      if (ok) shown++;
    }
    for (const ch of chips) ch.setAttribute("aria-pressed", String((ch.dataset.cat ?? "") === cat));
    if (empty) empty.hidden = shown > 0;
    const q = new URLSearchParams();
    if (input?.value) q.set("q", input.value);
    if (cat) q.set("c", cat);
    const s = q.toString();
    history.replaceState(null, "", s ? `?${s}` : location.pathname);
  }

  input.addEventListener("input", apply);
  for (const ch of chips) {
    ch.addEventListener("click", () => {
      cat = ch.dataset.cat ?? "";
      apply();
    });
  }
  document.addEventListener("keydown", (e) => {
    if (e.key === "/" && document.activeElement !== input) {
      e.preventDefault();
      input.focus();
    }
  });
  apply();
}
