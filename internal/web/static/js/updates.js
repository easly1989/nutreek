// Updates (#167): the owner's card and the «How to update» dialog.
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
