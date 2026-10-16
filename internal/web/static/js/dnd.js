// dnd.js — enables drag-and-drop on any element
//   <ul data-sortable="meals" data-sort-url="/meals/reorder?...">
//     <li data-id="42">...</li>
//     ...
//   </ul>
// On drop, it collects the data-ids in the current order and does an
// htmx.ajax POST to the given endpoint (form-encoded `ids=12,9,15`). When the
// response is an HTML fragment (e.g. a day_card swapped by the server), htmx
// applies it to the target named by the `HX-Target` header or, failing that,
// to the sortable's closest container. Re-init is automatic via htmx:afterSettle.
(function () {
  "use strict";

  if (typeof Sortable === "undefined") {
    console.warn("Sortable non caricato: drag-and-drop disabilitato.");
    return;
  }
  if (typeof htmx === "undefined") {
    console.warn("htmx non caricato: drag-and-drop disabilitato.");
    return;
  }

  var INIT_FLAG = "_dndInit";

  function init(el) {
    if (el[INIT_FLAG]) return;
    el[INIT_FLAG] = true;
    Sortable.create(el, {
      animation: 150,
      ghostClass: "opacity-40",
      handle: el.dataset.sortableHandle || undefined,
      onEnd: function () {
        var ids = Array.from(el.children)
          .filter(function (c) { return c.dataset && c.dataset.id; })
          .map(function (c) { return c.dataset.id; });
        if (!ids.length) return;
        var url = el.dataset.sortUrl;
        if (!url) return;
        // Target of the response swap: the nearest <section data-day>
        // container, when there is one; otherwise the sortable itself.
        var target = el.closest("section[data-day]") || el;
        htmx.ajax("POST", url, {
          target: target,
          swap: "outerHTML",
          values: { ids: ids.join(",") },
          source: el,
        });
      },
    });
  }

  function scan(root) {
    (root || document).querySelectorAll('[data-sortable]').forEach(init);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () { scan(); });
  } else {
    scan();
  }
  document.body.addEventListener("htmx:afterSettle", function (evt) {
    scan(evt.detail && evt.detail.elt ? evt.detail.elt : null);
    scan(); // safety: outside the swapped element too
  });
})();
