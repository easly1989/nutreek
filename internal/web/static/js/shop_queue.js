// shop_queue.js: what you do on the shopping list answers at once.
//
// An action used to wait for the server: POST, a redirect, then the whole list again, which
// on a phone behind a proxy takes seconds. Now the page does the change on screen the moment
// the form is submitted and puts the request in a queue, which is kept in localStorage and
// sent in the background, one request at a time, in order. It covers the tick of a row, the
// ignore and restore of a row (also by swipe), the removal of an item added by hand, the
// quantity of a row, the edit of an item added by hand, and adding an item.
//   - a request sets a state («checked=1», «ignored=0») or a value, it does not flip: one that
//     the server answered late and the browser sent again does no harm; an added item carries
//     an id of the page (cid), which the server remembers for a day;
//   - a request that fails (no network, no answer in 15 s, a 5xx) stays in the queue and is
//     tried again, after 1 s, 2 s, 4 s ... up to 30 s, and at once when the phone is online
//     again or the page comes back to the foreground; the queue survives a closed page;
//   - a session that expired or a CSRF token that is too old (a redirect, 401, 403) keeps the
//     queue too, and the note asks for a reload, which brings a fresh token and sends it;
//   - a row the server no longer has (404) or a request it refuses (another 4xx) is dropped,
//     so it does not hold the ones behind it;
//   - when the queue is empty the page takes the list from the server once, which moves the
//     rows between the sections and brings the numbers up to date;
//   - when a request has failed, a note says how many changes wait to be sent.
// What the server computes (merging two rows, copying a list, regenerating, the check before
// shopping, the grouping) is not queued: those forms go through htmx and the page shows a bar
// while they wait. A form with a value the server would refuse is not queued either, so it
// gets the message it always did. Without scripts the forms still post.
(function () {
  "use strict";

  var KEY = "nutreek.shop.queue";
  var TIMEOUT = 15000;
  var ROW = /^\/shopping-list\/(items|user-items)\/(\d+)\/(toggle|ignore|quantity|edit|delete)$/;
  var ADD = /^\/shopping-list\/(\d+)\/user-items$/;
  var CHECK = '<svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><use href="#i-check"/></svg>';

  var queue = load(); // [{key, row, path, body, meta}]
  var busy = false;   // a request is out: queue[0]
  var failing = ""; // "" | "net" (try again later) | "auth" (needs a reload)
  var wait = 1000;
  var retry = null;
  var refreshTimer = null;
  var epoch = 0;          // counts the changes: a list fetched before the last one is stale
  var refreshEpoch = null; // the epoch a list request in flight was started in

  function validMeta(m) {
    if (m === undefined) return true;
    return !!m && typeof m === "object" && typeof m.act === "string" &&
      (m.act === "add" ? /^[\w-]{1,64}$/.test(String(m.cid)) && typeof m.name === "string" : (m.kind === "items" || m.kind === "user-items") && /^\d+$/.test(String(m.id)));
  }
  function valid(op) {
    return op && typeof op.key === "string" && typeof op.body === "string" && typeof op.path === "string" &&
      (ROW.test(op.path) || ADD.test(op.path)) && validMeta(op.meta);
  }
  // The first version of the queue stored only ticks: {kind, id, checked}.
  function upgrade(op) {
    if (op && op.kind && op.id && typeof op.checked === "boolean" && !op.path) {
      return { key: "toggle:" + op.kind + ":" + op.id, row: op.kind + ":" + op.id, path: "/shopping-list/" + op.kind + "/" + op.id + "/toggle",
        body: "checked=" + (op.checked ? "1" : "0"), meta: { act: "toggle", kind: op.kind, id: String(op.id), on: op.checked } };
    }
    return op;
  }
  function load() {
    try {
      var a = JSON.parse(localStorage.getItem(KEY) || "[]");
      return Array.isArray(a) ? a.map(upgrade).filter(valid) : [];
    } catch (e) { return []; }
  }
  function save() {
    try { localStorage.setItem(KEY, JSON.stringify(queue)); } catch (e) { /* private mode: the queue lives in this page only */ }
  }
  // Another tab of Nutreek writes the same key: take its queue when this page is not sending.
  window.addEventListener("storage", function (e) {
    if (e.key === KEY && !busy) { queue = load(); note(); send(); }
  });

  // ---- what the page shows while a change waits --------------------------------------------

  function formOf(kind, id, act) {
    return document.querySelector('#shop-list form[action^="/shopping-list/' + kind + "/" + id + "/" + act + '"]');
  }
  function liOf(kind, id) {
    var f = formOf(kind, id, "toggle") || formOf(kind, id, "ignore") || formOf(kind, id, "quantity") || formOf(kind, id, "edit");
    return f ? f.closest("li.swipe-row") : null;
  }

  // paintCheck shows a tick on a row: the pressed state and the mark in the box, as the server draws them.
  function paintCheck(btn, on) {
    if (!btn) return;
    btn.setAttribute("aria-pressed", on ? "true" : "false");
    var box = btn.querySelector(".shop-box");
    if (box) box.innerHTML = on ? CHECK : "";
  }
  function paintIgnore(li, on) { if (li) li.classList.toggle("is-ignored", on); }
  function paintDelete(li) { if (li) li.hidden = true; }
  function paintName(li, text) {
    var n = li && li.querySelector("[data-shop-name]");
    if (n) n.firstChild ? (n.firstChild.nodeValue = text) : (n.textContent = text);
  }
  // paintQty puts the new quantity in the two columns of the row: pieces and grams or millilitres.
  function paintQty(li, q) {
    if (!li) return;
    var pc = li.querySelector(".shop-pc"), gr = li.querySelector(".shop-gr");
    if (q.text !== undefined) { if (gr) gr.textContent = q.text; if (pc) pc.textContent = ""; return; }
    if (pc) pc.innerHTML = "";
    if (q.unit === "pz") {
      if (pc) { var b = document.createElement("b"); b.textContent = q.n + " " + q.label; pc.appendChild(b); }
      if (gr) gr.textContent = q.grams ? q.grams + " " + (q.base || "g") : "";
    } else if (gr) {
      gr.textContent = q.n + " " + q.unit;
    }
  }
  function placeholder(meta) {
    // The list of the rows still to buy: the first one that is not inside a closed section.
    var lists = document.querySelectorAll("#shop-list > .card ul"), list = null;
    for (var i = 0; i < lists.length && !list; i++) if (!lists[i].closest("details")) list = lists[i];
    if (!list || document.querySelector('#shop-list [data-pending-add="' + meta.cid + '"]')) return;
    var li = document.createElement("li");
    li.className = "swipe-row";
    li.setAttribute("data-pending-add", meta.cid);
    li.style.opacity = ".6";
    var fg = document.createElement("div");
    fg.className = "swipe-fg flex items-center pr-2 bg-surface";
    var row = document.createElement("div");
    row.className = "shop-item grow min-w-0";
    var box = document.createElement("span"); box.className = "shop-box"; box.setAttribute("aria-hidden", "true");
    var nm = document.createElement("span"); nm.className = "shop-name";
    var n = document.createElement("span"); n.className = "shop-nm block"; n.textContent = meta.name;
    nm.appendChild(n);
    var qty = document.createElement("span"); qty.className = "shop-qty";
    var g = document.createElement("span"); g.className = "shop-gr"; g.textContent = meta.qty || "";
    qty.appendChild(g);
    row.appendChild(box); row.appendChild(nm); row.appendChild(qty);
    fg.appendChild(row); li.appendChild(fg);
    list.insertBefore(li, list.firstChild);
  }

  function paintOp(op) {
    var m = op.meta;
    if (!m) return;
    if (m.act === "add") { placeholder(m); return; }
    var li = liOf(m.kind, m.id);
    if (m.act === "toggle") paintCheck(document.getElementById((m.kind === "items" ? "shop-item-" : "shop-user-") + m.id), m.on);
    else if (m.act === "ignore") paintIgnore(li, m.on);
    else if (m.act === "delete") paintDelete(li);
    else if (m.act === "quantity" || m.act === "edit") { if (m.name !== undefined) paintName(li, m.name); if (m.q) paintQty(li, m.q); }
  }
  // The list the server sends does not know the changes still in the queue: show them again.
  function reapply() {
    queue.forEach(function (op) { try { paintOp(op); } catch (e) { /* a row that is not on this page */ } });
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

  // ---- sending -------------------------------------------------------------------------------

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
    window.htmx.ajax("GET", location.pathname + location.search, { target: "#shop-list", select: "#shop-list", swap: "outerHTML", headers: { "X-Shop-Refresh": "1" } });
  }

  // A list requested before the last change does not know it: do not swap it in (the answer to
  // that change asks for a newer one).
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
    fetch(op.path, {
      method: "POST",
      credentials: "same-origin",
      redirect: "manual", // a session that expired answers with a redirect to the login: not a success
      keepalive: op.body.length < 8000,
      signal: ctl ? ctl.signal : undefined,
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Shop-Queue": "1" },
      body: (op.body ? op.body + "&" : "") + "csrf_token=" + encodeURIComponent(token)
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

  // enqueue adds a request. Without sameRow, a change not sent yet that has the same key is
  // replaced; with it (a delete) every change not sent yet to that row goes. The request on its way stays.
  function enqueue(op, sameRow) {
    if (!busy) queue = load(); // another tab may have added or sent changes meanwhile
    queue = queue.filter(function (o, i) {
      if (i === 0 && busy) return true;
      return o.key !== op.key && !(sameRow && o.row === op.row);
    });
    queue.push(op);
    epoch++;
    save();
    note();
    clearTimeout(refreshTimer);
    send();
  }

  // ---- reading a form ----------------------------------------------------------------------

  // The same numbers the server reads (a comma or a dot, no hex or exponent): 0 for none, NaN for a refusal.
  function parseQty(raw) {
    raw = String(raw || "").trim().replace(",", ".");
    if (raw === "") return 0;
    if (!/^(\d+\.?\d*|\.\d+)$/.test(raw)) return NaN;
    var n = Number(raw);
    return isFinite(n) && n > 0 && n <= 100000 ? n : NaN;
  }
  function baseUnit(form) {
    var r = form.querySelector("input[name=qty_unit]:not([value=pz])");
    return r ? r.value : "g";
  }
  function body(form, extra) {
    var p = new URLSearchParams(new FormData(form));
    p.delete("csrf_token");
    Object.keys(extra || {}).forEach(function (k) { p.set(k, extra[k]); });
    return p.toString();
  }
  function unitOf(form) {
    var r = form.querySelector("input[name=qty_unit]:checked");
    if (!r) return { unit: "g", label: "g" };
    var l = r.closest("label");
    return { unit: r.value, label: (l ? l.textContent : r.value).trim() };
  }
  function closeSheet(form) {
    var d = form.closest("details[data-sheet]");
    if (d) d.open = false;
  }
  function closeMenu(form) {
    var m = form.closest(".menu-panel");
    if (!m) return;
    m.classList.add("hidden");
    var b = document.querySelector('[data-popover="' + m.id + '"]');
    if (b) b.setAttribute("aria-expanded", "false");
  }
  function num(n) { return n.toLocaleString(document.documentElement.lang || undefined, { maximumFractionDigits: 1 }); }

  // build turns a submitted form into a queued change and shows it. It returns false for a form it
  // does not take (the page then does what it always did).
  function build(form) {
    var path = new URL(form.getAttribute("action") || "", location.href).pathname;
    var a = ADD.exec(path);
    if (a) {
      var desc = (form.elements.description ? form.elements.description.value : "").trim();
      var q = parseQty(form.elements.quantity ? form.elements.quantity.value : "");
      if (!desc || desc.length > 200 || isNaN(q)) return false;
      var cid = (window.crypto && crypto.randomUUID) ? crypto.randomUUID() : String(Date.now()) + Math.random().toString(16).slice(2);
      var unit = form.elements.unit ? form.elements.unit.value : "g";
      var qtext = q ? num(q) + " " + (unit === "pz" ? (form.elements.unit.selectedOptions[0].textContent.trim()) : unit) : "";
      var op = { key: "add:" + cid, row: "add:" + cid, path: path, body: body(form, { cid: cid }), meta: { act: "add", cid: cid, name: desc, qty: qtext } };
      var dlg = form.closest("dialog");
      form.reset();
      var sg = document.getElementById("shop-suggest"); if (sg) sg.innerHTML = "";
      if (dlg && dlg.close) dlg.close();
      placeholder(op.meta);
      enqueue(op, false);
      return true;
    }
    var m = ROW.exec(path);
    if (!m) return false;
    var kind = m[1], id = m[2], act = m[3];
    var row = kind + ":" + id;
    var li = form.closest("li.swipe-row");
    if (act === "toggle") {
      var btn = form.querySelector("button.shop-item");
      if (!btn) return false;
      var on = btn.getAttribute("aria-pressed") !== "true";
      paintCheck(btn, on);
      enqueue({ key: "toggle:" + row, row: row, path: path, body: "checked=" + (on ? "1" : "0"), meta: { act: "toggle", kind: kind, id: id, on: on } }, false);
      return true;
    }
    if (act === "ignore") {
      if (!li) return false;
      var ign = !li.classList.contains("is-ignored");
      paintIgnore(li, ign);
      closeMenu(form);
      enqueue({ key: "ignore:" + row, row: row, path: path, body: "ignored=" + (ign ? "1" : "0"), meta: { act: "ignore", kind: kind, id: id, on: ign } }, false);
      return true;
    }
    if (act === "delete") {
      paintDelete(li);
      closeMenu(form);
      enqueue({ key: "delete:" + row, row: row, path: path, body: "", meta: { act: "delete", kind: kind, id: id } }, true);
      return true;
    }
    if (act === "quantity" || act === "edit") {
      var el = form.elements, meta = { act: act, kind: kind, id: id };
      if (act === "quantity") {
        var n = parseQty(el.quantity.value);
        if (!n || isNaN(n)) return false;
        var u = unitOf(form);
        var w = Number(form.dataset.unitWeight) || 0;
        if (u.unit === "pz" && w && n * w > 1000000) return false; // more than the list accepts: the server says so
        meta.q = { n: num(n), unit: u.unit, label: u.label, grams: u.unit === "pz" && w ? num(n * w) : "", base: baseUnit(form) };
      } else {
        var d = (el.description ? el.description.value : "").trim();
        if (!d || d.length > 200) return false;
        meta.name = d;
        if (el.quantity_text) {
          if (el.quantity_text.value.length > 200) return false;
          meta.q = { text: el.quantity_text.value.trim() };
        } else {
          var n2 = parseQty(el.quantity ? el.quantity.value : "");
          if (isNaN(n2)) return false;
          var u2 = unitOf(form);
          meta.q = n2 ? { n: num(n2), unit: u2.unit, label: u2.label, grams: "" } : { text: "" };
        }
      }
      closeSheet(form);
      paintOp({ meta: meta });
      enqueue({ key: act + ":" + row, row: row, path: path, body: body(form), meta: meta }, false);
      return true;
    }
    return false;
  }

  // Capture phase on the document: it runs before htmx, which listens on the form itself.
  document.addEventListener("submit", function (e) {
    var f = e.target;
    if (!f || !f.matches || !f.matches("form[action^='/shopping-list/']")) return;
    if (!build(f)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
  }, true);

  document.addEventListener("htmx:afterSwap", function () { reapply(); note(); });
  window.addEventListener("online", function () { wait = 1000; send(); });
  document.addEventListener("visibilitychange", function () { if (document.visibilityState === "visible") { wait = 1000; send(); } });
  window.addEventListener("pageshow", function () { send(); });

  // swipe.js asks for the same forms the buttons use, so a swipe is queued like a tap.
  window.nutreekShopQueue = {
    submit: function (li, act) {
      var f = li.querySelector('form[action*="/' + act + '"]');
      if (!f || !f.requestSubmit) return false;
      f.requestSubmit();
      return true;
    }
  };

  reapply();
  send();
})();
