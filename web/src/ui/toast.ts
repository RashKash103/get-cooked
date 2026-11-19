let timer: ReturnType<typeof setTimeout> | undefined;

export function toast(message: string, ms = 2600): void {
  const el = document.getElementById("toast");
  if (!el) return;
  el.textContent = message;
  // Sit above any running timers instead of covering them.
  const timers = document.getElementById("timers");
  const top = timers?.childElementCount ? timers.getBoundingClientRect().top : 0;
  el.style.bottom = top > 0 ? `${Math.round(window.innerHeight - top + 8)}px` : "";
  el.hidden = false;
  clearTimeout(timer);
  timer = setTimeout(() => {
    el.hidden = true;
  }, ms);
}
