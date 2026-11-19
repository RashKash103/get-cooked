// Kitchen timers: persisted as absolute end times so they survive reloads and
// backgrounded tabs, with sound, vibration and notifications when they finish.
import { duration } from "../runtime/units";
import { num, t as tr } from "./i18n";
import { native } from "./native";
import { load, save } from "./storage";
import { toast } from "./toast";

interface Timer {
  id: string;
  label: string;
  total: number; // ms
  endsAt: number; // epoch ms, when running
  left: number; // ms remaining, when paused
  paused: boolean;
  done: boolean;
  from?: TimerFrom;
}

/** Where a timer was started: the recipe page (with its settings) and step. */
export interface TimerFrom {
  url: string;
  step: number;
}

const KEY = "gc:timers";

// Duration words come from the recipe's unit table when on a recipe page.
function words(): Record<string, string> | undefined {
  try {
    const d = JSON.parse(document.getElementById("recipe-data")?.textContent || "null");
    return d?.compiled?.units?.words;
  } catch {
    return undefined;
  }
}
let timers: Timer[] = load<Timer[]>(KEY, []);
let interval: ReturnType<typeof setInterval> | undefined;
let audio: AudioContext | undefined;
let alarm: ReturnType<typeof setInterval> | undefined;
const baseTitle = document.title;

function persist(): void {
  save(KEY, timers.length ? timers : undefined);
}

function remaining(t: Timer): number {
  return t.paused ? t.left : Math.max(0, t.endsAt - Date.now());
}

