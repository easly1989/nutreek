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
