// sheet.js: the quantity editors of the shopping list (<details data-sheet>).
//  - Esc, the close buttons and a click on the dimmed page close the open one.
//  - Opening one puts the cursor in its number field.
//  The g/ml <-> pz conversion of the number comes from picker.js, which
//  handles every form with data-unit-weight.
(function () {
  "use strict";

  function closeAll(except) {
    document.querySelectorAll("details[data-sheet][open]").forEach(function (d) {
      if (d !== except) d.open = false;
    });
  }

  document.addEventListener("click", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;
    var closer = t.closest("[data-sheet-close]");
    if (closer) {
      var d = closer.closest("details[data-sheet]");
      if (d) d.open = false;
      return;
    }
    // A click on the dim is a click on the <details> itself (its ::before).
    if (t.matches("details[data-sheet][open]")) t.open = false;
  });

  document.addEventListener("keydown", function (evt) {
    if (evt.key === "Escape") closeAll();
  });

  document.addEventListener("toggle", function (evt) {
    var d = evt.target;
    if (!(d instanceof Element) || !d.matches("details[data-sheet]") || !d.open) return;
    closeAll(d);
    var field = d.querySelector(".sheet-qty, input[name=description]");
    if (field) { field.focus({ preventScroll: true }); if (field.select) field.select(); }
  }, true);
})();
