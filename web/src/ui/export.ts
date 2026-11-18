// Mirror of internal/export Markdown(): the browser's "Download Markdown"
// matches `get-cooked export -f md` byte for byte (tested via goldens).
import type { Compiled, IngM, Model, Seg, Value } from "../runtime/types";
import { qtyString } from "../runtime/units";
import { t } from "./i18n";

export interface Meta {
  title: string;
  description?: string;
  source?: { name?: string; url?: string } | null;
}

export function summary(c: Compiled, m: Model, opts: Record<string, Value>): string {
  const parts: string[] = [];
  for (const o of c.options ?? []) {
    if (!m.options.includes(o.id)) continue;
    const v = o.id in opts ? opts[o.id] : o.default;
    if (o.type === "toggle") {
      if (v === true) parts.push(o.label);
    } else if (o.type === "choice") {
      const ch = o.choices?.find((x) => x.id === v);
      if (ch) parts.push(`${o.label}: ${ch.label}`);
    } else {
      parts.push(`${o.label}: ${String(Math.round(Number(v) * 100) / 100)}${o.unit ?? ""}`);
    }
  }
  if (m.system !== "original") parts.push(`${t("units")}: ${t(`units_${m.system}`)}`);
  return parts.join(" · ");
}

export function stepText(segs: Seg[]): string {
  return segs.map((s) => (s.t === "ing" && s.q !== "" ? s.q + s.j : "") + s.v).join("");
}

function ingLine(it: IngM): string {
  let s = it.name;
  if (it.q) s = qtyString(it.q) + it.j + s;
  if (it.note) s += `, ${it.note}`;
  return s;
}

function uses(it: IngM): string {
  return it.uses.map((u) => t("uses", { q: u.q, n: u.n })).join(" · ");
}

export function toMarkdown(meta: Meta, c: Compiled, m: Model, opts: Record<string, Value>): string {
  let b = `# ${meta.title}\n\n`;
  if (meta.description) b += `${meta.description}\n\n`;
  b += `**${[m.yield, ...m.times.map((x) => `${x.label} ${x.text}`)].join(" · ")}**\n\n`;
  const sum = summary(c, m, opts);
  if (sum) b += `*${sum}*\n\n`;
  for (const n of m.notes) if (n.kind === "warning") b += `> ⚠ ${n.text}\n\n`;
  b += `## ${t("ingredients")}\n\n`;
  for (const s of m.ingredients) {
    if (s.title) b += `### ${s.title}\n\n`;
    for (const it of s.items) {
      const u = uses(it);
      b += `- ${ingLine(it)}${u ? ` (${u})` : ""}\n`;
    }
    b += "\n";
  }
  if (m.equipment.length > 0) {
    b += `## ${t("equipment")}\n\n`;
    for (const e of m.equipment) b += `- ${e.name}${e.note ? `, ${e.note}` : ""}\n`;
    b += "\n";
  }
  b += `## ${t("steps")}\n\n`;
  for (const s of m.steps) {
    if (s.title) b += `### ${s.title}\n\n`;
    for (const st of s.items) {
      b += `${st.n}. ${stepText(st.segs)}\n`;
      if (st.note) b += `   *${st.note}*\n`;
    }
    b += "\n";
  }
  const tips = m.notes.filter((n) => n.kind !== "warning");
  if (tips.length > 0) {
    b += `## ${t("notes")}\n\n`;
    for (const n of tips) b += `- ${n.text}\n`;
    b += "\n";
  }
  const src = meta.source;
  if (src && (src.name || src.url)) {
    const name = src.name || src.url;
    b += src.url ? `${t("source")}: [${name}](${src.url})\n` : `${t("source")}: ${name}\n`;
  }
  return `${b.replace(/\n+$/, "")}\n`;
}

export function download(name: string, text: string, type = "text/markdown"): void {
  const url = URL.createObjectURL(new Blob([text], { type: `${type};charset=utf-8` }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
