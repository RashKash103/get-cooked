// The top-left "All recipes" control on recipe and list pages. It always
// leads to the recipe index in the page's language. When the previous page
// was that same index, it goes back in history instead, so the reader's
// search, filters and scroll position are kept.
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
  const fromIndex =
    ref !== null && ref.origin === location.origin && ref.pathname === index.pathname && history.length > 1;
  if (!fromIndex) return; // plain link to this language's index
  link.addEventListener("click", (e) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    history.back();
  });
}
