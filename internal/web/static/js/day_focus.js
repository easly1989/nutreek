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
//       e la sposta nella sua posizione corretta (memoria o ordine logico).
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
//   Le card presenti in localStorage ma assenti dal DOM (es. giorni passati
//   non in focus di default) vengono fetched automaticamente all'init.
(function () {
  "use strict";

  // Memoria delle posizioni: per ogni data rimossa dal focus, salviamo la
  // data del predecessore nel #focus-grid al momento della rimozione (o
  // null se la card era la prima). Quando la card viene riselezionata,
  // proviamo a riposizionarla subito DOPO il predecessore se ancora
  // presente. Se non lo e' (es. anche lui rimosso), usiamo il posizionamento
  // logico (oggi prima, poi lun-dom saltando oggi).
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

    // Rimuovi card che non sono nel set salvato.
    if (grid) {
      grid.querySelectorAll("section[data-day]").forEach(function (s) {
        if (!set.has(s.dataset.day)) {
          s.remove();
        }
      });
    }

    // Aggiorna pillole.
    document
      .querySelectorAll("#timeline-pills button[data-day]")
      .forEach(function (pill) {
        setPillFocus(pill, set.has(pill.dataset.day));
      });

    // Fetcha le card che sono nel set ma non nel DOM (es. giorni passati
    // non renderizzati di default). HTMX le aggiunge in fondo a #focus-grid
    // e onAfterSwap le riposiziona nell'ordine corretto.
    if (typeof htmx === "undefined") return;
    set.forEach(function (date) {
      if (!grid) return;
      var existing = grid.querySelector('section[data-day="' + date + '"]');
      if (!existing) {
        htmx.ajax("GET", "/days/" + date, {
          target: "#focus-grid",
          swap: "beforeend",
        });
      }
    });
  }

  // ---- ordinamento logico --------------------------------------------------

  // computeWeekOrder ritorna l'array di 7 date YYYY-MM-DD nell'ordine
  // "oggi prima, poi lun-dom saltando oggi". Se oggi non appartiene alla
  // settimana visualizzata, ritorna l'ordine lun-dom canonico.
  function computeWeekOrder() {
    var grid = focusGridEl();
    if (!grid) return [];
    var weekStart = grid.dataset.weekStart;
    if (!weekStart) return [];

    // Costruisce i 7 giorni della settimana in ordine lun-dom.
    var week = [];
    var base = new Date(weekStart + "T00:00:00");
    for (var i = 0; i < 7; i++) {
      var d = new Date(base);
      d.setDate(base.getDate() + i);
      week.push(d.toISOString().slice(0, 10));
    }

    // Trova la data di "oggi" dalla card con data-state="today" (se presente).
    var todaySection = grid.querySelector("section[data-state='today']");
    // Oppure dalla pillola.
    if (!todaySection) {
      var todayPill = document.querySelector(
        "#timeline-pills button[data-state='today']"
      );
      if (todayPill) {
        var fakeTodayDate = todayPill.dataset.day;
        var tidx = week.indexOf(fakeTodayDate);
        if (tidx > 0) {
          var ordered = [fakeTodayDate];
          for (var j = 0; j < week.length; j++) {
            if (week[j] !== fakeTodayDate) ordered.push(week[j]);
          }
          return ordered;
        }
      }
      return week;
    }

    var todayDate = todaySection.dataset.day;
    var todayIdx = week.indexOf(todayDate);
    if (todayIdx <= 0) return week; // e' gia' lunedi' o non trovato

    var result = [todayDate];
    for (var k = 0; k < week.length; k++) {
      if (week[k] !== todayDate) result.push(week[k]);
    }
    return result;
  }

  // placeCardInOrder inserisce `card` nella posizione corretta del
  // focus-grid secondo l'ordinamento "oggi prima, poi lun-dom".
  function placeCardInOrder(card) {
    var grid = focusGridEl();
    if (!grid) {
      saveState();
      return;
    }
    var date = card.dataset.day;
    var order = computeWeekOrder();
    var pos = order.indexOf(date);
    if (pos < 0) {
      saveState();
      return; // data fuori settimana, lascia dove HTMX l'ha messa
    }

    // Trova la prima card gia' presente nel grid la cui posizione
    // nell'ordine logico e' SUCCESSIVA a quella della card da inserire.
    var existing = Array.from(
      grid.querySelectorAll("section[data-day]")
    ).filter(function (c) {
      return c !== card;
    });

    var insertBefore = null;
    for (var i = 0; i < existing.length; i++) {
      var otherPos = order.indexOf(existing[i].dataset.day);
      if (otherPos > pos) {
        insertBefore = existing[i];
        break;
      }
    }

    if (insertBefore) {
      grid.insertBefore(card, insertBefore);
    }
    // Se insertBefore e' null, la card e' gia' nell'ultima posizione corretta.
    saveState();
  }

  // ---- focus helpers -------------------------------------------------------

  function setPillFocus(pill, inFocus) {
    pill.dataset.inFocus = inFocus ? "true" : "false";
    var state = pill.dataset.state;
    pill.className =
      "relative w-full px-2 py-2 rounded border text-center transition " +
      (inFocus
        ? "border-accent bg-accent-soft text-text"
        : state === "past"
          ? "border-border bg-surface-2 text-muted opacity-70 hover:opacity-100"
          : "border-border bg-surface text-text hover:bg-surface-2");
  }

  function removeCard(date) {
    var card = document.querySelector('section[data-day="' + date + '"]');
    if (!card) return;
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
      // Nessuna memoria di posizione → usa l'ordine logico (oggi prima).
      placeCardInOrder(card);
      return;
    }

    positionMemory.delete(date);
    if (memo.prevDate === null) {
      grid.insertBefore(card, grid.firstChild);
      saveState();
      return;
    }
    var prev = grid.querySelector('section[data-day="' + memo.prevDate + '"]');
    if (prev && prev.parentElement === grid) {
      grid.insertBefore(card, prev.nextSibling);
    } else {
      // Predecessore non piu' in focus → posizionamento logico.
      placeCardInOrder(card);
      return;
    }
    saveState();
  }

  // ---- event handlers ------------------------------------------------------

  function onPillClick(evt) {
    var pill = evt.target.closest("[data-day]");
    if (!pill || pill.tagName !== "BUTTON") return;
    if (pill.dataset.inFocus === "true") {
      evt.preventDefault();
      evt.stopPropagation();
      removeCard(pill.dataset.day);
      setPillFocus(pill, false);
      return;
    }
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
