// reorder.js: the shopping list in the user's own order (#231).
//  - «Reorder» shows an up and a down arrow on every row, for the keyboard and
//    for those who cannot drag; drag and drop is dnd.js (Sortable).
//  - An arrow moves the row one place and saves the order like a drag does.
(function () {
  "use strict";

  document.addEventListener("click", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;

    var toggle = t.closest("[data-reorder-toggle]");
    if (toggle) {
      var list = document.getElementById("shop-list");
      var on = list.toggleAttribute("data-reorder-on");
      toggle.setAttribute("aria-pressed", on ? "true" : "false");
      return;
    }

    var btn = t.closest("[data-move]");
    if (!btn) return;
    var li = btn.closest("li[data-id]");
    var ul = li && li.parentElement;
    if (!ul || !ul.dataset.sortUrl) return;
    var other = btn.dataset.move === "up" ? li.previousElementSibling : li.nextElementSibling;
    if (!other) return;
    if (btn.dataset.move === "up") ul.insertBefore(li, other);
    else ul.insertBefore(other, li);
    btn.focus({ preventScroll: true });

    var ids = Array.from(ul.children).map(function (c) { return c.dataset.id; }).filter(Boolean);
    htmx.ajax("POST", ul.dataset.sortUrl, {
      target: ul,
      swap: "none",
      values: { ids: ids.join(",") },
      headers: { "X-CSRF-Token": ul.dataset.csrf },
    });
  });
})();
