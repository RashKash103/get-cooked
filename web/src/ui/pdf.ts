// The PDF viewer page (pdf/?f=<file>&p=<page>&r=<recipe slug>&t=<title>).
// pdf.js is large, so it loads only here, from a lazy chunk.
import { asciiDigits, num, t } from "./i18n";

const FILE = /^[\w.-]+\.pdf$/;
const ZOOMS = [1, 1.25, 1.5, 2, 2.5, 3];
// iOS refuses canvases over ~16.7M pixels.
const MAX_PIXELS = 16_000_000;
const MAX_WIDTH = 820;

export function initPdf(): void {
  const root = document.getElementById("pdf-page");
  if (!root) return;
  void run(root);
}

async function run(root: HTMLElement): Promise<void> {
  const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
  const status = $<HTMLParagraphElement>("pdf-status");
  const canvas = $<HTMLCanvasElement>("pdf-canvas");
  const stage = $<HTMLDivElement>("pdf-stage");
  const input = $<HTMLInputElement>("pdf-num");
  const prev = $<HTMLButtonElement>("pdf-prev");
  const next = $<HTMLButtonElement>("pdf-next");
  const fail = () => {
    status.hidden = false;
    status.textContent = t("pdf_failed");
  };

  const q = new URLSearchParams(location.search);
  const file = q.get("f") ?? "";
  if (!FILE.test(file)) return fail();
  const title = q.get("t") || file;
  const slug = q.get("r") ?? "";
  const url = (root.dataset.dir ?? "") + file;
  let page = Math.max(1, Number.parseInt(q.get("p") ?? "1", 10) || 1);
  let zoom = 0;

  $("pdf-title").textContent = title;
  document.title = document.title.replace(/^PDF/, title);
  const open = $<HTMLAnchorElement>("pdf-open");
  open.target = "_blank";
  open.rel = "noopener";
  open.hidden = false;

  // The top-left link leads back to the recipe (in history when we came from it).
  const back = document.getElementById("back") as HTMLAnchorElement | null;
  if (back && /^[^/?#]+$/.test(slug)) {
    back.href = `${document.documentElement.dataset.base ?? "/"}recipes/${encodeURIComponent(slug)}/`;
    const label = back.querySelector("span");
    if (label) label.textContent = t("pdf_back");
    const fromRecipe =
      document.referrer && new URL(document.referrer, location.href).pathname === new URL(back.href).pathname;
    if (fromRecipe && history.length > 1)
      back.addEventListener("click", (e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        history.back();
      });
  }

  let doc: Awaited<ReturnType<typeof import("./pdfjs").openPDF>>;
  try {
    const { openPDF } = await import("./pdfjs");
    doc = await openPDF(url);
  } catch {
    return fail();
  }
  const total = doc.numPages;
  page = Math.min(page, total);
  $("pdf-total").textContent = ` ${t("pdf_of", { total: num(total) })}`;

  let task: { cancel(): void; promise: Promise<unknown> } | null = null;
  let seq = 0;
  async function render(): Promise<void> {
    const mine = ++seq;
    task?.cancel();
    input.value = num(page);
    prev.disabled = page <= 1;
    next.disabled = page >= total;
    open.href = `${url}#page=${page}`;
    const u = new URL(location.href);
    u.searchParams.set("p", String(page));
    history.replaceState(history.state, "", u);
    try {
      const p = await doc.getPage(page);
      if (mine !== seq) return;
      const unit = p.getViewport({ scale: 1 });
      // Fit to width, but no wider than a comfortable reading width on big screens.
      const width = Math.min(stage.clientWidth, MAX_WIDTH) * (ZOOMS[zoom] ?? 1);
      let scale = (width / unit.width) * (window.devicePixelRatio || 1);
      const pixels = unit.width * unit.height * scale * scale;
      if (pixels > MAX_PIXELS) scale *= Math.sqrt(MAX_PIXELS / pixels);
      const viewport = p.getViewport({ scale });
      // Draw off-screen, then swap, so the old page stays up while the new one renders.
      const off = document.createElement("canvas");
      off.width = Math.floor(viewport.width);
      off.height = Math.floor(viewport.height);
      task = p.render({ canvas: off, viewport });
      await task.promise;
      if (mine !== seq) return;
      canvas.width = off.width;
      canvas.height = off.height;
      canvas.getContext("2d")?.drawImage(off, 0, 0);
      canvas.style.width = `${width}px`;
      canvas.setAttribute("role", "img");
      canvas.setAttribute("aria-label", `${title} · ${num(page)} / ${num(total)}`);
      canvas.hidden = false;
      status.hidden = true;
    } catch (e) {
      if ((e as { name?: string })?.name !== "RenderingCancelledException") fail();
    }
  }
  const go = (n: number) => {
    const to = Math.min(Math.max(1, n), total);
    if (to === page) {
      input.value = num(page);
      return;
    }
    page = to;
    stage.scrollTo({ top: 0, left: 0 });
    window.scrollTo({ top: 0 });
    void render();
  };
  prev.addEventListener("click", () => go(page - 1));
  next.addEventListener("click", () => go(page + 1));
  input.addEventListener("change", () => go(Number.parseInt(asciiDigits(input.value), 10) || page));
  input.addEventListener("focus", () => input.select());
  const zoomBy = (d: number) => {
    zoom = Math.min(Math.max(0, zoom + d), ZOOMS.length - 1);
    void render();
  };
  $("pdf-zoom-in").addEventListener("click", () => zoomBy(1));
  $("pdf-zoom-out").addEventListener("click", () => zoomBy(-1));
  document.addEventListener("keydown", (e) => {
    if (e.metaKey || e.ctrlKey || e.altKey || (e.target as HTMLElement).matches("input, textarea, select")) return;
    const rtl = document.documentElement.dir === "rtl";
    if (e.key === "PageDown" || e.key === (rtl ? "ArrowLeft" : "ArrowRight")) go(page + 1);
    else if (e.key === "PageUp" || e.key === (rtl ? "ArrowRight" : "ArrowLeft")) go(page - 1);
    else if (e.key === "+" || e.key === "=") zoomBy(1);
    else if (e.key === "-") zoomBy(-1);
    else return;
    e.preventDefault();
  });
  let resize = 0;
  let lastWidth = stage.clientWidth;
  window.addEventListener("resize", () => {
    clearTimeout(resize);
    resize = window.setTimeout(() => {
      if (stage.clientWidth !== lastWidth) {
        lastWidth = stage.clientWidth;
        void render();
      }
    }, 150);
  });
  await render();
}
