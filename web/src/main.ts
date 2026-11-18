import "./styles.css";
import { initBack } from "./ui/back";
import { initIndex } from "./ui/index";
import { initLang } from "./ui/lang";
import { initListPage, updateCount } from "./ui/list";
import { initRecipe } from "./ui/recipe";
import { initTheme } from "./ui/theme";
import { initTimers } from "./ui/timers";

initLang();
initBack();
initTheme();
initTimers();
initIndex();
initRecipe();
initListPage();
updateCount();

if ("serviceWorker" in navigator && location.protocol !== "file:") {
  const base = document.documentElement.dataset.root ?? "/";
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${base}sw.js`, { scope: base }).catch(() => {});
  });
}
