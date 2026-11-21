// swipe.js: swiping a shopping row on a touch screen (#230).
//   to the right: ignore the item (it stays, greyed out; swiping again restores it)
//   to the left:  delete the item, only when it was added by hand (an item
//                 from the weekly plan can be ignored, never deleted)
// A row is <li class="swipe-row" data-swipe-ignore="url" data-swipe-delete="url"
// data-csrf="token"> with a .swipe-fg that slides over the two layers behind it.
// Past the threshold the action is posted with htmx and #shop-list is swapped;
// short of it the row springs back. A mouse uses the buttons on the row instead.
(function () {
  "use strict";

  var THRESHOLD = 96; // pixels to pass to commit
  var start = null;

  function reset(li, fg) {
    li.removeAttribute("data-swipe");
    fg.style.transition = "transform 160ms ease";
    fg.style.transform = "";
    setTimeout(function () { fg.style.transition = ""; }, 200);
  }

  document.addEventListener("pointerdown", function (e) {
    if (e.pointerType === "mouse" || !e.isPrimary) return;
    var li = e.target instanceof Element && e.target.closest("li.swipe-row");
    if (!li || document.getElementById("shop-list").hasAttribute("data-reorder-on")) return;
    // Not from a control, the drag grip or an open editor.
    if (li.hasAttribute("data-busy")) return;
    if (e.target.closest("[data-shop-grip], details[data-sheet], input, select, textarea, [data-move-btns], [data-similar]")) return;
    start = { li: li, fg: li.querySelector(".swipe-fg"), x: e.clientX, y: e.clientY, id: e.pointerId, on: false, dx: 0 };
  });

  document.addEventListener("pointermove", function (e) {
    if (!start || e.pointerId !== start.id) return;
    var dx = e.clientX - start.x, dy = e.clientY - start.y;
    if (!start.on) {
      if (Math.abs(dy) > 10 && Math.abs(dy) > Math.abs(dx)) { start = null; return; } // a scroll
      if (Math.abs(dx) < 10 || Math.abs(dx) < Math.abs(dy) * 1.5) return;
      start.on = true;
      try { start.li.setPointerCapture(e.pointerId); } catch (err) { /* the row may be gone */ }
    }
    start.dx = dx;
    // A row without a delete address (one from the plan) does not swipe left.
    if (dx < 0 && !start.li.getAttribute("data-swipe-delete")) dx = 0;
    start.li.setAttribute("data-swipe", dx > 0 ? "right" : dx < 0 ? "left" : "");
    start.fg.style.transition = "none";
    start.fg.style.transform = "translateX(" + dx + "px)";
  });

  function finish(e) {
    if (!start || e.pointerId !== start.id) return;
    var s = start; start = null;
    if (!s.on) return;
    // The swipe must not count as a tap on the row.
    // (Touch browsers fire no click after a drag; if one comes it is swallowed,
    // and the guard goes away either way, so the next tap is a real one.)
    var swallow = function (ev) { ev.preventDefault(); ev.stopPropagation(); };
    s.li.addEventListener("click", swallow, true);
    setTimeout(function () { s.li.removeEventListener("click", swallow, true); }, 350);
    var url = s.dx > 0 ? s.li.getAttribute("data-swipe-ignore") : s.li.getAttribute("data-swipe-delete");
    if (e.type !== "pointercancel" && Math.abs(s.dx) >= THRESHOLD && url && typeof htmx !== "undefined") {
      s.li.setAttribute("data-busy", "");
      s.fg.style.transition = "transform 140ms ease";
      s.fg.style.transform = "translateX(" + (s.dx > 0 ? "100%" : "-100%") + ")";
      htmx.ajax("POST", url, {
        target: "#shop-list", select: "#shop-list", swap: "outerHTML",
        values: { csrf_token: s.li.getAttribute("data-csrf") },
      });
      return;
    }
    reset(s.li, s.fg);
  }
  // A request that fails leaves the row where it was.
  function failed() {
    document.querySelectorAll("li.swipe-row[data-busy]").forEach(function (li) {
      li.removeAttribute("data-busy");
      reset(li, li.querySelector(".swipe-fg"));
    });
  }
  document.addEventListener("htmx:responseError", failed);
  document.addEventListener("htmx:sendError", failed);
  document.addEventListener("pointerup", finish);
  document.addEventListener("pointercancel", finish);
})();
