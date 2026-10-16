// modal.js — gestisce i <dialog> caricati via HTMX negli host [data-modal-host]
// (#modal-host e, per una finestra aperta da un'altra, #modal-host-2).
// Convenzioni:
//   - Il server inserisce <dialog data-modal>...</dialog> in un host. Lo
//     script chiama showModal() automaticamente.
//   - Bottoni con [data-modal-close] chiudono il loro dialog.
//   - Form con [data-close-dialog] chiudono il loro dialog quando la
//     richiesta HTMX va a buon fine (es. «Aggiungi» nel picker).
//   - Eventi `ingredient-created` e `recipe-created` (dispatched via
//     HX-Trigger header del server) chiudono il dialog che li ha generati e
//     dispatchano un CustomEvent globale che i picker possono intercettare.
(function () {
  "use strict";

  function openInserted(host) {
    host.querySelectorAll("dialog[data-modal]:not([open])").forEach(function (d) {
      try { d.showModal(); }
      catch (e) { /* fallback: attribute open per browser senza HTMLDialog */ d.setAttribute("open", ""); }
    });
  }

  // Chiude il dialog e svuota il suo host.
  function close(d) {
    if (!d) return;
    try { d.close(); } catch (e) { d.removeAttribute("open"); }
    var host = d.closest("[data-modal-host]");
    if (host) host.innerHTML = "";
  }

  // HTMX inserisce contenuto in un host
  document.body.addEventListener("htmx:afterSwap", function (evt) {
    var t = evt.detail && evt.detail.target;
    var host = t && t.closest && t.closest("[data-modal-host]");
    if (host) openInserted(host);
  });

  // Click su bottoni di chiusura interni al dialog
  document.body.addEventListener("click", function (evt) {
    var btn = evt.target.closest && evt.target.closest("[data-modal-close]");
    if (btn) {
      evt.preventDefault();
      close(btn.closest("dialog[data-modal]"));
    }
  });

  // Click sul backdrop del <dialog> chiude. Quando il browser propaga un
  // click sul ::backdrop, il target dell'evento e' il <dialog> stesso (non
  // un suo child). Sui programmatic .click() il target e' l'elemento clicked
  // direttamente, quindi questa condizione non li intercetta per sbaglio.
  document.body.addEventListener("click", function (evt) {
    if (evt.target.matches && evt.target.matches("dialog[data-modal][open]")) close(evt.target);
  });

  // ESC: il <dialog> nativo gia' lo chiude col dispatch di "cancel"+"close",
  // ripuliamo l'host quando il dialog e' chiuso.
  document.body.addEventListener("close", function (evt) {
    var d = evt.target;
    if (d.matches && d.matches("dialog[data-modal]")) {
      var host = d.closest("[data-modal-host]");
      if (host) host.innerHTML = "";
    }
  }, true);

  // Form [data-close-dialog]: a richiesta riuscita il dialog si chiude.
  document.body.addEventListener("htmx:afterRequest", function (evt) {
    var elt = evt.detail && evt.detail.elt;
    if (!evt.detail.successful || !elt || !elt.matches || !elt.matches("[data-close-dialog]")) return;
    close(elt.closest("dialog[data-modal]"));
  });

  // Eventi custom emessi via HX-Trigger del server: chiudi e ridispatcha
  // su window cosi' i picker possono ricaricare i risultati.
  ["ingredient-created", "recipe-created"].forEach(function (ev) {
    document.body.addEventListener(ev, function (evt) {
      close(evt.target.closest && evt.target.closest("dialog[data-modal]"));
      window.dispatchEvent(new CustomEvent(ev, { detail: evt.detail }));
    });
  });
})();
