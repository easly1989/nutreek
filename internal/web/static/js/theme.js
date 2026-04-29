// theme.js — gestione tema persistente (cookie + localStorage).
// Lettura iniziale: il server inietta data-theme su <html> via cookie, niente
// FOUC. Qui aggiungiamo solo il toggle interattivo via dropdown.
(function () {
  "use strict";

  var THEMES = ["forest", "ocean", "sunset", "mono"];
  var COOKIE = "theme";
  var STORAGE = "nutreek-theme";

  function setCookie(value) {
    var maxAge = 60 * 60 * 24 * 365;
    document.cookie = COOKIE + "=" + value + "; path=/; max-age=" + maxAge + "; SameSite=Lax";
  }

  function applyTheme(value) {
    if (THEMES.indexOf(value) === -1) {
      value = "forest";
    }
    document.documentElement.setAttribute("data-theme", value);
    try {
      localStorage.setItem(STORAGE, value);
    } catch (e) { /* ignore */ }
    setCookie(value);
    // Aggiorna stato UI (radio + label)
    document.querySelectorAll("[data-theme-option]").forEach(function (el) {
      var active = el.dataset.themeOption === value;
      el.setAttribute("aria-checked", active ? "true" : "false");
      el.classList.toggle("bg-accent-soft", active);
    });
    var label = document.querySelector("[data-theme-current-label]");
    if (label) label.textContent = capitalize(value);
  }

  function capitalize(s) {
    return s.charAt(0).toUpperCase() + s.slice(1);
  }

  function init() {
    // Toggle dropdown
    var trigger = document.querySelector("[data-theme-trigger]");
    var menu = document.querySelector("[data-theme-menu]");
    if (trigger && menu) {
      trigger.addEventListener("click", function (e) {
        e.stopPropagation();
        menu.classList.toggle("hidden");
      });
      document.addEventListener("click", function (e) {
        if (!menu.contains(e.target) && e.target !== trigger) {
          menu.classList.add("hidden");
        }
      });
    }
    // Click su opzione
    document.querySelectorAll("[data-theme-option]").forEach(function (el) {
      el.addEventListener("click", function () {
        applyTheme(el.dataset.themeOption);
        if (menu) menu.classList.add("hidden");
      });
    });
    // Allinea UI al valore correntemente in data-theme (settato da server)
    var current = document.documentElement.getAttribute("data-theme") || "forest";
    applyTheme(current);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
