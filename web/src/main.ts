import "./styles.css";
import { initIndex } from "./ui/index";
import { initRecipe } from "./ui/recipe";
import { initTheme } from "./ui/theme";
import { initTimers } from "./ui/timers";

initTheme();
initTimers();
initIndex();
initRecipe();

if ("serviceWorker" in navigator && location.protocol !== "file:") {
  const base = document.documentElement.dataset.base ?? "/";
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${base}sw.js`, { scope: base }).catch(() => {});
  });
}
