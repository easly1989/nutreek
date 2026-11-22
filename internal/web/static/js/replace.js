// replace.js: the dialog that replaces the ingredient of a row (#227).
//
// The list of suggestions or results is loaded into the form by htmx; choosing a
// row turns the button on and names the ingredient in it. A new list (another
// search) drops the choice, because its radios are gone.
(function () {
  "use strict";

  function button(form) {
    return form.querySelector("[data-replace-submit]");
  }

  function reset(form) {
    var b = button(form);
    if (!b) return;
    b.disabled = true;
    b.textContent = b.dataset.label;
  }

  document.addEventListener("change", function (e) {
    var radio = e.target;
    if (!(radio instanceof Element) || !radio.matches("input[name=choice]")) return;
    var form = radio.closest("[data-replace-form]");
    var b = form && button(form);
    if (!b) return;
    b.disabled = false;
    b.textContent = radio.dataset.confirm || b.dataset.label;
  });

  // The button waits while the form is on its way (an OpenFoodFacts import can take
  // seconds), so a second tap does not send it again. Not hx-disabled-elt: the
  // list inside the form would inherit it, and htmx would enable the button again
  // when the list arrives.
  document.addEventListener("htmx:beforeRequest", function (e) {
    var elt = e.detail && e.detail.elt;
    if (!elt || !elt.matches || !elt.matches("[data-replace-form]")) return;
    var b = button(elt);
    if (b) b.disabled = true;
  });
  document.addEventListener("htmx:sendError", function (e) {
    var elt = e.detail && e.detail.elt;
    if (!elt || !elt.matches || !elt.matches("[data-replace-form]")) return;
    var b = button(elt);
    if (b) b.disabled = false;
  });

  // The row was deleted in another tab, or the server failed: htmx shows nothing
  // for an error status, so reload to show the week as it is.
  document.addEventListener("htmx:responseError", function (e) {
    var elt = e.detail && e.detail.elt;
    if (elt && elt.matches && elt.matches("[data-replace-form]")) window.location.reload();
  });

  // The search comes back twice (the catalog at once, OpenFoodFacts a moment
  // later): a row chosen in the first list stays chosen when it is in the second.
  var chosen = "";
  document.addEventListener("htmx:beforeSwap", function (e) {
    var t = e.detail && e.detail.target;
    if (!t || t.id !== "replace-results") return;
    var on = t.querySelector("input[name=choice]:checked");
    chosen = on ? on.value : "";
  });

  document.addEventListener("htmx:afterSwap", function (e) {
    var t = e.detail && e.detail.target;
    if (!t || t.id !== "replace-results") return;
    var form = t.closest("[data-replace-form]");
    if (!form) return;
    // A short line, not the list, goes to the screen reader.
    var live = document.getElementById("replace-live");
    var count = t.querySelector("[data-result-count]");
    if (live) live.textContent = count ? count.textContent : (t.textContent || "").trim();
    reset(form);
    if (!chosen) return;
    var again = Array.prototype.find.call(t.querySelectorAll("input[name=choice]"), function (r) { return r.value === chosen; });
    chosen = "";
    if (again) {
      again.checked = true;
      again.dispatchEvent(new Event("change", { bubbles: true }));
    }
  });
})();
