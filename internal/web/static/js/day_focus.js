// day_focus.js — gestisce il toggle focus delle card-giorno.
//
// Modello:
//   #timeline-pills contiene SEMPRE 7 pillole (una per giorno).
//   Una pillola con data-in-focus="true" indica che la card piena per
//   quel giorno e' presente in #focus-grid; "false" la nasconde.
//
// Interazioni:
//   - Click su pillola data-in-focus="false":
//       fetch /days/{date} via HTMX -> il server ritorna la card +
//       una pillola OOB (data-in-focus="true") che HTMX swappa al posto
//       della pillola cliccata. Lo script appende la card a #focus-grid.
//   - Click su pillola data-in-focus="true":
//       UI-only: rimuove la card corrispondente dal DOM e flippa la
//       pillola a "false" senza chiamare il server.
//   - Click su [data-unfocus] dentro la card: equivalente a click sulla
//       pillola "true" -> nasconde la card senza round-trip.
(function () {
  "use strict";

  function setPillFocus(pill, inFocus) {
    pill.dataset.inFocus = inFocus ? "true" : "false";
    var state = pill.dataset.state;
    // Riapplica le classi semantiche (mirror del template day_pill.html).
    pill.className =
      "shrink-0 px-3 py-2 rounded border text-left transition " +
      (inFocus
        ? "border-accent bg-accent-soft text-text"
        : state === "past"
          ? "border-border bg-surface-2 text-muted opacity-70 hover:opacity-100"
          : "border-border bg-surface text-text hover:bg-surface-2");
  }

  function removeCard(date) {
    var card = document.querySelector('section[data-day="' + date + '"]');
    if (card) card.remove();
  }

  function onPillClick(evt) {
    var pill = evt.target.closest("[data-day]");
    if (!pill || pill.tagName !== "BUTTON") return;
    if (pill.dataset.inFocus === "true") {
      // Rimuovi dal focus, UI-only.
      evt.preventDefault();
      evt.stopPropagation();
      removeCard(pill.dataset.day);
      setPillFocus(pill, false);
      return;
    }
    // Non in focus -> richiede la card al server. Chiamo htmx.ajax cosi'
    // posso controllare il target (la card va in #focus-grid, non sostituisce
    // la pillola). HTMX gestira' anche lo OOB swap della pillola dal response.
    evt.preventDefault();
    evt.stopPropagation();
    if (typeof htmx === "undefined") return;
    htmx.ajax("GET", "/days/" + pill.dataset.day, {
      target: "#focus-grid",
      swap: "beforeend",
    });
  }

  function onUnfocusClick(evt) {
    var btn = evt.target.closest("[data-unfocus]");
    if (!btn) return;
    evt.preventDefault();
    var date = btn.dataset.unfocus;
    removeCard(date);
    var pill = document.getElementById("pill-" + date);
    if (pill) setPillFocus(pill, false);
  }

  function init() {
    document.body.addEventListener("click", onPillClick, true);
    document.body.addEventListener("click", onUnfocusClick);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
