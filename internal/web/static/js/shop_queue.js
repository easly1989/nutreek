// shop_queue.js: ticking a row of the shopping list answers at once.
//
// A tick used to wait for the server: POST, a redirect, then the whole list again, which
// on a phone behind a proxy takes seconds. Now the page paints the tick the moment the
// form is submitted and puts the change in a queue, which is kept in localStorage and
// sent in the background, one request at a time, in order:
//   - the request sets the state («checked=1»), it does not flip it, so a request the
//     server answered late and the browser sent again does no harm;
//   - a request that fails (no network, no answer in 15 s, a 5xx) stays in the queue and is
//     tried again, after 1 s, 2 s, 4 s ... up to 30 s, and at once when the phone is online
//     again or the page comes back to the foreground; the queue survives a closed page;
//   - a session that expired or a CSRF token that is too old (a redirect, 401, 403) keeps the
//     queue too, and the note asks for a reload, which brings a fresh token and sends it;
//   - a row the server no longer has (404) or a request it refuses (another 4xx) is dropped,
//     so it does not hold the ones behind it;
//   - when the queue is empty the page takes the list from the server once, which moves the
//     row to the cart and brings the progress up to date;
//   - when a request has failed, a note says how many changes wait to be sent.
// Without scripts the form still posts and the server flips the tick.
(function () {
  "use strict";

  var KEY = "nutreek.shop.queue";
  var TIMEOUT = 15000;
  var CHECK = '<svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><use href="#i-check"/></svg>';

  var queue = load(); // [{kind: "items" | "user-items", id: "12", checked: true}]
  var busy = false;   // a request is out: queue[0]
  var failing = ""; // "" | "net" (try again later) | "auth" (needs a reload)
  var wait = 1000;
  var retry = null;
  var refreshTimer = null;
  var epoch = 0;          // counts the taps: a list fetched before the last one is stale
  var refreshEpoch = null; // the epoch a list request in flight was started in

  function valid(op) {
    return op && (op.kind === "items" || op.kind === "user-items") && /^\d+$/.test(String(op.id)) && typeof op.checked === "boolean";
  }
  function load() {
    try {
      var a = JSON.parse(localStorage.getItem(KEY) || "[]");
      return Array.isArray(a) ? a.filter(valid) : [];
    } catch (e) { return []; }
  }
  function save() {
    try { localStorage.setItem(KEY, JSON.stringify(queue)); } catch (e) { /* private mode: the queue lives in this page only */ }
  }
  // Another tab of Nutreek writes the same key: take its queue when this page is not sending.
  window.addEventListener("storage", function (e) {
    if (e.key === KEY && !busy) { queue = load(); note(); send(); }
  });

  function button(op) {
    return document.getElementById((op.kind === "items" ? "shop-item-" : "shop-user-") + op.id);
  }

  // paint shows a tick on a row: the pressed state and the mark in the box, as the server draws them.
  function paint(btn, on) {
    if (!btn) return;
    btn.setAttribute("aria-pressed", on ? "true" : "false");
    var box = btn.querySelector(".shop-box");
    if (box) box.innerHTML = on ? CHECK : "";
  }

  // The list the server sends does not know the changes still in the queue: show them again.
  function reapply() {
    queue.forEach(function (op) { paint(button(op), op.checked); });
  }

  function note() {
    var el = document.querySelector("[data-shop-sync]");
    if (!el) return;
    var n = queue.length;
    el.hidden = !(failing && n > 0);
    if (el.hidden) return;
    var t = failing === "auth" ? el.dataset.auth : (n === 1 ? el.dataset.one : el.dataset.many);
    el.textContent = t.replace("{n}", String(n));
  }

  function csrf() {
    var f = document.querySelector("input[name=csrf_token]");
    return f ? f.value : "";
  }

  function schedule() {
    clearTimeout(retry);
    retry = setTimeout(send, wait);
    wait = Math.min(wait * 2, 30000);
  }

  // refresh takes the list from the server once the queue is empty. It waits while a quantity
  // editor is open or a field of the list has the focus, so it does not wipe what is typed.
  function refresh() {
    if (queue.length || busy || !window.htmx || !document.getElementById("shop-list")) return;
    var a = document.activeElement;
    if (document.querySelector("#shop-list details[data-sheet][open]") || (a && a.matches && a.matches("#shop-list input, #shop-list select, #shop-list textarea"))) {
      clearTimeout(refreshTimer);
      refreshTimer = setTimeout(refresh, 2000);
      return;
    }
    refreshEpoch = epoch;
    window.htmx.ajax("GET", location.pathname + location.search, { target: "#shop-list", select: "#shop-list", swap: "outerHTML" });
  }

  // A list requested before the last tap does not know it: do not swap it in (the answer to
  // that tap asks for a newer one).
  document.addEventListener("htmx:beforeSwap", function (e) {
    if (refreshEpoch !== null && refreshEpoch !== epoch && e.detail && e.detail.target && e.detail.target.id === "shop-list") {
      e.detail.shouldSwap = false;
    }
    refreshEpoch = null;
  });

  function drop() {
    queue.shift();
    save();
    wait = 1000;
    if (queue.length) { send(); return; }
    failing = "";
    note();
    clearTimeout(refreshTimer);
    refreshTimer = setTimeout(refresh, 350);
  }

  function send() {
    clearTimeout(retry);
    if (busy || !queue.length) return;
    var token = csrf();
    if (!token) return; // a page without forms: the next page that has the token sends it
    var op = queue[0];
    busy = true;
    var ctl = window.AbortController ? new AbortController() : null;
    var timer = ctl ? setTimeout(function () { ctl.abort(); }, TIMEOUT) : null;
    fetch("/shopping-list/" + op.kind + "/" + op.id + "/toggle", {
      method: "POST",
      credentials: "same-origin",
      redirect: "manual", // a session that expired answers with a redirect to the login: not a success
      keepalive: true,
      signal: ctl ? ctl.signal : undefined,
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Shop-Queue": "1" },
      body: "csrf_token=" + encodeURIComponent(token) + "&checked=" + (op.checked ? "1" : "0")
    }).then(function (r) {
      clearTimeout(timer);
      busy = false;
      if (r.status === 204) { drop(); return; }
      if (r.type === "opaqueredirect" || r.status === 401 || r.status === 403) { // signed out, or an old token
        failing = "auth"; note(); schedule(); return;
      }
      if (r.status >= 400 && r.status < 500) { drop(); return; } // 404: the row is gone; any other 4xx: it will not be accepted
      failing = "net"; note(); schedule(); // 5xx
    }).catch(function () {
      clearTimeout(timer);
      busy = false;
      failing = "net";
      note();
      schedule();
    });
  }

  function enqueue(kind, id, checked) {
    if (!busy) queue = load(); // another tab may have added or sent changes meanwhile
    // A change not sent yet to the same row is replaced by this one; the one on its way stays.
    queue = queue.filter(function (op, i) { return !(op.kind === kind && op.id === id && !(i === 0 && busy)); });
    queue.push({ kind: kind, id: id, checked: checked });
    epoch++;
    save();
    note();
    clearTimeout(refreshTimer);
    send();
  }

  // Capture phase on the document: it runs before htmx, which listens on the form itself.
  document.addEventListener("submit", function (e) {
    var f = e.target;
    if (!f || !f.matches || !f.matches("form[data-shop-check]")) return;
    var btn = f.querySelector("button.shop-item");
    if (!btn) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    var on = btn.getAttribute("aria-pressed") !== "true";
    paint(btn, on);
    enqueue(f.dataset.shopCheck, f.dataset.id, on);
  }, true);

  document.addEventListener("htmx:afterSwap", function () { reapply(); note(); });
  window.addEventListener("online", function () { wait = 1000; send(); });
  document.addEventListener("visibilitychange", function () { if (document.visibilityState === "visible") { wait = 1000; send(); } });
  window.addEventListener("pageshow", function () { send(); });

  reapply();
  send();
})();