function clock(ms: number): string {
  const s = Math.ceil(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const mm = h > 0 ? String(m).padStart(2, "0") : String(m);
  return num(`${h > 0 ? `${h}:` : ""}${mm}:${String(sec).padStart(2, "0")}`);
}

const isAndroid = /Android/i.test(navigator.userAgent);
const isIOS = /iPhone|iPad|iPod/i.test(navigator.userAgent);

function clockAppURL(t: Timer): string | null {
  const html = document.documentElement;
  const seconds = Math.round(t.total / 1000);
  if (isAndroid && html.dataset.timerIntent === "true") {
    const msg = encodeURIComponent(t.label);
    return `intent:#Intent;action=android.intent.action.SET_TIMER;i.android.intent.extra.alarm.LENGTH=${seconds};S.android.intent.extra.alarm.MESSAGE=${msg};B.android.intent.extra.alarm.SKIP_UI=true;end`;
  }
  const shortcut = html.dataset.iosShortcut;
  if (isIOS && shortcut) {
    return `shortcuts://run-shortcut?name=${encodeURIComponent(shortcut)}&input=text&text=${seconds}`;
  }
  return null;
}

let expanded = false;

/** The timer to show when several are collapsed: a finished one, else the next to finish. */
export function leadTimer<T extends { done: boolean; paused: boolean; endsAt: number; left: number }>(
  ts: T[],
  now: number,
): T | undefined {
  const left = (t: T) => (t.paused ? t.left : t.endsAt - now);
  return ts.find((t) => t.done) ?? [...ts].sort((a, b) => Number(a.paused) - Number(b.paused) || left(a) - left(b))[0];
}

function render(): void {
  const root = document.getElementById("timers");
  if (!root) return;
  // More than one timer collapses to the most urgent, so they don't cover the recipe.
  if (timers.length < 2) expanded = false;
  const lead = leadTimer(timers, Date.now());
  const shown = timers.length > 1 && !expanded && lead ? [lead] : timers;
  const more =
    timers.length > 1
      ? `<button type="button" class="timers-more" data-act="expand" aria-expanded="${expanded}">${expanded ? tr("timers_less") : tr("timers_more", { n: num(timers.length - 1) })}</button>`
      : "";
  root.innerHTML =
    shown
      .map((t) => {
        const app = clockAppURL(t);
        return `<div class="timer${t.done ? " done" : ""}" data-timer="${t.id}" role="timer">
  <div class="timer-info"${t.from?.step ? ` data-act="go" role="link" tabindex="0" title="${tr("timer_go_to_step")}"` : ""}><div class="timer-left">${t.done ? tr("timer_done_short") : clock(remaining(t))}</div><div class="timer-label">${t.label.replace(/[<>&]/g, "")}</div></div>
  ${app && !t.done ? `<a class="icon-btn" href="${app}" aria-label="${tr("timer_clock_app")}" title="${tr("timer_clock_app")}"><svg class="icon" aria-hidden="true"><use href="#i-bell"></use></svg></a>` : ""}
  ${t.done ? "" : `<button type="button" class="icon-btn" data-act="add" aria-label="${tr("timer_add")}" title="${tr("timer_add")}"><svg class="icon" aria-hidden="true"><use href="#i-plus"></use></svg></button>`}
  ${t.done ? "" : `<button type="button" class="icon-btn" data-act="toggle" aria-label="${t.paused ? tr("timer_resume") : tr("timer_pause")}"><svg class="icon" aria-hidden="true"><use href="#i-${t.paused ? "play" : "pause"}"></use></svg></button>`}
  <button type="button" class="icon-btn" data-act="close" aria-label="${t.done ? tr("timer_dismiss") : tr("timer_cancel")}"><svg class="icon" aria-hidden="true"><use href="#i-x"></use></svg></button>
</div>`;
      })
      .join("") + more;
}

function goToStep(f: TimerFrom): void {
  const step = document.querySelector<HTMLElement>(`.step[data-n="${f.step}"]`);
  if (step && location.pathname === new URL(f.url, location.href).pathname) {
    step.scrollIntoView({ behavior: "smooth", block: "center" });
    step.classList.add("flash");
    setTimeout(() => step.classList.remove("flash"), 1200);
  } else {
    location.href = `${f.url}#step-${f.step}`;
  }
}

function beep(): void {
  try {
    audio ??= new AudioContext();
    const ctx = audio;
    for (let i = 0; i < 3; i++) {
      const o = ctx.createOscillator();
      const g = ctx.createGain();
      o.type = "sine";
      o.frequency.value = 880;
      const t0 = ctx.currentTime + i * 0.25;
      g.gain.setValueAtTime(0.0001, t0);
      g.gain.exponentialRampToValueAtTime(0.4, t0 + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + 0.2);
      o.connect(g).connect(ctx.destination);
      o.start(t0);
      o.stop(t0 + 0.22);
    }
  } catch {
    // audio unavailable
  }
}

async function notify(t: Timer): Promise<void> {
  if (!("Notification" in window) || Notification.permission !== "granted") return;
  const opts = {
    body: t.label,
    tag: `gc-timer-${t.id}`,
    renotify: true,
    requireInteraction: true,
  } as NotificationOptions;
  try {
    const reg = await navigator.serviceWorker?.getRegistration();
    if (reg) await reg.showNotification(tr("timer_done"), opts);
    else new Notification(tr("timer_done"), opts);
  } catch {
    // ignore
  }
}

function ring(t: Timer): void {
  t.done = true;
  beep();
  navigator.vibrate?.([300, 150, 300, 150, 600]);
  void notify(t);
  clearInterval(alarm);
  let n = 0;
  alarm = setInterval(() => {
    if (!timers.some((x) => x.done) || ++n > 10) {
      clearInterval(alarm);
      document.title = baseTitle;
      return;
    }
    beep();
    document.title = n % 2 ? `⏰ ${tr("timer_done")}` : baseTitle;
  }, 3000);
}

function tick(): void {
  let changed = false;
  for (const t of timers) {
    if (!t.done && !t.paused && remaining(t) <= 0) {
      ring(t);
      changed = true;
    }
  }
  if (changed) persist();
  render();
  if (!timers.some((t) => !t.done && !t.paused)) {
    clearInterval(interval);
    interval = undefined;
  }
}

function ensureTicking(): void {
  interval ??= setInterval(tick, 250);
  tick();
}

export function startTimer(minutes: number, label: string, from?: TimerFrom): void {
  const seconds = Math.round(minutes * 60);
  const n = native();
  if (n) {
    n.setTimer(seconds, label);
    toast(tr("timer_set", { time: num(duration(minutes, words())) }));
    return;
  }
  // Ask once, from this user gesture, so finished timers can notify.
  if ("Notification" in window && Notification.permission === "default") {
    void Notification.requestPermission();
  }
  // Unlock audio on iOS/Safari from the tap.
  try {
    audio ??= new AudioContext();
    void audio.resume();
  } catch {
    // ignore
  }
  const total = seconds * 1000;
  timers.push({
    id: Math.random().toString(36).slice(2, 9),
    label,
    total,
    endsAt: Date.now() + total,
    left: total,
    paused: false,
    done: false,
    from,
  });
  persist();
  ensureTicking();
  toast(tr("timer_started", { time: num(duration(minutes, words())) }));
}

export function initTimers(): void {
  document.getElementById("timers")?.addEventListener("click", (e) => {
    const btn = (e.target as Element).closest<HTMLElement>("[data-act]");
    if (btn?.dataset.act === "expand") {
      expanded = !expanded;
      render();
      return;
    }
    const el = btn?.closest<HTMLElement>("[data-timer]");
    const t = timers.find((x) => x.id === el?.dataset.timer);
    if (!btn || !t) return;
    switch (btn.dataset.act) {
      case "go":
        if (t.from) goToStep(t.from);
        return;
      case "close":
        timers = timers.filter((x) => x !== t);
        break;
      case "add":
        if (t.paused) t.left += 60_000;
        else t.endsAt += 60_000;
        t.total += 60_000;
        break;
      case "toggle":
        if (t.paused) {
          t.endsAt = Date.now() + t.left;
          t.paused = false;
        } else {
          t.left = remaining(t);
          t.paused = true;
        }
        break;
    }
    persist();
    ensureTicking();
  });
  // Catch up immediately when returning to the tab.
  document.getElementById("timers")?.addEventListener("keydown", (e) => {
    const el = (e.target as Element).closest<HTMLElement>('[data-act="go"]');
    if (el && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      el.click();
    }
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && timers.length) ensureTicking();
  });
  if (timers.length) ensureTicking();
}

