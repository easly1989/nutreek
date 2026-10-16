// modal.js — handles the <dialog>s loaded via HTMX into the [data-modal-host]
// hosts (#modal-host and, for a dialog opened from another, #modal-host-2).
// Conventions:
//   - The server inserts <dialog data-modal>...</dialog> into a host. The
//     script calls showModal() automatically.
//   - Buttons with [data-modal-close] close their dialog.
//   - Forms with [data-close-dialog] close their dialog when the HTMX
//     request succeeds (e.g. «Aggiungi» in the picker).
//   - The `ingredient-created` and `recipe-created` events (dispatched via
//     the server's HX-Trigger header) close the dialog that produced them and
//     dispatch a global CustomEvent the pickers can listen for.
(function () {
  "use strict";

  function openInserted(host) {
    host.querySelectorAll("dialog[data-modal]:not([open])").forEach(function (d) {
      try { d.showModal(); }
      catch (e) { /* fallback: attribute open per browser senza HTMLDialog */ d.setAttribute("open", ""); }
    });
  }

  // Close the dialog and empty its host.
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

  // Clicks on close buttons inside the dialog
  document.body.addEventListener("click", function (evt) {
    var btn = evt.target.closest && evt.target.closest("[data-modal-close]");
    if (btn) {
      evt.preventDefault();
      close(btn.closest("dialog[data-modal]"));
    }
  });

  // A click on the <dialog>'s backdrop closes it. When the browser propagates
  // a click on ::backdrop, the event's target is the <dialog> itself (not one
  // of its children). On programmatic .click() the target is the element
  // clicked directly, so this condition does not catch those by mistake.
  document.body.addEventListener("click", function (evt) {
    if (evt.target.matches && evt.target.matches("dialog[data-modal][open]")) close(evt.target);
  });

  // ESC: the native <dialog> already closes by dispatching "cancel"+"close";
  // we clean up the host when the dialog is closed.
  document.body.addEventListener("close", function (evt) {
    var d = evt.target;
    if (d.matches && d.matches("dialog[data-modal]")) {
      var host = d.closest("[data-modal-host]");
      if (host) host.innerHTML = "";
    }
  }, true);

  // Form [data-close-dialog]: when the request succeeds the dialog closes.
  document.body.addEventListener("htmx:afterRequest", function (evt) {
    var elt = evt.detail && evt.detail.elt;
    if (!evt.detail.successful || !elt || !elt.matches || !elt.matches("[data-close-dialog]")) return;
    close(elt.closest("dialog[data-modal]"));
  });

  // Custom events emitted via the server's HX-Trigger: close and re-dispatch
  // on window so the pickers can reload their results.
  ["ingredient-created", "recipe-created"].forEach(function (ev) {
    document.body.addEventListener(ev, function (evt) {
      close(evt.target.closest && evt.target.closest("dialog[data-modal]"));
      window.dispatchEvent(new CustomEvent(ev, { detail: evt.detail }));
    });
  });
})();
