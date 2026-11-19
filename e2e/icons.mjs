// Renders the PWA icon PNGs from web/public/icon.svg (run: make icons).
// - icon-192.png / icon-512.png: the icon as-is ("any" purpose)
// - icon-maskable-512.png: artwork at 80% on a full-bleed background so
//   Android's adaptive-icon mask never clips it
// - apple-touch-icon.png (180): opaque, full-bleed (iOS rounds the corners)
import { readFileSync } from "node:fs";
import { chromium } from "@playwright/test";

const pub = new URL("../web/public/", import.meta.url).pathname;
const svg = readFileSync(`${pub}icon.svg`, "utf8");
const bg = "#ea580c";
const b = await chromium.launch();
const page = await b.newPage();
async function render(file, size, scale, background) {
  await page.setViewportSize({ width: size, height: size });
  const inner = Math.round(size * scale);
  await page.setContent(
    `<html><body style="margin:0;width:${size}px;height:${size}px;display:grid;place-items:center;background:${background}">` +
      `<div style="width:${inner}px;height:${inner}px">${svg.replace("<svg ", `<svg width="${inner}" height="${inner}" `)}</div></body></html>`,
  );
  await page.screenshot({ path: `${pub}${file}`, omitBackground: background === "transparent" });
}
await render("icon-192.png", 192, 1, "transparent");
await render("icon-512.png", 512, 1, "transparent");
await render("icon-maskable-512.png", 512, 0.8, bg);
await render("apple-touch-icon.png", 180, 1, bg);
await b.close();
console.log("icons written to web/public");
