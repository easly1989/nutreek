// day_swipe.js: swipe the open day of the week to move to the next or the
// previous one on a touch screen (#250), like tapping its tab in #timeline-pills.
//
//  - Touch only. The day follows the finger; at the edge, the name of the day that
//    comes in shows («Domenica 22 ›»). Past THRESHOLD (or short of it) the day
//    goes on or springs back.
//  - The end of the week does not wrap: the day gives a little and comes back.
//  - A gesture that starts on a control, on the grip of the drag and drop, in an
//    area that scrolls sideways or in an open dialog is left alone, and a mostly
//    vertical move is a scroll.
//  - The change itself is the tab's own click (day_tabs.js and htmx do the rest).
(function () {
  "use strict";

  var THRESHOLD = 80; // pixels to pass to change day
  var panel = document.getElementById("day-panel");
  if (!panel) return;
  var start = null;
  var pendingSide = null; // "next" or "prev" while the new day is loading
  var hint = null;
  var calm = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)");

  function tabs() {
    return Array.prototype.slice.call(document.querySelectorAll("#timeline-pills [role=tab]"));
  }

  // neighbours returns the tabs before and after the open day (null at the ends).
  function neighbours() {
    var list = tabs();
    var i = list.findIndex(function (t) { return t.getAttribute("aria-selected") === "true"; });
    if (i < 0) return { prev: null, next: null };
    return { prev: list[i - 1] || null, next: list[i + 1] || null };
  }

  // blocked says whether a gesture that starts on el is not ours.
  function blocked(el) {
    if (el.closest("a, button, input, select, textarea, summary, label, dialog, [aria-modal=true], [data-meal-grip], [data-item-grip], [role=tab]")) return true;
    for (var n = el; n && n !== panel; n = n.parentElement) {
      var ox = getComputedStyle(n).overflowX;
      if ((ox === "auto" || ox === "scroll") && n.scrollWidth > n.clientWidth + 1) return true;
    }
    return false;
  }

  function section() { return panel.querySelector("section[data-day]"); }

  function showHint(side, tab, strength) {
    if (!tab) return hideHint();
    if (!hint) {
      hint = document.createElement("div");
      hint.className = "day-swipe-hint";
      hint.setAttribute("aria-hidden", "true");
      document.body.appendChild(hint);
    }
    var title = tab.dataset.title || "";
    hint.dataset.side = side;
    hint.textContent = side === "next" ? title + " ›" : "‹ " + title;
    hint.style.opacity = String(Math.min(1, strength));
  }

  function hideHint() {
    if (!hint) return;
    var h = hint; hint = null;
    h.style.transition = calm && calm.matches ? "none" : "opacity 120ms ease";
    h.style.opacity = "0";
    setTimeout(function () { h.remove(); }, 160);
  }

  function springBack(sec) {
    if (!sec) return;
    sec.style.transition = calm && calm.matches ? "none" : "transform 160ms ease, opacity 160ms ease";
    sec.style.transform = "";
    sec.style.opacity = "";
    setTimeout(function () { sec.style.transition = ""; }, 200);
  }

  panel.addEventListener("pointerdown", function (e) {
    if (e.pointerType === "mouse" || !e.isPrimary || !(e.target instanceof Element)) return;
    // A day is loading: the open tab is not the new one yet.
    if (pendingSide || blocked(e.target) || !section()) return;
    start = { x: e.clientX, y: e.clientY, id: e.pointerId, on: false, dx: 0, sec: section() };
  });

  panel.addEventListener("pointermove", function (e) {
    if (!start || e.pointerId !== start.id) return;
    var dx = e.clientX - start.x, dy = e.clientY - start.y;
    if (!start.on) {
      if (Math.abs(dy) > 10 && Math.abs(dy) > Math.abs(dx)) { start = null; return; } // a scroll
      if (Math.abs(dx) < 10 || Math.abs(dx) < Math.abs(dy) * 1.5) return;
      start.on = true;
      try { panel.setPointerCapture(e.pointerId); } catch (err) { /* the day may be gone */ }
    }
    start.dx = dx;
    var side = dx < 0 ? "next" : "prev";
    var tab = neighbours()[side];
    // At the end of the week the day gives a little (a quarter of the move).
    var shown = tab ? dx : dx * 0.25;
    start.sec.style.transition = "none";
    start.sec.style.transform = "translateX(" + shown + "px)";
    if (tab) showHint(side, tab, Math.abs(dx) / 40); else hideHint();
  });

  function finish(e) {
    if (!start || e.pointerId !== start.id) return;
    var s = start; start = null;
    if (!s.on) return;
    // The swipe must not count as a tap on what is under the finger.
    var swallow = function (ev) { ev.preventDefault(); ev.stopPropagation(); };
    panel.addEventListener("click", swallow, { capture: true, once: true });
    setTimeout(function () { panel.removeEventListener("click", swallow, true); }, 350);
    hideHint();
    var side = s.dx < 0 ? "next" : "prev";
    var tab = neighbours()[side];
    if (e.type === "pointercancel" || Math.abs(s.dx) < THRESHOLD || !tab) {
      springBack(s.sec);
      return;
    }
    pendingSide = side;
    s.sec.style.transition = calm && calm.matches ? "none" : "transform 120ms ease, opacity 120ms ease";
    s.sec.style.transform = "translateX(" + (side === "next" ? "-40px" : "40px") + ")";
    s.sec.style.opacity = "0.4";
    tab.click();
  }
  panel.addEventListener("pointerup", finish);
  panel.addEventListener("pointercancel", finish);

  // The new day comes in from the side the finger moved to; a request that fails
  // leaves the day where it was.
  document.addEventListener("htmx:afterSwap", function (e) {
    if (!pendingSide || e.detail.target !== panel) return;
    var sec = section();
    if (sec) {
      sec.classList.add("day-in-" + pendingSide);
      setTimeout(function () { sec.classList.remove("day-in-next", "day-in-prev"); }, 260);
    }
    pendingSide = null;
  });
  function failed() {
    if (!pendingSide) return;
    pendingSide = null;
    springBack(section());
  }
  document.addEventListener("htmx:responseError", failed);
  document.addEventListener("htmx:sendError", failed);
  // The tab's request ended with no swap (a timeout, an abort, an empty answer):
  // afterRequest comes after afterSwap, which clears pendingSide when there was one.
  document.addEventListener("htmx:afterRequest", function (e) {
    if (pendingSide && e.detail.elt && e.detail.elt.matches && e.detail.elt.matches("#timeline-pills [role=tab]")) failed();
  });
})();
