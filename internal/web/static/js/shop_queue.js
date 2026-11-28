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
//   - a tick also moves the row between the list and the cart on screen, and brings the counts of
//     the sections and the progress bar up to date, so nothing waits for the server; the page
//     takes the list from the server once things have been quiet for 6 s (at once, 350 ms after
//     the queue empties, for the actions that need what the server computes);
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
  var IDLE = 6000;        // how long things stay quiet before the list is taken from the server
  var fast = false;       // a change the page could not finish on screen: take the list soon
  var epoch = 0;          // counts the changes: a list fetched before the last one is stale

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
  // ---- a tick moves the row: list <-> cart ---------------------------------------------------

  function section(name) { return document.querySelector('#shop-list details[data-shop-acc="' + name + '"]'); }
  function nameOf(li) {
    var n = li.querySelector("[data-shop-name]");
    return (n ? n.textContent : "").trim().toLowerCase();
  }
  function insertSorted(ul, li) {
    var nm = nameOf(li), kids = ul.querySelectorAll(":scope > li.swipe-row");
    for (var i = 0; i < kids.length; i++) {
      if (kids[i] !== li && nameOf(kids[i]).localeCompare(nm) > 0) { ul.insertBefore(li, kids[i]); return; }
    }
    ul.appendChild(li);
  }
  function visibleRows(ul) { return ul.querySelectorAll(":scope > li.swipe-row:not([hidden])"); }

  // updateSection brings the heading of a closed section up to date: the count and the first names.
  function updateSection(d) {
    var ul = d.querySelector(":scope > ul"), rows = ul ? visibleRows(ul) : [];
    if (!rows.length) { d.remove(); return; }
    var sum = d.querySelector(":scope > summary");
    if (!sum) return;
    for (var i = 0; i < sum.childNodes.length; i++) {
      var t = sum.childNodes[i];
      if (t.nodeType === 3 && /·\s*\d+/.test(t.nodeValue)) { t.nodeValue = t.nodeValue.replace(/·\s*\d+/, "· " + rows.length); break; }
    }
    var pv = sum.querySelector(".basis-full");
    if (pv) {
      var names = [];
      for (var k = 0; k < rows.length && k < 3; k++) {
        var n = rows[k].querySelector("[data-shop-name]");
        names.push(n ? n.textContent.trim() : "");
      }
      pv.textContent = names.join(", ");
      if (rows.length > 3) {
        var b = document.createElement("b");
        b.className = "font-semibold text-accent";
        b.textContent = "+" + (rows.length - 3);
        pv.appendChild(document.createTextNode(" "));
        pv.appendChild(b);
      }
    }
  }

  // recount follows the rows in the headings and hides a group with none left.
  function recount() {
    document.querySelectorAll("#shop-list [data-shop-count]").forEach(function (c) {
      var ul = c.closest("h2") && c.closest("h2").nextElementSibling;
      if (!ul) return;
      var n = String(visibleRows(ul).length);
      c.textContent = n;
      if (c.dataset.total) c.dataset.total = n; // shop_sections.js shows this when no search is on
    });
    document.querySelectorAll("#shop-list [data-shop-group]").forEach(function (g) {
      g.hidden = !g.querySelector("li.swipe-row:not([hidden])");
    });
  }

  // updateProgress takes the numbers of the progress card from the rows in the cart.
  function updateProgress() {
    var bar = document.querySelector("#shop-progress [role=progressbar]");
    var cart = section("cart");
    if (!bar) return;
    var total = parseInt(bar.getAttribute("aria-valuemax"), 10) || 0;
    var done = cart ? visibleRows(cart.querySelector(":scope > ul")).length : 0;
    var pct = total ? Math.floor(done * 100 / total) : 0;
    bar.setAttribute("aria-valuenow", String(done));
    var fill = bar.querySelector("[data-progress-fill]");
    if (fill) { fill.style.transition = "clip-path 300ms cubic-bezier(0.23, 1, 0.32, 1)"; fill.style.clipPath = "inset(0 " + (100 - pct) + "% 0 0 round 9999px)"; }
    var card = bar.closest("#shop-progress");
    var b = card && card.querySelector("b");
    if (b) b.textContent = b.textContent.replace(/\d+/, String(done));
    var p = card && card.querySelector("span.ml-auto");
    if (p) p.textContent = pct + "%";
  }

  // moveRow puts a ticked row in the cart and an unticked one back in the list, and brings the
  // counts and the progress along. It returns false when it could not finish (no cart section yet,
  // the list grouped by category, the last row leaving the list): the list is then taken soon.
  function moveRow(li, on) {
    if (!li) return false;
    var cart = section("cart"), inCart = !!li.closest("details[data-shop-acc=cart]");
    if (on === inCart) return true;
    var from = li.parentNode, ok = true;
    if (on) {
      if (!cart) return false;
      insertSorted(cart.querySelector(":scope > ul"), li);
    } else {
      var card = document.querySelector("#shop-list > .card"), ul = card && card.querySelector(":scope > ul");
      if (!ul) return false; // grouped by category: the group of the row is not known here
      insertSorted(ul, li);
    }
    if (cart && from && from.closest("details[data-shop-acc=cart]")) updateSection(cart);
    if (on && cart) updateSection(cart);
    if (on && from && !from.closest("details") && !visibleRows(from).length && !from.closest("[data-shop-group]")) ok = false; // the list is empty: the server draws its message
    recount();
    updateProgress();
    return ok;
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
    if (m.act === "toggle") {
      var tb = document.getElementById((m.kind === "items" ? "shop-item-" : "shop-user-") + m.id);
      paintCheck(tb, m.on);
      if (tb && !moveRow(tb.closest("li.swipe-row"), m.on)) fast = true;
    }
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

  // refresh takes the list from the server once the queue is empty, and changes the page only where
  // the list differs from what it shows (a row that is the same is left alone), so the page does not
  // rebuild a few hundred rows for a tap. It waits while a quantity editor is open or a field of
  // the list has the focus, so it does not wipe what is typed.
  function refresh() {
    if (queue.length || busy || !document.getElementById("shop-list")) return;
    var a = document.activeElement;
    if (document.querySelector("#shop-list details[data-sheet][open]") || (a && a.matches && a.matches("#shop-list input, #shop-list select, #shop-list textarea"))) {
      scheduleRefresh(2000);
      return;
    }
    var mine = epoch;
    fetch(location.pathname + location.search, { credentials: "same-origin", redirect: "manual", headers: { "X-Shop-Refresh": "1" } })
      .then(function (r) { if (r.status !== 200) throw new Error("status " + r.status); return r.text(); })
      .then(function (html) {
        if (mine !== epoch || queue.length || busy) return; // a change came meanwhile: its own refresh asks again
        var next = new DOMParser().parseFromString("<body>" + html + "</body>", "text/html").getElementById("shop-list");
        var cur = document.getElementById("shop-list");
        if (!next || !cur) return;
        // Every response has a new CSRF token in its forms; any of them is good for the session, so
        // take the one the page has and a row that did not change is equal to the one on screen.
        var tok = csrf();
        if (tok) {
          next.querySelectorAll("input[name=csrf_token]").forEach(function (i) { i.setAttribute("value", tok); });
          next.querySelectorAll("[data-csrf]").forEach(function (e) { e.setAttribute("data-csrf", tok); });
        }
        morph(cur, next);
        cur.dispatchEvent(new CustomEvent("htmx:afterSwap", { bubbles: true, detail: { target: cur } }));
        cur.dispatchEvent(new CustomEvent("htmx:afterSettle", { bubbles: true, detail: { target: cur } }));
      })
      .catch(function () { /* the next change, or the next visit, takes the list */ });
  }

  // ---- changing only what differs ------------------------------------------------------------

  // key names the elements that keep their identity from one list to the next: a row, an element
  // with an id, a section.
  function key(el) {
    if (el.nodeType !== 1) return null;
    var id = el.getAttribute("data-id");
    if (id) return "r:" + id;
    if (el.id) return "#" + el.id;
    var acc = el.getAttribute("data-shop-acc");
    return acc ? "a:" + acc : null;
  }
  function syncAttrs(o, n) {
    var i, a;
    for (i = o.attributes.length - 1; i >= 0; i--) {
      a = o.attributes[i].name;
      if (!n.hasAttribute(a) && !(a === "open" && o.tagName === "DETAILS")) o.removeAttribute(a);
    }
    for (i = 0; i < n.attributes.length; i++) {
      a = n.attributes[i];
      if (a.name === "open" && o.tagName === "DETAILS") continue; // the sections keep how the user left them
      if (o.getAttribute(a.name) !== a.value) o.setAttribute(a.name, a.value);
    }
  }
  function adopt(n) {
    var c = document.importNode(n, true);
    if (window.htmx && c.nodeType === 1) window.htmx.process(c); // the hx-* attributes of a new element
    return c;
  }
  // patch makes o look like n and returns the node that stays in the page.
  function patch(o, n) {
    if (o.nodeType !== 1 || n.nodeType !== 1) {
      if (o.nodeType === n.nodeType && o.nodeValue !== n.nodeValue) o.nodeValue = n.nodeValue;
      return o;
    }
    if (o.tagName !== n.tagName) { var c = adopt(n); o.replaceWith(c); return c; }
    if (o.isEqualNode(n)) return o;
    var k = key(o);
    if (k && k.charAt(0) === "r") { var r = adopt(n); o.replaceWith(r); return r; } // a row that changed is replaced whole
    syncAttrs(o, n);
    morphChildren(o, n);
    return o;
  }
  function morphChildren(oldP, newP) {
    var byKey = {}, loose = [], li = 0, kept = [];
    Array.prototype.forEach.call(oldP.childNodes, function (o) {
      var k = key(o);
      if (k) byKey[k] = o; else loose.push(o);
    });
    Array.prototype.forEach.call(newP.childNodes, function (n) {
      var k = key(n), o = null;
      if (k) o = byKey[k] || null;
      else {
        while (li < loose.length && !(loose[li].nodeType === n.nodeType && loose[li].nodeName === n.nodeName)) li++;
        if (li < loose.length) o = loose[li++];
      }
      kept.push(o ? patch(o, n) : adopt(n));
    });
    var pos = oldP.firstChild;
    kept.forEach(function (el) {
      if (el === pos) pos = pos.nextSibling;
      else oldP.insertBefore(el, pos);
    });
    while (pos) { var nx = pos.nextSibling; oldP.removeChild(pos); pos = nx; }
  }
  function morph(cur, next) {
    syncAttrs(cur, next);
    morphChildren(cur, next);
  }

  function drop() {
    queue.shift();
    save();
    wait = 1000;
    if (queue.length) { send(); return; }
    failing = "";
    note();
    scheduleRefresh(fast ? 350 : IDLE);
  }

  function scheduleRefresh(ms) {
    clearTimeout(refreshTimer);
    refreshTimer = setTimeout(function () { fast = false; refresh(); }, ms);
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
    if (!op.meta || op.meta.act !== "toggle") fast = true;
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
      if (!moveRow(btn.closest("li.swipe-row"), on)) fast = true;
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
