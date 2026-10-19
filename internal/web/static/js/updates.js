// Updates (#167): the owner's card, the «How to update» dialog, and the
// notice for a page left open across an update.
(function () {
  "use strict";

  var KEY = "nutreek_update_dismissed";

  function dismissed() {
    try { return localStorage.getItem(KEY); } catch (e) { return null; }
  }

  // A card closed in this browser stays closed until the next release.
  function hideDismissed() {
    var v = dismissed();
    if (!v) return;
    document.querySelectorAll("[data-update-card], [data-update-dot]").forEach(function (el) {
      if ((el.dataset.updateCard || el.dataset.updateDot) === v) el.classList.add("hidden");
    });
  }

  // The server answers with its build; a different one means this page's JS
  // and CSS are old. Nothing reloads by itself, so nothing typed is lost.
  var later = false;
  document.addEventListener("htmx:afterRequest", function (e) {
    var xhr = e.detail.xhr;
    var build = xhr && xhr.getResponseHeader("X-Nutreek-Build");
    var notice = document.getElementById("new-build");
    if (!later && notice && build && build !== document.body.dataset.build) notice.classList.remove("hidden");
  });

  function init() {
    hideDismissed();
    document.querySelectorAll("[data-copy]").forEach(function (b) {
      if (navigator.clipboard) b.classList.remove("hidden");
    });

    document.addEventListener("click", function (e) {
      var close = e.target.closest("[data-update-dismiss]");
      if (close) {
        var card = close.closest("[data-update-card]");
        try { localStorage.setItem(KEY, card.dataset.updateCard); } catch (err) { /* private mode: hide for now */ }
        card.classList.add("hidden");
        hideDismissed();
        return;
      }

      // Without JS, or without the dialog, the link opens /admin/update.
      var howto = e.target.closest("[data-update-howto]");
      var dialog = document.getElementById("update-howto");
      if (howto && dialog && dialog.showModal) {
        e.preventDefault();
        var sheet = howto.closest("dialog");
        if (sheet) sheet.close();
        dialog.showModal();
        return;
      }

      if (e.target.closest("[data-new-build-reload]")) {
        location.reload();
        return;
      }
      if (e.target.closest("[data-new-build-later]")) {
        later = true;
        document.getElementById("new-build").classList.add("hidden");
        return;
      }

      var copy = e.target.closest("[data-copy]");
      if (copy) {
        var text = copy.parentElement.querySelector("pre").textContent;
        navigator.clipboard.writeText(text).then(function () {
          var label = copy.querySelector("span");
          var was = label.textContent;
          label.textContent = copy.dataset.copied;
          setTimeout(function () { label.textContent = was; }, 1500);
        });
      }
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
