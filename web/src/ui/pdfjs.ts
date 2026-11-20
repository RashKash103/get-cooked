// pdf.js, loaded lazily by the PDF viewer page. The worker, WebAssembly image
// decoders (for scanned books) and standard fonts sit next to this chunk.
import { GlobalWorkerOptions, getDocument, version } from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";

GlobalWorkerOptions.workerSrc = workerUrl;

export function openPDF(url: string) {
  // Built by hand: Vite rewrites `new URL(template, import.meta.url)` into a glob.
  const res = `${import.meta.url.replace(/[^/]*$/, "")}pdfjs-${version}/`;
  return getDocument({
    url,
    wasmUrl: `${res}wasm/`,
    standardFontDataUrl: `${res}standard_fonts/`,
    // Fetch only the byte ranges the shown page needs (64 KB chunks), never
    // the whole file: cookbook scans can be hundreds of megabytes.
    disableAutoFetch: true,
    disableStream: true,
  }).promise;
}
