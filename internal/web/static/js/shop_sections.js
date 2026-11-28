// shop_sections.js: the sections and the search of the shopping list.
//
// - «Not needed» and «In the cart» are closed <details>. Every tick swaps the whole
//   #shop-list, which would close them again, so the open state is kept here and put
//   back after each swap (and in sessionStorage, for a reload of the page).
// - The search (header on a wide screen, dock on a phone) hides the rows that do not
//   match while typing; a section with a match opens, the others are hidden. The
//   list is swapped under it, so the filter runs again after every swap.
(function () {
  "use strict";

  var KEY = "nutreek.shop.open";
  var open = {};
  try { open = JSON.parse(sessionStorage.getItem(KEY) || "{}") || {}; } catch (e) { open = {}; }
  function save() { try { sessionStorage.setItem(KEY, JSON.stringify(open)); } catch (e) { /* private mode */ } }

  var query = ""; // while it is not empty a search opens the sections, not the user: do not remember that

  function sections() { return document.querySelectorAll("#shop-list details[data-shop-acc]"); }

  function restore() {
    sections().forEach(function (d) { d.open = !!open[d.dataset.shopAcc]; });
  }

  // toggle does not bubble: listen in the capture phase on the document.
  document.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d || !d.dataset || !d.dataset.shopAcc || query.trim()) return;
    open[d.dataset.shopAcc] = d.open;
    save();
  }, true);

  function norm(s) {
    return s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
  }

  function filter() {
    var list = document.getElementById("shop-list");
    if (!list) return;
    var q = norm(query.trim());
    var matches = 0;
    list.querySelectorAll("li.swipe-row").forEach(function (li) {
      var nm = li.querySelector(".shop-nm");
      var hit = !q || norm((nm || li).textContent).indexOf(q) >= 0;
      li.hidden = !hit;
      if (hit) matches++;
    });
    sections().forEach(function (d) {
      var any = d.querySelector("li.swipe-row:not([hidden])");
      d.hidden = !!q && !any;
      d.open = q ? !!any : !!open[d.dataset.shopAcc];
    });
    // The headings of the groups and the count lines have nothing under them when no row matches.
    list.querySelectorAll("[data-shop-group]").forEach(function (g) {
      g.hidden = !!q && !g.querySelector("li.swipe-row:not([hidden])");
    });
    // The count in a heading follows what is shown.
    list.querySelectorAll("[data-shop-count]").forEach(function (c) {
      if (!c.dataset.total) c.dataset.total = c.textContent;
      var ul = c.closest("h2") && c.closest("h2").nextElementSibling;
      c.textContent = q && ul ? String(ul.querySelectorAll("li.swipe-row:not([hidden])").length) : c.dataset.total;
    });
    var none = list.querySelector("[data-shop-none]");
    if (none) none.hidden = !(q && matches === 0);
  }

  function inputs() { return document.querySelectorAll("input[data-shop-search]"); }

  document.addEventListener("input", function (e) {
    if (!e.target.matches || !e.target.matches("input[data-shop-search]")) return;
    query = e.target.value;
    inputs().forEach(function (i) { if (i !== e.target) i.value = query; });
    filter();
  });

  // The search field of a phone keeps the focus when the list is swapped (it is outside
  // #shop-list); the keyboard's «search» key only closes the keyboard.
  document.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && e.target.matches && e.target.matches("input[data-shop-search]")) {
      e.preventDefault();
      e.target.blur();
    }
  });

  document.addEventListener("htmx:afterSwap", function () {
    if (!document.getElementById("shop-list")) return;
    restore();
    if (query) filter();
  });

  restore();
})();
