// day_tabs.js — the day tabs of the weekly dashboard.
//
// Every tab (#timeline-pills [role=tab]) is a link with hx-get that loads the
// day into #day-panel and updates the URL (?day=). The server's response also
// carries the day's tab with aria-selected="true" (OOB swap): here we
// deselect the others and keep the tabpanel aligned. The arrow keys move the
// selection as in a standard tablist.
(function () {
  "use strict";

  function tabs() {
    return Array.prototype.slice.call(document.querySelectorAll("#timeline-pills [role=tab]"));
  }

  function markSelected(date) {
    tabs().forEach(function (t) {
      var on = t.dataset.day === date;
      t.setAttribute("aria-selected", String(on));
      t.setAttribute("tabindex", on ? "0" : "-1");
    });
    var panel = document.getElementById("day-panel");
    if (panel) panel.setAttribute("aria-labelledby", "pill-" + date);
  }

  // htmx:load fires on every new element, those arriving OOB too: after a
  // tab change or a mutation the open day is the one of the <section> in
  // #day-panel.
  document.addEventListener("htmx:load", function (e) {
    var el = e.target;
    if (!el.matches || !el.matches("section[data-day], #timeline-pills [role=tab]")) return;
    var section = document.querySelector("#day-panel section[data-day]");
    if (section) markSelected(section.dataset.day);
  });

  document.addEventListener("keydown", function (e) {
    var tab = e.target.closest && e.target.closest("#timeline-pills [role=tab]");
    if (!tab) return;
    var list = tabs();
    var i = list.indexOf(tab);
    var next = null;
    if (e.key === "ArrowRight") next = list[(i + 1) % list.length];
    else if (e.key === "ArrowLeft") next = list[(i - 1 + list.length) % list.length];
    else if (e.key === "Home") next = list[0];
    else if (e.key === "End") next = list[list.length - 1];
    if (!next) return;
    e.preventDefault();
    next.focus();
    next.click();
  });
})();
