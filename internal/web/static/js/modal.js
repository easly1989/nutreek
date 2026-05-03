// modal.js — gestisce <dialog> caricato in #modal-host via HTMX.
// Convenzioni:
//   - Il server inserisce <dialog data-modal>...</dialog> dentro
//     #modal-host. Lo script chiama showModal() automaticamente.
//   - Bottoni con [data-modal-close] chiudono il dialog (preventDefault).
//   - Eventi `ingredient-created` e `recipe-created` (dispatched via
//     HX-Trigger header del server) chiudono il dialog e dispatch un
//     CustomEvent globale che picker possono intercettare per refresh.
(function () {
  "use strict";

  function activeModal() {
    var host = document.getElementById("modal-host");
    return host ? host.querySelector("dialog[data-modal][open]") : null;
  }

  function openInsertedModals() {
    var host = document.getElementById("modal-host");
    if (!host) return;
    host.querySelectorAll("dialog[data-modal]:not([open])").forEach(function (d) {
      try { d.showModal(); }
      catch (e) { /* fallback: attribute open per browser senza HTMLDialog */ d.setAttribute("open", ""); }
    });
  }

  function closeActive() {
    var d = activeModal();
    if (!d) return;
    try { d.close(); } catch (e) { d.removeAttribute("open"); }
    var host = document.getElementById("modal-host");
    if (host) host.innerHTML = "";
  }

  // HTMX inserisce contenuto in #modal-host
  document.body.addEventListener("htmx:afterSwap", function (evt) {
    var t = evt.detail && evt.detail.target;
    if (t && (t.id === "modal-host" || (t.closest && t.closest("#modal-host")))) {
      openInsertedModals();
    }
  });

  // Click su bottoni di chiusura interni al dialog
  document.body.addEventListener("click", function (evt) {
    if (evt.target.closest && evt.target.closest("[data-modal-close]")) {
      evt.preventDefault();
      closeActive();
    }
  });

  // Click sul backdrop del <dialog> chiude. Quando il browser propaga un
  // click sul ::backdrop, il target dell'evento e' il <dialog> stesso (non
  // un suo child). Sui programmatic .click() il target e' l'elemento clicked
  // direttamente, quindi questa condizione non li intercetta per sbaglio.
  document.body.addEventListener("click", function (evt) {
    var d = activeModal();
    if (d && evt.target === d) closeActive();
  });

  // ESC: il <dialog> nativo gia' lo chiude col dispatch di "cancel"+"close",
  // ripuliamo l'host quando il dialog e' chiuso.
  document.body.addEventListener("close", function (evt) {
    var d = evt.target;
    if (d.matches && d.matches("dialog[data-modal]")) {
      var host = document.getElementById("modal-host");
      if (host) host.innerHTML = "";
    }
  }, true);

  // Eventi custom emessi via HX-Trigger del server: chiudi e ridispatcha
  // su window cosi' i picker possono ricaricare i risultati.
  ["ingredient-created", "recipe-created"].forEach(function (ev) {
    document.body.addEventListener(ev, function (evt) {
      closeActive();
      window.dispatchEvent(new CustomEvent(ev, { detail: evt.detail }));
    });
  });
})();
