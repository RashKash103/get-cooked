// UI strings injected by the site builder (<script id="i18n">).
let dict: Record<string, string> | null = null;

export function t(key: string, vars: Record<string, string | number> = {}): string {
  if (dict === null) {
    try {
      dict = JSON.parse(document.getElementById("i18n")?.textContent || "{}") as Record<string, string>;
    } catch {
      dict = {};
    }
  }
  let s = dict[key] ?? key;
  for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
  return s;
}

/** Sets the dictionary directly (tests). */
export function setDict(d: Record<string, string>): void {
  dict = d;
}
