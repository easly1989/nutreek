// shop_progress.js: the progress card of the shopping list (#248).
//
// Every tick swaps the whole list, so the card is a new element each time. This
// script remembers the numbers of the last card it saw and, when the same list comes
// back with more items in the cart, fills the bar from the old value to the new one.
// When the cart gets every item the bar's shine runs, the «Spesa completata» line
// pops in and confetti leaves the end of the bar.
//
// Purpose: state indication (the fill) and delight, but only for the rare moment of
// finishing a list. With «reduce motion» nothing moves: the card is as the server
// drew it, with the «Spesa completata» line.
(function () {
  "use strict";

  var EASE = "cubic-bezier(0.23, 1, 0.32, 1)";
  var FILL_MS = 320;
  var calm = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)");
  // Mid tones, readable on the light and on the dark theme, next to the accent.
  var COLORS = ["#E9B851", "#A99DF0", "#5CC9BB", "#FF8A80"];

  // read takes the numbers from the progressbar, not from the card: htmx gives a
  // swapped element that has an id its old attributes until it settles.
  function read() {
    var card = document.getElementById("shop-progress");
    var bar = card && card.querySelector("[role=progressbar]");
    if (!bar) return null;
    return {
      card: card,
      list: bar.dataset.list,
      done: parseInt(bar.getAttribute("aria-valuenow"), 10) || 0,
      total: parseInt(bar.getAttribute("aria-valuemax"), 10) || 0
    };
  }

  // A polite live region, filled when the cart gets everything: the card is new
  // on every tick, so a line inside it is not announced.
  var live = null;
  function announce(text) {
    if (!live) {
      live = document.createElement("div");
      live.setAttribute("role", "status");
      live.style.cssText = "position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap";
      document.body.appendChild(live);
    }
    live.textContent = "";
    setTimeout(function () { live.textContent = text; }, 50);
  }

  function clip(done, total) {
    var rest = total > 0 ? 100 - (done * 100) / total : 100;
    return "inset(0 " + rest + "% 0 0 round 9999px)";
  }

  function fill(prev, now) {
    var bar = now.card.querySelector("[data-progress-fill]");
    if (!bar || !bar.animate) return;
    bar.animate(
      [{ clipPath: clip(prev.done, prev.total) }, { clipPath: clip(now.done, now.total) }],
      { duration: FILL_MS, easing: EASE }
    );
  }

  // sheen passes over the full bar, once.
  function sheen(card) {
    var track = card.querySelector("[role=progressbar]");
    if (!track || !track.animate) return;
    var s = document.createElement("span");
    s.setAttribute("aria-hidden", "true");
    s.style.cssText = "position:absolute;top:0;bottom:0;left:0;width:60px;" +
      "background:linear-gradient(90deg,transparent,rgba(255,255,255,.7),transparent);transform:skewX(-20deg)";
    track.appendChild(s);
    var a = s.animate(
      [{ transform: "translateX(-80px) skewX(-20deg)" }, { transform: "translateX(" + (track.clientWidth + 20) + "px) skewX(-20deg)" }],
      { duration: 700, easing: EASE }
    );
    a.onfinish = function () { s.remove(); };
  }

  function popIn(card) {
    var line = card.querySelector("[data-progress-done]");
    if (!line || !line.animate) return;
    line.animate(
      [{ opacity: 0, transform: "scale(0.9)" }, { opacity: 1, transform: "scale(1)" }],
      { duration: 240, easing: EASE, delay: 300, fill: "backwards" }
    );
  }

  // confetti throws pieces from the end of the bar, up and then down, fading.
  function confetti(card) {
    var track = card.querySelector("[role=progressbar]");
    if (!track || !card.animate) return;
    var cr = card.getBoundingClientRect(), tr = track.getBoundingClientRect();
    var ox = tr.right - cr.left - 4, oy = tr.top - cr.top + tr.height / 2;
    // Keep the pieces inside the screen: a piece past the edge would widen the page.
    var minX = -(tr.right - 8), maxX = document.documentElement.clientWidth - tr.right - 8;
    var colors = [getComputedStyle(card).getPropertyValue("--accent").trim() || "#2F7A55"].concat(COLORS);
    var layer = document.createElement("div");
    layer.setAttribute("aria-hidden", "true");
    layer.style.cssText = "position:absolute;left:0;top:0;width:0;height:0;pointer-events:none;z-index:30";
    card.appendChild(layer);
    var pending = 36;
    for (var i = 0; i < pending; i++) {
      var p = document.createElement("i");
      p.style.cssText = "position:absolute;left:" + ox + "px;top:" + oy + "px;width:6px;height:4px;border-radius:1px;background:" + colors[i % colors.length];
      layer.appendChild(p);
      var spread = (Math.random() - 0.5) * 2; // -1 (left) .. 1 (right)
      var dx = Math.min(maxX, Math.max(minX, spread * 130 - 40));
      var up = 50 + Math.random() * 70;
      var down = 40 + Math.random() * 60;
      var turn = (Math.random() * 720 - 360) + "deg";
      var anim = p.animate([
        { transform: "translate(0,0) rotate(0)", opacity: 1, easing: "cubic-bezier(0.23, 1, 0.32, 1)" },
        { transform: "translate(" + dx * 0.7 + "px," + -up + "px) rotate(" + turn + ")", opacity: 1, offset: 0.4, easing: "cubic-bezier(0.55, 0, 1, 0.45)" },
        { transform: "translate(" + dx + "px," + down + "px) rotate(" + turn + ")", opacity: 0 }
      ], { duration: 1100 + Math.random() * 500, delay: Math.random() * 120, fill: "both" });
      anim.onfinish = function () { if (--pending === 0) layer.remove(); };
    }
  }

  var last = read();

  // The page works the numbers out itself after a tick (shop_queue.js): compare with those next time,
  // so the list that comes from the server later does not play the fill again.
  document.addEventListener("shop:progress", function () { last = read(); });

  // afterSwap, not afterSettle: the settle comes 20 ms later and the new card
  // would show its final state for a frame before the animation starts.
  document.addEventListener("htmx:afterSwap", function () {
    var now = read();
    var prev = last;
    last = now;
    if (!now || !prev || now.list !== prev.list) return;
    if (now.done === prev.done && now.total === prev.total) return;
    var finished = now.done > prev.done && now.done === now.total && prev.done < prev.total;
    if (finished) {
      var line = now.card.querySelector("[data-progress-done]");
      if (line) announce(line.textContent.trim());
    }
    if (calm && calm.matches) return;
    fill(prev, now);
    if (finished) {
      sheen(now.card);
      popIn(now.card);
      confetti(now.card);
    }
  });
})();
