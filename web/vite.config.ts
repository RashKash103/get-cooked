import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vitest/config";

// The bundle is embedded into the Go binary (internal/assets/dist) with stable
// file names; the site builder fingerprints them when writing a site.
export default defineConfig({
  plugins: [tailwindcss()],
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
        chunkFileNames: "[name].js",
        assetFileNames: (info) => (info.names?.[0]?.endsWith(".css") ? "app.css" : "[name][extname]"),
      },
    },
  },
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
  },
});
