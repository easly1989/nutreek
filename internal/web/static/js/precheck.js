// precheck.js: the check before shopping (form[data-precheck]). A row answered
// «Some» shows how much the user has, and that field is required; the others
// keep it hidden and out of the form.
(function () {
  "use strict";

  function sync(row) {
    var some = row.querySelector("input[type=radio][value=some]");
    var on = !!(some && some.checked);
    var panel = row.querySelector(".pc-have");
    if (!panel) return;
    panel.classList.toggle("hidden", !on);
    panel.classList.toggle("flex", on);
    panel.querySelectorAll("input, select").forEach(function (el) { el.disabled = !on; });
    var qty = panel.querySelector("input[name^=have-]");
    if (qty) {
      qty.required = on;
      if (on && !qty.value) qty.focus({ preventScroll: true });
    }
  }

  // The browser may bring back a checked «Some» (form restore, back button):
  // show its field then too.
  function syncAll() {
    document.querySelectorAll("form[data-precheck] .pc-row").forEach(sync);
  }
  document.addEventListener("DOMContentLoaded", syncAll);
  window.addEventListener("pageshow", syncAll);

  document.addEventListener("change", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element) || !t.matches("form[data-precheck] input[type=radio]")) return;
    sync(t.closest(".pc-row"));
  });
})();
