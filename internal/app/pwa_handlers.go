package app

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/amon2126/nutreek/internal/web"
)

// App installabile (#59): manifest e service worker. Entrambi alla radice:
// il worker controlla solo i percorsi sotto il suo, e deve coprire tutta l'app.

type manifestIconOut struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// handleManifest GET /manifest.webmanifest — colore e icone seguono il tema
// dell'utente (cookie tema_colore), lo sfondo d'avvio il modo scuro.
func (a *App) handleManifest(w http.ResponseWriter, r *http.Request) {
	ap := readAppearance(r)
	background := "#F7F8FA" // --bg chiaro
	if ap.Mode == "scuro" {
		background = "#16181C" // --bg scuro
	}
	icon := func(kind, size, purpose string) manifestIconOut {
		return manifestIconOut{
			Src:     fmt.Sprintf("/static/icons/%s-%s-%s.png", kind, ap.Accent, size),
			Sizes:   size + "x" + size,
			Type:    "image/png",
			Purpose: purpose,
		}
	}
	m := map[string]any{
		"id":               "/",
		"name":             "Nutreek",
		"short_name":       "Nutreek",
		"description":      "Piani alimentari settimanali, macro e lista della spesa.",
		"lang":             "it",
		"dir":              "ltr",
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"theme_color":      web.AccentColor(ap.Accent),
		"background_color": background,
		"icons": []manifestIconOut{
			icon("icon", "192", "any"),
			icon("icon", "512", "any"),
			icon("maskable", "512", "maskable"),
		},
	}
	h := w.Header()
	h.Set("Content-Type", "application/manifest+json; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Vary", "Cookie")
	_ = json.NewEncoder(w).Encode(m)
}

// serviceWorkerJS: cache solo per /static (CSS, JS, font, icone), servito
// dalla cache e aggiornato quando cambia la versione. Pagine, API e POST
// vanno sempre in rete: contengono i dati dell'utente e devono essere freschi.
const serviceWorkerJS = `// Nutreek — service worker (#59), generato dal server.
"use strict";
const CACHE = %q;

self.addEventListener("install", () => self.skipWaiting());

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys
        .filter((k) => k.startsWith("nutreek-static-") && k !== CACHE)
        .map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith("/static/")) return;
  event.respondWith(
    caches.open(CACHE).then((cache) =>
      cache.match(req).then((hit) => hit || fetch(req).then((res) => {
        if (res.ok) cache.put(req, res.clone());
        return res;
      }))
    )
  );
});
`

// handleServiceWorker GET /sw.js.
func (a *App) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, serviceWorkerJS, "nutreek-static-"+web.StaticVersion())
}
