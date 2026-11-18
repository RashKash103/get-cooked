import "./styles.css";
import { initIndex } from "./ui/index";
import { initLang } from "./ui/lang";
import { initRecipe } from "./ui/recipe";
import { initTheme } from "./ui/theme";
import { initTimers } from "./ui/timers";

initLang();
initTheme();
initTimers();
initIndex();
initRecipe();

if ("serviceWorker" in navigator && location.protocol !== "file:") {
  const base = document.documentElement.dataset.root ?? "/";
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${base}sw.js`, { scope: base }).catch(() => {});
  });
}
