// busy_bar.js: a thin bar at the top of the page while a request of htmx waits, for the actions
// the server has to compute (merge two rows, copy a list, regenerate, the grouping, ...), so a
// tap that does not answer at once does not look like nothing happened. It shows only when the
// wait passes 250 ms. The list's own refresh (shop_queue.js, header X-Shop-Refresh) and the
// searches typed into a field and the pickers that load themselves (hx-trigger "input", "load") stay out of it.
(function () {
  "use strict";

  var pending = 0;
  var showTimer = null;
  var bar = null;
  var calm = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)");

  function ignored(e) {
    var d = e.detail || {};
    var h = (d.requestConfig && d.requestConfig.headers) || {};
    return !!h["X-Shop-Refresh"] || (d.elt && d.elt.matches && d.elt.matches("[hx-trigger*=input], [hx-trigger*=load]"));
  }

  function make() {
    bar = document.createElement("div");
    bar.setAttribute("aria-hidden", "true");
    bar.style.cssText = "position:fixed;top:0;left:0;right:0;height:3px;z-index:60;pointer-events:none;background:var(--accent);" +
      "transform-origin:left;transform:scaleX(0);opacity:0";
    document.body.appendChild(bar);
  }

  function show() {
    if (!bar) make();
    bar.style.transition = calm && calm.matches ? "none" : "transform 6s cubic-bezier(0.1, 0.7, 0.2, 1)";
    bar.style.opacity = "1";
    bar.style.transform = "scaleX(0.85)";
  }

  function hide() {
    if (!bar) return;
    bar.style.transition = calm && calm.matches ? "none" : "transform 150ms ease-out, opacity 250ms ease 120ms";
    bar.style.transform = "scaleX(1)";
    bar.style.opacity = "0";
    setTimeout(function () { if (!pending && bar) bar.style.transform = "scaleX(0)"; }, 450);
  }

  document.addEventListener("htmx:beforeRequest", function (e) {
    if (ignored(e)) return;
    pending++;
    if (pending === 1) { clearTimeout(showTimer); showTimer = setTimeout(show, 250); }
  });
  function done(e) {
    if (ignored(e)) return;
    pending = Math.max(0, pending - 1);
    if (pending === 0) { clearTimeout(showTimer); hide(); }
  }
  document.addEventListener("htmx:afterRequest", done);
})();
