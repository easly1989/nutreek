// shop_queue.js: ticking a row of the shopping list answers at once.
//
// A tick used to wait for the server: POST, a redirect, then the whole list again, which
// on a phone behind a proxy takes seconds. Now the page paints the tick the moment the
// form is submitted and puts the change in a queue, which is kept in localStorage and
// sent in the background, one request at a time, in order:
//   - the request sets the state («checked=1»), it does not flip it, so a request the
//     server answered late and the browser sent again does no harm;
//   - a request that fails (no network, a 5xx, a session that expired) stays in the queue and
//     is tried again, after 1 s, 2 s, 4 s ... up to 30 s, and at once when the phone is online
//     again or the page comes back to the foreground; the queue survives a closed page;
//   - a row the server no longer has (404) is dropped;
//   - when the queue is empty the page takes the list from the server once, which moves the
//     row to the cart and brings the progress up to date;
//   - when a request has failed, a note says how many changes wait to be sent.
// Without scripts the form still posts and the server flips the tick.
(function () {
  "use strict";

  var KEY = "nutreek.shop.queue";
  var CHECK = '<svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><use href="#i-check"/></svg>';

  var queue = load(); // [{kind: "items" | "user-items", id: "12", checked: true}]
  var busy = false;   // a request is out: queue[0]
  var failing = false;
  var wait = 1000;
  var retry = null;
  var refreshTimer = null;

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
    if (!el.hidden) el.textContent = (n === 1 ? el.dataset.one : el.dataset.many).replace("{n}", String(n));
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

  function refresh() {
    if (queue.length || busy || !window.htmx || !document.getElementById("shop-list")) return;
    window.htmx.ajax("GET", location.pathname + location.search, { target: "#shop-list", select: "#shop-list", swap: "outerHTML" });
  }

  function send() {
    clearTimeout(retry);
    if (busy || !queue.length) return;
    var token = csrf();
    if (!token) { failing = true; note(); schedule(); return; }
    var op = queue[0];
    busy = true;
    fetch("/shopping-list/" + op.kind + "/" + op.id + "/toggle", {
      method: "POST",
      credentials: "same-origin",
      redirect: "manual", // a session that expired answers with a redirect to the login: not a success
      keepalive: true,
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Shop-Queue": "1" },
      body: "csrf_token=" + encodeURIComponent(token) + "&checked=" + (op.checked ? "1" : "0")
    }).then(function (r) {
      busy = false;
      if (r.status === 204 || r.status === 404) { // done, or the row is gone
        queue.shift();
        save();
        failing = false;
        wait = 1000;
        note();
        if (queue.length) send();
        else { clearTimeout(refreshTimer); refreshTimer = setTimeout(refresh, 350); }
        return;
      }
      throw new Error("status " + r.status);
    }).catch(function () {
      busy = false;
      failing = true;
      note();
      schedule();
    });
  }

  function enqueue(kind, id, checked) {
    // A change not sent yet to the same row is replaced by this one; the one on its way stays.
    queue = queue.filter(function (op, i) { return !(op.kind === kind && op.id === id && !(i === 0 && busy)); });
    queue.push({ kind: kind, id: id, checked: checked });
    save();
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
