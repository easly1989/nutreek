// awake.js: keep the phone screen on (#249).
//  - Settings: the card with the switch is shown only where the browser has the
//    Screen Wake Lock API and the device is touch (a phone or a tablet); the
//    switch saves itself.
//  - Every page: when the user turned it on (body[data-keep-awake]), the page
//    asks for the lock while it is visible, and asks again when it comes back to
//    the foreground (the browser drops the lock when the page is hidden). Where
//    the API is missing, or the page is not on HTTPS, it does nothing.
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

  if (!supported || !touch || document.body.dataset.keepAwake !== "1") return;

  var lock = null;
  function acquire() {
    if (lock || document.visibilityState !== "visible") return;
    navigator.wakeLock.request("screen").then(function (l) {
      lock = l;
      l.addEventListener("release", function () { lock = null; });
    }).catch(function () { /* refused (battery saver, no HTTPS): the screen sleeps as usual */ });
  }
  document.addEventListener("visibilitychange", acquire);
  acquire();
})();
