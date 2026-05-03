// dnd.js — abilita drag-and-drop su qualunque elemento
//   <ul data-sortable="meals" data-sort-url="/meals/reorder?...">
//     <li data-id="42">...</li>
//     ...
//   </ul>
// Al drop, raccoglie i data-id nell'ordine attuale e fa htmx.ajax POST
// sull'endpoint dato (form-encoded `ids=12,9,15`). Se la risposta e' un
// frammento HTML (es. day_card swappato dal server), htmx lo applica al
// target indicato dall'header `HX-Target` o, in mancanza, al closest
// container del sortable. Re-init e' automatico via htmx:afterSettle.
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
        // Target dello swap di risposta: il container <section data-day> piu'
        // vicino, se esiste; altrimenti il sortable stesso.
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
    scan(); // safety: anche fuori dall'elemento swappato
  });
})();
