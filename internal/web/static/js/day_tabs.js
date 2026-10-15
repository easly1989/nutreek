// day_tabs.js — schede dei giorni nella dashboard settimanale.
//
// Ogni scheda (#timeline-pills [role=tab]) e' un link con hx-get che carica
// il giorno in #day-panel e aggiorna l'URL (?day=). La risposta del server
// porta anche la scheda del giorno con aria-selected="true" (swap OOB):
// qui deselezioniamo le altre e teniamo allineato il tabpanel. Le frecce
// spostano la selezione come in un tablist standard.
(function () {
  "use strict";

  function tabs() {
    return Array.prototype.slice.call(document.querySelectorAll("#timeline-pills [role=tab]"));
  }

  function markSelected(date) {
    tabs().forEach(function (t) {
      var on = t.dataset.day === date;
      t.setAttribute("aria-selected", String(on));
      t.setAttribute("tabindex", on ? "0" : "-1");
    });
    var panel = document.getElementById("day-panel");
    if (panel) panel.setAttribute("aria-labelledby", "pill-" + date);
  }

  // htmx:load scatta su ogni elemento nuovo, anche quelli arrivati OOB:
  // dopo un cambio di scheda o una mutazione il giorno aperto e' quello
  // della <section> in #day-panel.
  document.addEventListener("htmx:load", function (e) {
    var el = e.target;
    if (!el.matches || !el.matches("section[data-day], #timeline-pills [role=tab]")) return;
    var section = document.querySelector("#day-panel section[data-day]");
    if (section) markSelected(section.dataset.day);
  });

  document.addEventListener("keydown", function (e) {
    var tab = e.target.closest && e.target.closest("#timeline-pills [role=tab]");
    if (!tab) return;
    var list = tabs();
    var i = list.indexOf(tab);
    var next = null;
    if (e.key === "ArrowRight") next = list[(i + 1) % list.length];
    else if (e.key === "ArrowLeft") next = list[(i - 1 + list.length) % list.length];
    else if (e.key === "Home") next = list[0];
    else if (e.key === "End") next = list[list.length - 1];
    if (!next) return;
    e.preventDefault();
    next.focus();
    next.click();
  });
})();
