// sheet.js: the quantity editors of the shopping list (<details data-sheet>).
//  - Esc, the close buttons and a click on the dimmed page close the open one.
//  - Opening one puts the cursor in its number field.
//  The g/ml <-> pz conversion of the number comes from picker.js, which
//  handles every form with data-unit-weight.
(function () {
  "use strict";

  // close shuts an editor, drops what was typed in it and gives the focus back
  // to its pencil.
  function close(d) {
    if (!d || !d.open) return;
    d.open = false;
    var form = d.querySelector("form");
    if (form) form.reset();
    var pencil = d.querySelector("summary");
    if (pencil) pencil.focus({ preventScroll: true });
  }

  function closeAll(except) {
    document.querySelectorAll("details[data-sheet][open]").forEach(function (d) {
      if (d !== except) close(d);
    });
  }

  document.addEventListener("click", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;
    var closer = t.closest("[data-sheet-close]");
    if (closer) {
      close(closer.closest("details[data-sheet]"));
      return;
    }
    // A click on the dim is a click on the <details> itself (its ::before).
    if (t.matches("details[data-sheet][open]")) close(t);
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
