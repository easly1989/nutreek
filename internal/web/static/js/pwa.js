// pwa.js — app installabile (#59).
//
// Registra il service worker (/sw.js: cache dei soli file in /static) e, in
// Impostazioni, mostra il blocco «App sul telefono» giusto per il browser:
//   installata  aperta dall'icona sulla schermata Home
//   http        contesto non sicuro: niente installazione ne' fotocamera
//   prompt      Chrome/Edge/Android: bottone «Installa l'app» (beforeinstallprompt)
//   ios         iPhone/iPad: i passaggi in Safari, non c'e' un prompt
//   altro       tutto il resto (e senza JS): installa dal menu del browser
(function () {
  "use strict";

  if (window.isSecureContext && "serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register("/sw.js").catch(function () { /* senza cache, l'app funziona lo stesso */ });
    });
  }

  var deferred = null; // l'evento beforeinstallprompt, da usare al click

  function standalone() {
    return window.matchMedia("(display-mode: standalone)").matches || window.navigator.standalone === true;
  }
  function ios() {
    return /iPad|iPhone|iPod/.test(navigator.userAgent) ||
      (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1); // iPad che si presenta come Mac
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
    // Il prompt vale una volta sola: accettato arriva appinstalled, rifiutato
    // resta l'indicazione del menu finche' il browser non ne manda un altro.
    ev.userChoice.then(render, render);
  });

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", render);
  else render();
})();
