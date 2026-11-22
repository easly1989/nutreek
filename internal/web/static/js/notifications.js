// notifications.js: the notifications page (#237).
//
//  - The device card: it picks one of its states from what the browser can do
//    (secure context, service worker, Push API, the permission, an existing
//    subscription, iPhone outside the installed app) and runs its buttons: turn on
//    (ask the permission, subscribe with the server's VAPID key, send the device and
//    the time zone), try it, turn off.
//  - A switch on a reminder or on the quiet hours submits its form when it is changed.
(function () {
  "use strict";

  // The switches work without the device card.
  document.addEventListener("change", function (e) {
    var el = e.target;
    if (el instanceof Element && el.matches("input.notify-switch") && el.form) el.form.requestSubmit();
  });

  var card = document.querySelector("[data-notify]");
  if (!card || card.dataset.available !== "true") return;

  var states = card.querySelectorAll("[data-notify-state]");
  var errorBox = card.querySelector("[data-notify-error]");

  function show(name) {
    // A class, not the hidden attribute: the blocks are display:flex, which would win.
    states.forEach(function (s) { s.classList.toggle("hidden", s.dataset.notifyState !== name); });
  }

  // The box is red for an error and neutral for a result that went well.
  function say(msg, ok) {
    if (!errorBox) return;
    errorBox.textContent = msg;
    errorBox.hidden = false;
    errorBox.classList.toggle("bg-danger-soft", !ok);
    errorBox.classList.toggle("text-danger", !ok);
    errorBox.classList.toggle("bg-surface-2", !!ok);
    errorBox.classList.toggle("text-text", !!ok);
  }
  function fail(msg) { say(msg, false); }

  function post(url, body) {
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": card.dataset.csrf },
      body: JSON.stringify(body || {})
    });
  }

  // The key travels as base64url; subscribe() wants bytes.
  function keyBytes(b64) {
    var pad = "=".repeat((4 - (b64.length % 4)) % 4);
    var raw = atob((b64 + pad).replace(/-/g, "+").replace(/_/g, "/"));
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  var ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  var standalone = window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
  var capable = window.isSecureContext && "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

  function current() {
    return navigator.serviceWorker.ready.then(function (reg) { return reg.pushManager.getSubscription(); });
  }

  function refresh() {
    if (!capable) {
      show(ios && !standalone && window.isSecureContext ? "ios" : "unsupported");
      return;
    }
    if (Notification.permission === "denied") { show("blocked"); return; }
    current().then(function (sub) {
      show(sub && Notification.permission === "granted" ? "ok" : "todo");
    }).catch(function () { show("unsupported"); });
  }

  card.addEventListener("click", function (e) {
    var btn = e.target instanceof Element && e.target.closest("button");
    if (!btn) return;
    if (errorBox) errorBox.hidden = true;

    if (btn.hasAttribute("data-notify-enable")) {
      btn.disabled = true;
      Notification.requestPermission().then(function (perm) {
        if (perm !== "granted") { fail(card.dataset.msgDenied); refresh(); return null; }
        return navigator.serviceWorker.ready.then(function (reg) {
          return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(card.dataset.vapid) });
        }).then(function (sub) {
          var j = sub.toJSON();
          return post("/notifications/subscribe", {
            endpoint: j.endpoint,
            keys: j.keys,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
          }).then(function (r) { if (!r.ok) throw new Error("subscribe " + r.status); });
        });
      }).catch(function () { fail(card.dataset.msgFailed); }).then(function () { btn.disabled = false; refresh(); });
    }

    if (btn.hasAttribute("data-notify-test")) {
      btn.disabled = true;
      post("/notifications/test").then(function (r) {
        if (!r.ok) throw new Error("test " + r.status);
        return r.json();
      }).then(function (res) {
        if (res.sent > 0) say(card.dataset.msgSent, true); else fail(card.dataset.msgNone);
      }).catch(function () { fail(card.dataset.msgFailed); }).then(function () { btn.disabled = false; });
    }

    if (btn.hasAttribute("data-notify-disable")) {
      btn.disabled = true;
      current().then(function (sub) {
        if (!sub) return null;
        var endpoint = sub.endpoint;
        return sub.unsubscribe().then(function () { return post("/notifications/unsubscribe", { endpoint: endpoint }); });
      }).catch(function () { fail(card.dataset.msgFailed); }).then(function () { btn.disabled = false; refresh(); });
    }
  });

  refresh();
})();
