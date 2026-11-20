import { readdirSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

// pdf.js support files the PDF viewer loads at runtime: WebAssembly image
// decoders (JBIG2 and JPEG 2000, common in scanned books, with their JS
// fallbacks) and the standard fonts for PDFs that don't embed them.
function pdfjsFiles(): Plugin {
  const require = createRequire(import.meta.url);
  const root = dirname(require.resolve("pdfjs-dist/package.json"));
  const { version } = JSON.parse(readFileSync(join(root, "package.json"), "utf8")) as { version: string };
  const pick: Record<string, (f: string) => boolean> = {
    wasm: (f) => /\.(wasm|js)$/.test(f) && !f.startsWith("quickjs"),
    standard_fonts: (f) => /\.(pfb|ttf)$|^LICENSE/.test(f),
  };
  return {
    name: "pdfjs-files",
    generateBundle() {
      for (const [dir, keep] of Object.entries(pick)) {
        for (const f of readdirSync(join(root, dir)).filter(keep)) {
          this.emitFile({
            type: "asset",
            fileName: `lazy/pdfjs-${version}/${dir}/${f}`,
            source: readFileSync(join(root, dir, f)),
          });
        }
      }
    },
  };
}

// The bundle is embedded into the Go binary (internal/assets/dist) with stable
// entry names; the site builder fingerprints them when writing a site.
// Lazily loaded code (the PDF viewer) goes to lazy/ with hashed names and is
// copied as is, next to the fingerprinted app.js that imports it.
export default defineConfig({
  base: "./",
  plugins: [tailwindcss(), pdfjsFiles()],
  build: {
    outDir: resolve(import.meta.dirname, "../internal/assets/dist"),
    emptyOutDir: true,
    target: "es2022",
    cssCodeSplit: false,
    rollupOptions: {
      input: {
        app: resolve(import.meta.dirname, "src/main.ts"),
        sw: resolve(import.meta.dirname, "src/sw.ts"),
      },
      output: {
        entryFileNames: "[name].js",
        chunkFileNames: "lazy/[name]-[hash].js",
        assetFileNames: (info) => {
          const name = info.names?.[0] ?? "";
          if (name.endsWith(".css")) return "app.css";
          // .js, not .mjs: servers without an .mjs MIME type would break the worker.
          if (name.endsWith(".mjs")) return "lazy/[name]-[hash].js";
          return "[name][extname]";
        },
      },
    },
  },
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
  },
});
