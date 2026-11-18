// Service worker: offline support for the kitchen.
// The site builder prepends `self.__GC = {version, base, precache}`.
const sw = self as unknown as ServiceWorkerGlobalScope & {
  __GC?: { version: string; base: string; precache: string[] };
};
const cfg = sw.__GC ?? { version: "dev", base: "/", precache: [] };
const VERSION = cfg.version;
const BASE = cfg.base;
const PRECACHE = cfg.precache;
const CACHE = `get-cooked-${VERSION}`;

sw.addEventListener("install", (e: ExtendableEvent) => {
  e.waitUntil(
    caches
      .open(CACHE)
      .then((c) => c.addAll(PRECACHE))
      .then(() => sw.skipWaiting()),
  );
});

sw.addEventListener("activate", (e: ExtendableEvent) => {
  e.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(keys.filter((k) => k.startsWith("get-cooked-") && k !== CACHE).map((k) => caches.delete(k))),
      )
      .then(() => sw.clients.claim()),
  );
});

sw.addEventListener("fetch", (e: FetchEvent) => {
  const req = e.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== location.origin || !url.pathname.startsWith(BASE)) return;
  if (url.pathname.endsWith("/__livereload")) return;
  // Fingerprinted assets: cache first. Pages and everything else: network first.
  if (url.pathname.startsWith(`${BASE}assets/`)) {
    e.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ??
          fetch(req).then((res) => {
            const copy = res.clone();
            void caches.open(CACHE).then((c) => c.put(req, copy));
            return res;
          }),
      ),
    );
    return;
  }
  e.respondWith(
    fetch(req)
      .then((res) => {
        if (res.ok) {
          const copy = res.clone();
          void caches.open(CACHE).then((c) => c.put(req, copy));
        }
        return res;
      })
      .catch(
        async () => (await caches.match(req, { ignoreSearch: true })) ?? (await caches.match(BASE)) ?? Response.error(),
      ),
  );
});

sw.addEventListener("notificationclick", (e: NotificationEvent) => {
  e.notification.close();
  e.waitUntil(
    sw.clients.matchAll({ type: "window" }).then((list) => {
      const c = list[0];
      return c ? c.focus() : sw.clients.openWindow(BASE);
    }),
  );
});
