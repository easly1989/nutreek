// awake.js: keep the phone screen on (#249).
//  - Settings: the card with the switch is shown only where the browser has the
//    Screen Wake Lock API and the device is touch (a phone or a tablet); the
//    switch saves itself.
//  - Every page: when the user turned it on (body[data-keep-awake]), the page
//    asks for the lock while it is visible, and asks again when it comes back to
//    the foreground (the browser drops the lock when the page is hidden). Where
//    the API is missing, or the page is not on HTTPS, it does nothing.
//  - Shopping list: the same switch as an item of its menu (form[data-awake-menu]),
//    shown on the same terms; after the save the page takes the new value without a reload.
(function () {
  "use strict";

  var supported = "wakeLock" in navigator;
  var touch = window.matchMedia && window.matchMedia("(pointer: coarse)").matches;

  var card = document.querySelector("[data-awake-card]");
  if (card && supported && touch) card.classList.remove("hidden");

  var form = document.querySelector("form[data-awake-form]");
  if (form) {
    form.addEventListener("change", function () { form.requestSubmit(); });
  }

  if (!supported || !touch) return;

  // lock holds the request itself, so a second call while the first is pending
  // does not make a second lock.
  var lock = null;
  var retries = 0; // asks again after a release the browser made on its own, a few times
  function wanted() { return document.body.dataset.keepAwake === "1"; }

  // status writes what happened where the page has a [data-awake-status] (the settings
  // card and the menu of the list), so a refusal does not pass in silence.
  function status(kind, reason) {
    document.querySelectorAll("[data-awake-status]").forEach(function (el) {
      var t = el.dataset[kind];
      el.textContent = t ? t.replace("{reason}", reason || "") : "";
    });
  }

  function acquire() {
    if (!wanted() || lock || document.visibilityState !== "visible") return;
    lock = navigator.wakeLock.request("screen").then(function (l) {
      l.addEventListener("release", function () {
        lock = null;
        if (!wanted() || document.visibilityState !== "visible") return;
        status("released");
        if (retries++ < 3) setTimeout(acquire, 500);
      });
      status("held");
      return l;
    }).catch(function (err) {
      lock = null; // refused (battery saver, no HTTPS): the screen sleeps as usual
      status("denied", err && err.name ? err.name : "?");
    });
  }
  function release() {
    var p = lock;
    lock = null;
    status("");
    if (p) p.then(function (l) { if (l) l.release(); });
  }
  document.addEventListener("visibilitychange", function () { retries = 0; acquire(); });

  // The item in the menu of the shopping list: its hidden field holds the value the
  // next tap saves (1 when the screen is not kept on now).
  var item = document.querySelector("form[data-awake-menu]");
  if (item) {
    item.classList.remove("hidden");
    item.addEventListener("htmx:afterRequest", function (e) {
      if (!e.detail.successful) return;
      var field = item.querySelector("input[name=keep_awake]");
      var on = field.value === "1";
      document.body.dataset.keepAwake = on ? "1" : "0";
      field.value = on ? "0" : "1";
      item.querySelector("button").setAttribute("aria-checked", on ? "true" : "false");
      item.querySelector("[data-awake-mark]").classList.toggle("invisible", !on);
      if (on) acquire(); else release();
    });
  }
  acquire();
})();
