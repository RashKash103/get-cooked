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

function render(): void {
  const root = document.getElementById("timers");
  if (!root) return;
  root.innerHTML = timers
    .map((t) => {
      const app = clockAppURL(t);
      return `<div class="timer${t.done ? " done" : ""}" data-timer="${t.id}" role="timer">
  <div class="timer-info"><div class="timer-left">${t.done ? tr("timer_done_short") : clock(remaining(t))}</div><div class="timer-label">${t.label.replace(/[<>&]/g, "")}</div></div>
  ${app && !t.done ? `<a class="icon-btn" href="${app}" aria-label="${tr("timer_clock_app")}" title="${tr("timer_clock_app")}"><svg class="icon" aria-hidden="true"><use href="#i-bell"></use></svg></a>` : ""}
  ${t.done ? "" : `<button type="button" class="icon-btn" data-act="add" aria-label="${tr("timer_add")}" title="${tr("timer_add")}"><svg class="icon" aria-hidden="true"><use href="#i-plus"></use></svg></button>`}
  ${t.done ? "" : `<button type="button" class="icon-btn" data-act="toggle" aria-label="${t.paused ? tr("timer_resume") : tr("timer_pause")}"><svg class="icon" aria-hidden="true"><use href="#i-${t.paused ? "play" : "pause"}"></use></svg></button>`}
  <button type="button" class="icon-btn" data-act="close" aria-label="${t.done ? tr("timer_dismiss") : tr("timer_cancel")}"><svg class="icon" aria-hidden="true"><use href="#i-x"></use></svg></button>
</div>`;
    })
    .join("");
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

export function startTimer(minutes: number, label: string): void {
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
  });
  persist();
  ensureTicking();
  toast(tr("timer_started", { time: num(duration(minutes, words())) }));
}

export function initTimers(): void {
  document.getElementById("timers")?.addEventListener("click", (e) => {
    const btn = (e.target as Element).closest<HTMLElement>("[data-act]");
    const el = btn?.closest<HTMLElement>("[data-timer]");
    const t = timers.find((x) => x.id === el?.dataset.timer);
    if (!btn || !t) return;
    switch (btn.dataset.act) {
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
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && timers.length) ensureTicking();
  });
  if (timers.length) ensureTicking();
}