/**
 * Picker steps for a time range: 15 s for short timers (under 10 min, where
 * durations show seconds), 1 min for spans up to an hour, else 5 min.
 */
export function rangeStep(loMin: number, hiMin: number): number {
  const span = (hiMin - loMin) * 60;
  if (hiMin <= 10 && span <= 600) return 15;
  if (span <= 3600) return 60;
  return 300;
}

/** Quick choices for a range: start, 25%, 50%, 75% and end (in seconds, deduped). */
export function rangeChoices(loMin: number, hiMin: number): number[] {
  const step = rangeStep(loMin, hiMin);
  const lo = Math.round(loMin * 60);
  const hi = Math.round(hiMin * 60);
  const out: number[] = [];
  for (const f of [0, 0.25, 0.5, 0.75, 1]) {
    const s = f === 0 ? lo : f === 1 ? hi : Math.round((lo + (hi - lo) * f) / step) * step;
    if (!out.includes(s)) out.push(s);
  }
  return out;
}

let picker: HTMLDialogElement | null = null;

/** Opens the range picker for a "4–5 min" timer and starts the chosen time. */
export function pickTimer(loMin: number, hiMin: number, label: string, from?: TimerFrom): void {
  const fmt = (sec: number) => num(duration(sec / 60, words()));
  const choices = rangeChoices(loMin, hiMin);
  const step = rangeStep(loMin, hiMin);
  const lo = Math.round(loMin * 60);
  const hi = Math.round(hiMin * 60);
  let value = choices[Math.floor(choices.length / 2)] as number;
  if (!picker) {
    picker = document.createElement("dialog");
    picker.className = "sheet timer-picker";
    picker.setAttribute("aria-labelledby", "timer-picker-title");
    document.body.append(picker);
    picker.addEventListener("click", (e) => {
      if (e.target === picker) picker?.close();
    });
  }
  const p = picker;
  p.innerHTML = `<form method="dialog" class="sheet-inner">
  <div class="sheet-head"><h2 id="timer-picker-title">${tr("timer_pick")}</h2>
    <button class="icon-btn" value="close" aria-label="${tr("timer_cancel")}"><svg class="icon" aria-hidden="true"><use href="#i-x"></use></svg></button></div>
  <p class="timer-picker-label">${label.replace(/[<>&]/g, "")}</p>
  <div class="timer-choices">${choices
    .map((s) => `<button type="button" class="chip" data-sec="${s}" aria-pressed="false">${fmt(s)}</button>`)
    .join("")}</div>
  <label class="opt-number"><span class="opt-label"><output id="timer-picker-value"></output></span>
    <input type="range" id="timer-picker-range" min="${lo}" max="${hi}" step="${step}" aria-label="${tr("timer_pick")}"></label>
  <button type="button" class="start-btn" id="timer-picker-start"><svg class="icon" aria-hidden="true"><use href="#i-play"></use></svg>${tr("timer_start")}</button>
</form>`;
  const range = p.querySelector<HTMLInputElement>("#timer-picker-range") as HTMLInputElement;
  const out = p.querySelector("#timer-picker-value") as HTMLOutputElement;
  const paint = () => {
    range.value = String(value);
    out.textContent = fmt(value);
    for (const b of p.querySelectorAll<HTMLElement>("[data-sec]")) {
      b.setAttribute("aria-pressed", String(Number(b.dataset.sec) === value));
    }
  };
  p.querySelector(".timer-choices")?.addEventListener("click", (e) => {
    const b = (e.target as Element).closest<HTMLElement>("[data-sec]");
    if (b) {
      value = Number(b.dataset.sec);
      paint();
    }
  });
  range.addEventListener("input", () => {
    value = Number(range.value);
    paint();
  });
  p.querySelector("#timer-picker-start")?.addEventListener("click", () => {
    p.close();
    startTimer(value / 60, label, from);
  });
  paint();
  p.showModal();
}
