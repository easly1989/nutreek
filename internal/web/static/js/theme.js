// theme.js — aspetto (modalita' x colore) e piccoli comandi della shell.
//
// Il server scrive data-mode / data-accent su <html> leggendo i cookie
// tema_modo / tema_colore (internal/app/appearance.go), quindi la pagina
// nasce gia' col tema giusto. Qui gestiamo solo il cambio interattivo:
// aggiorniamo l'attributo, il cookie e lo stato dei controlli, che possono
// comparire piu' volte nella pagina (sidebar e foglio «Altro»).
(function () {
  "use strict";

  var MODES = ["chiaro", "scuro", "sistema"];
  var ACCENTS = ["bosco", "mare", "tramonto", "grafite"];
  var YEAR = 60 * 60 * 24 * 365;
  var root = document.documentElement;

  function setCookie(name, value) {
    document.cookie = name + "=" + value + "; path=/; max-age=" + YEAR + "; SameSite=Lax";
  }

  function syncControls() {
    var mode = root.getAttribute("data-mode");
    var accent = root.getAttribute("data-accent");
    document.querySelectorAll("[data-appearance-mode]").forEach(function (el) {
      el.setAttribute("aria-checked", String(el.dataset.appearanceMode === mode));
    });
    document.querySelectorAll("[data-appearance-accent]").forEach(function (el) {
      el.setAttribute("aria-checked", String(el.dataset.appearanceAccent === accent));
    });
  }

  function apply(attr, cookie, allowed, value) {
    if (allowed.indexOf(value) === -1) return;
    root.setAttribute(attr, value);
    setCookie(cookie, value);
    // Il vecchio cookie a quattro temi non serve piu': i nuovi lo sostituiscono.
    document.cookie = "theme=; path=/; max-age=0; SameSite=Lax";
    syncControls();
    document.dispatchEvent(new CustomEvent("nutreek:appearance"));
  }

  // Popover ancorati (es. «Aspetto» nella sidebar): data-popover="<id>".
  function closePopovers(except) {
    document.querySelectorAll("[data-popover]").forEach(function (btn) {
      var panel = document.getElementById(btn.dataset.popover);
      if (!panel || panel === except) return;
      panel.classList.add("hidden");
      btn.setAttribute("aria-expanded", "false");
    });
  }

  function init() {
    document.addEventListener("click", function (e) {
      var mode = e.target.closest("[data-appearance-mode]");
      if (mode) { apply("data-mode", "tema_modo", MODES, mode.dataset.appearanceMode); return; }
      var accent = e.target.closest("[data-appearance-accent]");
      if (accent) { apply("data-accent", "tema_colore", ACCENTS, accent.dataset.appearanceAccent); return; }

      var trigger = e.target.closest("[data-popover]");
      if (trigger) {
        var panel = document.getElementById(trigger.dataset.popover);
        if (!panel) return;
        var open = panel.classList.contains("hidden");
        closePopovers(panel);
        panel.classList.toggle("hidden", !open);
        trigger.setAttribute("aria-expanded", String(open));
        return;
      }
      if (!e.target.closest(".menu-panel")) closePopovers(null);

      // data-focus="<id>": porta il cursore su un campo (es. «Aggiungi pasto»).
      var focusBtn = e.target.closest("[data-focus]");
      if (focusBtn) {
        var field = document.getElementById(focusBtn.dataset.focus);
        if (field) {
          field.scrollIntoView({ behavior: "smooth", block: "center" });
          field.focus({ preventScroll: true });
        }
        return;
      }

      var sheetBtn = e.target.closest("[data-sheet]");
      if (sheetBtn) {
        var sheet = document.getElementById(sheetBtn.dataset.sheet);
        if (sheet && sheet.showModal) sheet.showModal();
        return;
      }
      if (e.target.closest("[data-sheet-close]")) {
        var d = e.target.closest("dialog");
        if (d) d.close();
        return;
      }
      // Click sullo sfondo del foglio (fuori dal contenuto) lo chiude.
      if (e.target.tagName === "DIALOG" && e.target.classList.contains("sheet")) {
        e.target.close();
      }
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") closePopovers(null);
    });
    syncControls();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
