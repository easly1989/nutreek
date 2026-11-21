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
    pop.className = "fixed z-[60] pointer-events-none overflow-hidden rounded-[14px] border border-border bg-surface";
    pop.style.cssText = "display:none;box-shadow:var(--shadow);width:" + WIDTH + "px";
    var img = document.createElement("img");
    img.alt = "";
    img.referrerPolicy = "no-referrer";
    img.className = "block w-full h-auto";
    pop.appendChild(img);
    document.body.appendChild(pop);
  }

  function hide() {
    clearTimeout(timer);
    current = null;
    if (pop) pop.style.display = "none";
  }

  function place(src) {
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
