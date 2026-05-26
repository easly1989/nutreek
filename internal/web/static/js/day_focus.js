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
//       della pillola cliccata. Lo script appende la card a #focus-grid
//       e la sposta nella sua ultima posizione conosciuta (se memorizzata).
//   - Click su pillola data-in-focus="true":
//       UI-only: rimuove la card corrispondente dal DOM e flippa la
//       pillola a "false" senza chiamare il server. La posizione attuale
//       della card viene memorizzata per il ripristino successivo.
//   - Click su [data-unfocus] dentro la card: equivalente a click sulla
//       pillola "true" -> nasconde la card senza round-trip.
//
// Persistenza:
//   Lo stato (quali giorni sono in focus) viene salvato in localStorage
//   con chiave "nutreek_focus_{weekStart}" e ripristinato al page load.
(function () {
  "use strict";

  // Memoria delle posizioni: per ogni data rimossa dal focus, salviamo la
  // data del predecessore nel #focus-grid al momento della rimozione (o
  // null se la card era la prima). Quando la card viene riselezionata,
  // proviamo a riposizionarla subito DOPO il predecessore se ancora
  // presente. Se non lo e' (es. anche lui rimosso), default = in fondo.
  // La memoria viene "consumata" al ripristino (mappa Map -> delete) cosi'
  // un secondo focus successivo a una posizione cambiata via DnD parte da
  // quella nuova posizione.
  var positionMemory = new Map();

  // ---- localStorage helpers ------------------------------------------------

  function focusGridEl() {
    return document.getElementById("focus-grid");
  }

  function storageKey() {
    var grid = focusGridEl();
    var ws = grid ? grid.dataset.weekStart || "" : "";
    return ws ? "nutreek_focus_" + ws : "";
  }

  function saveState() {
    var key = storageKey();
    if (!key) return;
    var dates = [];
    var grid = focusGridEl();
    if (grid) {
      grid.querySelectorAll("section[data-day]").forEach(function (s) {
        dates.push(s.dataset.day);
      });
    }
    try {
      localStorage.setItem(key, JSON.stringify(dates));
    } catch (_) {}
  }

  function loadState() {
    var key = storageKey();
    if (!key) return;
    var saved;
    try {
      saved = JSON.parse(localStorage.getItem(key));
    } catch (_) {
      return;
    }
    if (!Array.isArray(saved)) return;
    var set = new Set(saved);
    var grid = focusGridEl();
    if (grid) {
      grid.querySelectorAll("section[data-day]").forEach(function (s) {
        if (!set.has(s.dataset.day)) {
          s.remove();
        }
      });
    }
    document
      .querySelectorAll("#timeline-pills button[data-day]")
      .forEach(function (pill) {
        setPillFocus(pill, set.has(pill.dataset.day));
      });
  }

  // ---- focus helpers -------------------------------------------------------

  function setPillFocus(pill, inFocus) {
    pill.dataset.inFocus = inFocus ? "true" : "false";
    var state = pill.dataset.state;
    // Riapplica le classi semantiche (mirror del template day_pill.html).
    pill.className =
      "w-full px-2 py-2 rounded border text-center transition " +
      (inFocus
        ? "border-accent bg-accent-soft text-text"
        : state === "past"
          ? "border-border bg-surface-2 text-muted opacity-70 hover:opacity-100"
          : "border-border bg-surface text-text hover:bg-surface-2");
  }

  function removeCard(date) {
    var card = document.querySelector('section[data-day="' + date + '"]');
    if (!card) return;
    // Memorizza la posizione (predecessore) prima di rimuovere.
    var prev = card.previousElementSibling;
    var prevDate = prev && prev.dataset ? prev.dataset.day || null : null;
    positionMemory.set(date, { prevDate: prevDate });
    card.remove();
    saveState();
  }

  function placeCardAtRemembered(card) {
    var grid = focusGridEl();
    if (!grid) return;
    var date = card.dataset.day;
    var memo = positionMemory.get(date);
    if (!memo) {
      saveState();
      return; // niente memoria -> resta dove HTMX l'ha messo (in fondo)
    }
    positionMemory.delete(date);
    if (memo.prevDate === null) {
      // Era la prima card -> rimettila in cima.
      grid.insertBefore(card, grid.firstChild);
      saveState();
      return;
    }
    var prev = grid.querySelector('section[data-day="' + memo.prevDate + '"]');
    if (prev && prev.parentElement === grid) {
      grid.insertBefore(card, prev.nextSibling);
    }
    // else: predecessore non piu' in focus -> lascia in fondo (default).
    saveState();
  }

  // ---- event handlers ------------------------------------------------------

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
    // Non in focus -> richiede la card al server.
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

  function onAfterSwap(evt) {
    // Quando il server ritorna una card-giorno (via /days/{date}) e
    // HTMX la appende a #focus-grid, controlla se abbiamo memoria della
    // sua posizione e ripristinala.
    var t = evt.detail && evt.detail.target;
    if (!t || t.id !== "focus-grid") return;
    var grid = t;
    Array.from(grid.children).forEach(function (el) {
      if (el.matches && el.matches("section[data-day]")) {
        placeCardAtRemembered(el);
      }
    });
  }

  function init() {
    loadState();
    document.body.addEventListener("click", onPillClick, true);
    document.body.addEventListener("click", onUnfocusClick);
    document.body.addEventListener("htmx:afterSwap", onAfterSwap);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
