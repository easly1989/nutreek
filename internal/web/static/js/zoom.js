// zoom.js: a larger preview of an image while the pointer rests on it.
//   <img data-zoom>                       the image itself, shown larger
//   <img data-zoom data-zoom-src="/big">  another, larger file for the preview
// Pointer devices only (hover and a fine pointer): on a touch screen nothing
// changes. The preview sits to the right of the image, to the left when there
// is no room, and never over the pointer. It closes on leaving, scrolling,
// Esc or when the page swaps its content (htmx).
(function () {
  "use strict";

  if (!window.matchMedia || !window.matchMedia("(hover: hover) and (pointer: fine)").matches) return;

  var WIDTH = 200, GAP = 10, DELAY = 120;
  var pop = null, timer = 0, current = null;

  function build() {
    pop = document.createElement("div");
    pop.setAttribute("aria-hidden", "true");
    pop.style.cssText = "display:none;position:fixed;z-index:60;pointer-events:none;overflow:hidden;border-radius:14px;" +
      "border:1px solid var(--border);background:var(--surface);box-shadow:var(--shadow);width:" + WIDTH + "px;max-height:calc(100vh - 16px)";
    var img = document.createElement("img");
    img.alt = "";
    img.referrerPolicy = "no-referrer";
    img.style.cssText = "display:block;width:100%;height:auto";
    pop.appendChild(img);
  }

  function hide() {
    clearTimeout(timer);
    current = null;
    if (pop) pop.style.display = "none";
  }

  // A modal <dialog> is in the top layer, above any z-index: inside one, the
  // preview has to be a child of the dialog to show.
  function host(src) { return src.closest("dialog") || document.body; }

  function place(src) {
    if (!src.isConnected) return hide();
    var r = src.getBoundingClientRect();
    pop.style.display = "block";
    var h = pop.offsetHeight || WIDTH * 0.75;
    var left = r.right + GAP;
    if (left + WIDTH > window.innerWidth - 8) left = Math.max(8, r.left - GAP - WIDTH);
    var top = Math.min(Math.max(8, r.top + r.height / 2 - h / 2), window.innerHeight - h - 8);
    pop.style.left = left + "px";
    pop.style.top = Math.max(8, top) + "px";
  }

  function show(src) {
    if (!pop) build();
    var h = host(src);
    if (pop.parentNode !== h) h.appendChild(pop);
    var img = pop.firstChild;
    var url = src.getAttribute("data-zoom-src") || src.currentSrc || src.src;
    if (!url) return;
    pop.style.display = "none";
    img.onload = function () { if (current === src) place(src); };
    img.onerror = hide;
    if (img.getAttribute("src") === url && img.complete) place(src);
    else img.src = url;
  }

  document.addEventListener("mouseover", function (e) {
    var t = e.target;
    if (!(t instanceof Element) || !t.matches("img[data-zoom]")) return;
    if (t === current) return;
    hide();
    current = t;
    timer = setTimeout(function () { if (current === t) show(t); }, DELAY);
  });

  document.addEventListener("mouseout", function (e) {
    if (e.target === current) hide();
  });
  document.addEventListener("scroll", hide, true);
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") hide(); });
  document.addEventListener("htmx:beforeSwap", hide);
})();
