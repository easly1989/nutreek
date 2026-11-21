// manual.js: the items added by hand to the shopping list (#234).
//  - Suggestions: htmx fills #shop-suggest while the user types; a click (or
//    Enter) on one puts its text, quantity and unit in the form. The arrows
//    move through them, Esc and leaving the field close them.
//  - Copy sheet (form[data-copy]): the tabs show one past list at a time, the
//    button counts the ticked items (all tabs) and stays off when there are none.
(function () {
  "use strict";

  function panel() { return document.getElementById("shop-suggest"); }
  function clearSuggest() { var p = panel(); if (p) p.innerHTML = ""; }

  function pick(btn) {
    var form = btn.closest("form");
    if (!form) return;
    form.elements.description.value = btn.dataset.desc || "";
    var qty = form.elements.quantity;
    qty.value = btn.dataset.qty || "";
    var unit = form.elements.unit;
    if (btn.dataset.qty && unit && Array.prototype.some.call(unit.options, function (o) { return o.value === btn.dataset.unit; })) {
      unit.value = btn.dataset.unit;
    }
    clearSuggest();
    qty.focus({ preventScroll: true });
    qty.select();
  }

  document.addEventListener("click", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;
    var s = t.closest("[data-suggest]");
    if (s) { pick(s); return; }
    var tab = t.closest("[data-copy-tab]");
    if (tab) showTab(tab);
  });

  // Keep the focus in the field while a suggestion is pressed, or the list
  // would close before the click lands.
  document.addEventListener("mousedown", function (evt) {
    if (evt.target instanceof Element && evt.target.closest("#shop-suggest")) evt.preventDefault();
  });

  document.addEventListener("focusout", function (evt) {
    if (!(evt.target instanceof Element) || !evt.target.matches("[data-suggest-field]")) return;
    var to = evt.relatedTarget;
    if (to instanceof Element && to.closest("#shop-suggest")) return;
    setTimeout(clearSuggest, 150);
  });

  document.addEventListener("keydown", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;
    if (evt.key === "Escape" && panel() && panel().firstChild) { clearSuggest(); return; }
    if (evt.key !== "ArrowDown" && evt.key !== "ArrowUp") return;
    var items = Array.prototype.slice.call(document.querySelectorAll("#shop-suggest [data-suggest]"));
    if (!items.length) return;
    var i = items.indexOf(t);
    if (i < 0 && !t.matches("[data-suggest-field]")) return;
    evt.preventDefault();
    var next = evt.key === "ArrowDown" ? i + 1 : i - 1;
    if (next < 0) { document.querySelector("[data-suggest-field]").focus(); return; }
    if (next < items.length) items[next].focus();
  });

  // A request that went through empties the suggestions the form's reset left.
  document.body.addEventListener("htmx:afterRequest", function (evt) {
    var f = evt.detail.elt;
    if (f && f.matches && f.matches("#shop-manual form[data-reset-on-success]") && evt.detail.successful) clearSuggest();
  });

  function showTab(tab) {
    var form = tab.closest("form");
    form.querySelectorAll("[data-copy-tab]").forEach(function (b) { b.setAttribute("aria-selected", b === tab ? "true" : "false"); });
    form.querySelectorAll("[data-copy-pane]").forEach(function (p) { p.hidden = p.dataset.copyPane !== tab.dataset.copyTab; });
  }

  // refresh counts the ticked items and words the button («Copy 2 items»: the
  // singular and plural forms come from the catalog, the number is put in).
  function refresh(form) {
    var n = form.querySelectorAll("input[name=item]:checked").length;
    var btn = form.querySelector("[data-copy-submit]");
    btn.textContent = (n === 1 ? btn.dataset.single : btn.dataset.plural).replace(/\d+/, n);
    btn.disabled = n === 0;
  }

  document.addEventListener("change", function (evt) {
    var form = evt.target instanceof Element && evt.target.closest("form[data-copy]");
    if (form) refresh(form);
  });

  document.addEventListener("toggle", function (evt) {
    var d = evt.target;
    if (!(d instanceof Element) || !d.matches("details[data-sheet]") || !d.open) return;
    var form = d.querySelector("form[data-copy]");
    if (form) refresh(form);
  }, true);
})();
