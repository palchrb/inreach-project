const CACHE_NAME = "inreach-decoder-v1";

const iconList = [
  "01d","01n","01m","02d","02n","02m","03d","03n","03m","04",
  "05d","05n","05m","06d","06n","06m","07d","07n","07m","08d",
  "08n","08m","09","10","11","12","13","14","15","20d","20n",
  "20m","21d","21n","21m","22","23","24d","24n","24m","25d",
  "25n","25m","26d","26n","26m","27d","27n","27m","28d","28n",
  "28m","29d","29n","29m","30","31","32","33","34","40d","40n",
  "40m","41d","41n","41m","42d","42n","42m","43d","43n","43m",
  "44d","44n","44m","45d","45n","45m","46","47","48","49","50"
];

const baseResources = [
  "index.html", "avalanche.html", "cmd.html",
  "script.js", "script2.js", "manifest.json", "icon.png"
];

const iconResources = iconList.map(icon => `svg/${icon}.svg`);
const resourcesToCache = [...baseResources, ...iconResources];

// IndexedDB fallback
function openDatabase() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open("inreach-decoder-db", 1);
    request.onupgradeneeded = (e) => {
      if (!e.target.result.objectStoreNames.contains("files")) {
        e.target.result.createObjectStore("files");
      }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject("IndexedDB could not be opened.");
  });
}

async function saveToIndexedDB(url, response) {
  try {
    const db = await openDatabase();
    const blob = await response.blob();
    const tx = db.transaction("files", "readwrite");
    tx.objectStore("files").put(blob, url);
  } catch (err) {
    console.warn("IDB save failed:", url, err);
  }
}

async function getFromIndexedDB(url) {
  const db = await openDatabase();
  return new Promise((resolve, reject) => {
    const tx = db.transaction("files", "readonly");
    const request = tx.objectStore("files").get(url);
    request.onsuccess = () => {
      if (request.result) resolve(new Response(request.result));
      else reject("Not in IDB: " + url);
    };
    request.onerror = (e) => reject("IDB error: " + e);
  });
}

// Install: cache all resources
self.addEventListener("install", (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(CACHE_NAME);
    for (const url of resourcesToCache) {
      try {
        const response = await fetch(url);
        if (response.ok) {
          await saveToIndexedDB(url, response.clone());
          await cache.put(url, response.clone());
        }
      } catch (err) {
        console.warn("Failed to cache:", url, err);
      }
    }
  })());
  self.skipWaiting();
});

// Activate: clean old caches
self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then(names =>
      Promise.all(names.map(name => name !== CACHE_NAME ? caches.delete(name) : undefined))
    ).then(() => self.clients.claim())
  );
});

// Fetch: cache first, then network, then IndexedDB
self.addEventListener("fetch", (event) => {
  if (event.request.method !== "GET") return;
  event.respondWith(
    caches.match(event.request).then(cached => {
      if (cached) return cached;
      return fetch(event.request).then(async (response) => {
        if (response && response.ok && response.type === "basic") {
          saveToIndexedDB(event.request.url, response.clone()).catch(() => {});
          const cache = await caches.open(CACHE_NAME);
          cache.put(event.request, response.clone());
        }
        return response;
      }).catch(() => getFromIndexedDB(event.request.url).catch(() => {}));
    })
  );
});
