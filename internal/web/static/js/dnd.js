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
    console.warn("Sortable not loaded: drag and drop is off.");
    return;
  }
  if (typeof htmx === "undefined") {
    console.warn("htmx not loaded: drag and drop is off.");
    return;
  }

  var INIT_FLAG = "_dndInit";

  function idsOf(list) {
    return Array.from(list.children)
      .filter(function (c) { return c.dataset && c.dataset.id; })
      .map(function (c) { return c.dataset.id; });
  }

  // The meal under the dragged row shows as the place it will land (#228).
  function highlight(list) {
    document.querySelectorAll(".dnd-over").forEach(function (n) { n.classList.remove("dnd-over"); });
    var card = list && list.closest && list.closest("article");
    if (card) card.classList.add("dnd-over");
  }

  function endDrag(el) {
    highlight(null);
    var day = el.closest("section[data-day]");
    if (day) day.classList.remove("dnd-active");
  }

  function post(el, url, values) {
    // Target of the response swap: the nearest <section data-day>
    // container, when there is one; otherwise the sortable itself.
    var target = el.closest("section[data-day]") || el;
    htmx.ajax("POST", url, {
      target: target,
      swap: "outerHTML",
      values: values,
      source: el,
      // A sortable outside any form names its CSRF token itself.
      headers: el.dataset.csrf ? { "X-CSRF-Token": el.dataset.csrf } : undefined,
    });
  }

  function init(el) {
    if (el[INIT_FLAG]) return;
    el[INIT_FLAG] = true;
    Sortable.create(el, {
      animation: 150,
      ghostClass: "opacity-40",
      handle: el.dataset.sortableHandle || undefined,
      // Lists of one group take rows from each other: the rows of the meals of a day.
      // A meal with cheats (data-no-drop) takes no rows: its own are struck out.
      group: el.dataset.sortableGroup
        ? { name: el.dataset.sortableGroup, put: function (to) { return !to.el.hasAttribute("data-no-drop"); } }
        : undefined,
      // On a touch screen a drag starts after a short press, so a thumb that
      // brushes the handle while scrolling does not pick the row up.
      delay: el.dataset.sortableGroup ? 120 : 0,
      delayOnTouchOnly: true,
      touchStartThreshold: 6,
      onStart: function () {
        var day = el.closest("section[data-day]");
        if (day && el.dataset.sortableGroup) day.classList.add("dnd-active");
      },
      onMove: function (evt) {
        if (el.dataset.sortableGroup) highlight(evt.to !== evt.from ? evt.to : null);
        return true;
      },
      onEnd: function (evt) {
        endDrag(el);
        // Dropped in another meal: the row moves, in the place it was dropped in.
        if (evt && evt.to && evt.to !== evt.from) {
          var moveURL = el.dataset.moveUrl, meal = evt.to.dataset.mealId, id = evt.item && evt.item.dataset.id;
          if (!moveURL || !meal || !id) return;
          post(el, moveURL.replace("%s", id), { meal_id: meal, ids: idsOf(evt.to).join(",") });
          return;
        }
        var ids = idsOf(el);
        if (!ids.length) return;
        var url = el.dataset.sortUrl;
        if (!url) return;
        post(el, url, { ids: ids.join(",") });
      },
    });
  }

  // A request of a list that fails (the row is gone, the order is stale) leaves the
  // page showing a move that did not happen: show the week as it is.
  // Also the «Move to…» menu of a row (move-menu-<id>): a row deleted in another
  // tab leaves it on «Loading…», or its choice with no answer.
  function fromLists(e) {
    var elt = e.detail && e.detail.elt;
    if (!elt || !elt.matches) return;
    var menu = elt.matches('[data-popover^="move-menu-"]') || (elt.closest && elt.closest('[id^="move-menu-"]'));
    if (elt.matches("[data-sortable]") || menu) window.location.reload();
  }
  ["htmx:responseError", "htmx:sendError", "htmx:timeout"].forEach(function (name) {
    document.body.addEventListener(name, fromLists);
  });

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
