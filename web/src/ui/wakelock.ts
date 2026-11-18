import { native } from "./native";

let sentinel: WakeLockSentinel | null = null;
let wanted = false;

async function acquire(): Promise<boolean> {
  const n = native();
  if (n?.keepScreenOn) {
    n.keepScreenOn(true);
    return true;
  }
  if (!("wakeLock" in navigator)) return false;
  try {
    sentinel = await navigator.wakeLock.request("screen");
    sentinel.addEventListener("release", () => {
      sentinel = null;
    });
    return true;
  } catch {
    return false;
  }
}

/** Keeps the screen awake while `on`. Resolves false if unsupported. */
export async function keepAwake(on: boolean): Promise<boolean> {
  wanted = on;
  if (on) return acquire();
  native()?.keepScreenOn?.(false);
  await sentinel?.release().catch(() => {});
  sentinel = null;
  return true;
}

// The lock is dropped whenever the page is hidden; take it back on return.
document.addEventListener("visibilitychange", () => {
  if (wanted && document.visibilityState === "visible" && !sentinel) void acquire();
});
