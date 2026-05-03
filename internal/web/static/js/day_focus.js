// day_focus.js — dopo ogni swap HTMX, garantisce che le card-giorno con
// state=focus siano nella griglia #focus-grid e che le pillole con
// state=past stiano nella timeline #timeline-past. Click sulla pillola
// genera HTMX swap inline nel timeline; questo script poi trasloca la
// card espansa nella griglia.
(function () {
  "use strict";

  function reflowDays() {
    var grid = document.getElementById("focus-grid");
    if (grid) {
      document.querySelectorAll('section[data-day][data-state="focus"]').forEach(function (el) {
        if (el.parentElement !== grid) grid.appendChild(el);
      });
    }
    var tl = document.getElementById("timeline-past");
    if (tl) {
      document.querySelectorAll('button[data-day][data-state="past"]').forEach(function (el) {
        if (el.parentElement !== tl) tl.appendChild(el);
      });
    }
  }

  document.body.addEventListener("htmx:afterSettle", reflowDays);
})();
