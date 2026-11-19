// Small bottom sheets (dialogs) built on demand: the swap menu and the
// conversions sheet. The timer range picker uses the same look.
import { esc } from "./blocks";
import { t } from "./i18n";

const sheets = new Map<string, HTMLDialogElement>();

/** Opens (creating once) a bottom sheet with a title and body HTML. */
export function openSheet(id: string, title: string, body: string): HTMLDialogElement {
  let d = sheets.get(id);
  if (!d) {
    d = document.createElement("dialog");
    d.className = `sheet ${id}`;
    d.setAttribute("aria-labelledby", `${id}-title`);
    document.body.append(d);
    const dd = d;
    dd.addEventListener("click", (e) => {
      if (e.target === dd) dd.close();
    });
    sheets.set(id, d);
  }
  d.innerHTML = `<form method="dialog" class="sheet-inner">
  <div class="sheet-head"><h2 id="${id}-title">${esc(title)}</h2>
    <button class="icon-btn" value="close" aria-label="${esc(t("close"))}"><svg class="icon" aria-hidden="true"><use href="#i-x"></use></svg></button></div>
  ${body}
</form>`;
  d.showModal();
  // Focus the sheet itself, so the close button doesn't light up on open.
  d.tabIndex = -1;
  d.focus();
  return d;
}
