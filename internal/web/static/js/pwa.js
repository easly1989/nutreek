// pwa.js — installable app (#59).
//
// Registers the service worker (/sw.js: caches only the files in /static)
// and, in Settings, shows the «App sul telefono» block right for the browser:
//   installata  opened from the icon on the Home screen
//   http        insecure context: no install and no camera
//   prompt      Chrome/Edge/Android: an «Installa l'app» button (beforeinstallprompt)
//   ios         iPhone/iPad: the steps in Safari, there is no prompt
//   altro       everything else (and without JS): install from the browser menu
(function () {
  "use strict";

  if (window.isSecureContext && "serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register("/sw.js").catch(function () { /* senza cache, l'app funziona lo stesso */ });
    });
  }

  var deferred = null; // the beforeinstallprompt event, to use on click

  function standalone() {
    return window.matchMedia("(display-mode: standalone)").matches || window.navigator.standalone === true;
  }
  function ios() {
    return /iPad|iPhone|iPod/.test(navigator.userAgent) ||
      (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1); // an iPad presenting itself as a Mac
  }

  function state() {
    if (standalone()) return "installata";
    if (!window.isSecureContext) return "http";
    if (deferred) return "prompt";
    if (ios()) return "ios";
    return "altro";
  }

  function render() {
    var box = document.querySelector("[data-pwa]");
    if (!box) return;
    var s = state();
    box.querySelectorAll("[data-pwa-state]").forEach(function (el) {
      el.classList.toggle("hidden", el.dataset.pwaState !== s);
    });
  }

  window.addEventListener("beforeinstallprompt", function (e) {
    e.preventDefault(); // niente mini-barra automatica: si installa dal bottone
    deferred = e;
    render();
  });
  window.addEventListener("appinstalled", function () {
    deferred = null;
    render();
  });

  document.addEventListener("click", function (e) {
    if (!deferred || !e.target.closest || !e.target.closest("[data-pwa-install]")) return;
    var ev = deferred;
    deferred = null;
    ev.prompt();
    // The prompt works only once: accepted, appinstalled arrives; declined,
    // the menu hint stays until the browser sends another one.
    ev.userChoice.then(render, render);
  });

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", render);
  else render();
})();
