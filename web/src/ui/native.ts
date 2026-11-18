// Optional bridge for a native wrapper app (e.g. an Android WebView/TWA).
// If the host injects `window.GetCookedNative`, timers and screen wake are
// delegated to the OS.
export interface GetCookedNative {
  setTimer(seconds: number, label: string): void;
  keepScreenOn?(on: boolean): void;
}

declare global {
  interface Window {
    GetCookedNative?: GetCookedNative;
  }
}

export function native(): GetCookedNative | undefined {
  return typeof window !== "undefined" ? window.GetCookedNative : undefined;
}
