// marquee.js: a line of text that is too long for its box scrolls slowly back
// and forth; one that fits does not move (#259).
//   <span class="marq"><span class="mq">A long name</span></span>
// The box (.marq) has a fixed width and one line; marquee.js measures the text
// (.mq), and when part of it is hidden marks the box with data-over and sets
// how far (--mq-d) and how long (--mq-t) it scrolls. The animation is in
// input.css (.marq[data-over]); with «reduce motion» it does not scroll, the
// text is cut with an ellipsis and the full text is the box's tooltip.
(function () {
  "use strict";

  var SPEED = 24; // pixels per second

  function measure(box) {
    var mq = box.firstElementChild;
    if (!mq) return;
    var over = Math.ceil(mq.scrollWidth - box.clientWidth);
    if (over > 1) {
      box.setAttribute("data-over", "");
      box.style.setProperty("--mq-d", over + "px");
      box.style.setProperty("--mq-t", Math.max(4, over / SPEED + 2.4).toFixed(1) + "s");
      if (!box.title) box.title = mq.textContent.replace(/\s+/g, " ").trim();
    } else {
      box.removeAttribute("data-over");
    }
  }

  // One observer for every box: it measures a box when it appears or changes
  // size (a dialog opening, the phone turning, a swapped list).
  var ro = typeof ResizeObserver === "function" ? new ResizeObserver(function (entries) {
    entries.forEach(function (e) { measure(e.target); });
  }) : null;

  function scan(root) {
    (root || document).querySelectorAll(".marq").forEach(function (box) {
      if (ro) ro.observe(box);
      else measure(box);
    });
  }

  scan();
  document.addEventListener("DOMContentLoaded", function () { scan(); });
  document.body && document.body.addEventListener("htmx:afterSettle", function (evt) {
    scan(evt.detail && evt.detail.elt ? evt.detail.elt : null);
    scan();
  });
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(function () { scan(); });
})();
