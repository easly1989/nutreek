// The household in the day (#238).
//  - The switch in a meal's header decides what the people's columns show: the quantities or
//    the kcal. It is one choice for every meal (data-hh-mode on <html>, which the CSS reads)
//    and it is remembered in this browser.
//  - The buttons beside a meal's macros pick whose macros the line shows.
(function () {
  "use strict";
  var KEY = "nutreek.hhmode";
  var root = document.documentElement;

  function sync() {
    var mode = root.getAttribute("data-hh-mode") === "kcal" ? "kcal" : "qty";
    document.querySelectorAll("button[data-hh-set]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.dataset.hhSet === mode));
    });
  }

  function setMode(mode) {
    if (mode === "kcal") root.setAttribute("data-hh-mode", "kcal");
    else root.removeAttribute("data-hh-mode");
    try { localStorage.setItem(KEY, mode); } catch (e) { /* private mode: the choice lasts until the page is left */ }
    sync();
  }

  try { if (localStorage.getItem(KEY) === "kcal") root.setAttribute("data-hh-mode", "kcal"); } catch (e) { /* none */ }
  sync();
  // A day swapped in by HTMX brings its own switches.
  document.addEventListener("htmx:afterSwap", sync);

  document.addEventListener("click", function (e) {
    var set = e.target.closest && e.target.closest("button[data-hh-set]");
    if (set) {
      setMode(set.dataset.hhSet);
      return;
    }
    var who = e.target.closest && e.target.closest("button[data-hh-person]");
    if (!who) return;
    var meal = who.closest("article") || document;
    meal.querySelectorAll("button[data-hh-person]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b === who));
    });
    meal.querySelectorAll("[data-hh-macros]").forEach(function (s) {
      s.hidden = s.dataset.hhMacros !== who.dataset.hhPerson;
    });
  });
})();
