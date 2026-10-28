// picker.js — rows of the ingredient picker (the picker_results partial).
//
// Every row is a <details data-picker-row>: opening it closes the others and
// moves the cursor to the quantity. The form carries data-kcal100 (kcal per
// 100 g/ml) and data-unit-weight (grams of one piece, 0 when unknown), or for
// recipes data-kcal-each (kcal per serving): from these we compute the kcal
// preview while typing.
(function () {
  "use strict";

  var LANG = document.documentElement.lang || "it";

  function parseQty(v) {
    var n = parseFloat(String(v || "").replace(",", "."));
    return isFinite(n) && n > 0 ? n : 0;
  }

  // convertQty turns a quantity into the other unit when the g / pz switch
  // changes (#206): pz -> g is qty x unit weight to a whole gram, g -> pz is
  // qty / unit weight to two decimals; never 0 (at least 1 g or 0.01 pz).
  // Without a weight it leaves the number alone.
  function convertQty(value, toUnit, unitWeight) {
    var qty = parseQty(value);
    var w = parseFloat(unitWeight);
    if (!qty || !(w > 0)) return value;
    var n = toUnit === "pz" ? Math.max(0.01, Math.round(qty / w * 100) / 100) : Math.max(1, Math.round(qty * w));
    return n.toLocaleString(LANG, { maximumFractionDigits: 2, useGrouping: false });
  }

  function updatePreview(form) {
    var out = form.querySelector("[data-kcal-preview]");
    if (!out) return;
    var qty = parseQty(form.elements.quantity && form.elements.quantity.value);
    var each = parseFloat(form.dataset.kcalEach);
    if (isFinite(each)) {
      out.textContent = qty ? Math.round(qty * each).toLocaleString(LANG) + " kcal" : "";
      return;
    }
    var kcal100 = parseFloat(form.dataset.kcal100);
    if (!isFinite(kcal100) || qty === 0) { out.textContent = ""; return; }
    var unit = form.querySelector('input[name="qty_unit"]:checked') || form.querySelector('input[name="qty_unit"]');
    var grams = qty;
    if (unit && unit.value === "pz") grams = qty * (parseFloat(form.dataset.unitWeight) || 0);
    out.textContent = Math.round(grams * kcal100 / 100).toLocaleString(LANG) + " kcal";
  }

  document.addEventListener("toggle", function (e) {
    var row = e.target;
    if (!row.matches || !row.matches("details[data-picker-row]") || !row.open) return;
    var list = row.closest("[data-picker-list]") || document;
    list.querySelectorAll("details[data-picker-row][open]").forEach(function (d) {
      if (d !== row) d.open = false;
    });
    var form = row.querySelector("form[data-kcal100], form[data-kcal-each]");
    if (!form) return;
    updatePreview(form);
    var qty = form.elements.quantity;
    if (qty) { qty.focus({ preventScroll: true }); qty.select(); }
  }, true);

  // A meal's picker ([data-meal-picker], page or dialog): the Ingredient /
  // Recipe tabs switch the live search and reloads the results.
  function mealPickerURL(picker, kind, q) {
    return picker.dataset.pickerUrl + "?kind=" + kind + "&" + picker.dataset.pickerExtra + "&q=" + encodeURIComponent(q || "");
  }
  function refreshMealPicker(picker) {
    var search = picker.querySelector("input[data-ingredient-search]");
    var list = picker.querySelector("[data-picker-list]");
    if (!search || !list) return;
    htmx.ajax("GET", mealPickerURL(picker, picker.dataset.kind || "ingredient", search.value), { target: list, swap: "innerHTML" });
  }

  document.addEventListener("click", function (e) {
    var tab = e.target.closest("[data-meal-picker] [data-picker-tab]");
    if (!tab) return;
    var picker = tab.closest("[data-meal-picker]");
    var kind = tab.dataset.pickerTab;
    picker.dataset.kind = kind;
    picker.querySelectorAll("[data-picker-tab]").forEach(function (b) {
      b.setAttribute("aria-checked", String(b === tab));
    });
    picker.querySelectorAll("[data-quick-target]").forEach(function (b) {
      b.classList.toggle("hidden", b.dataset.quickTarget !== kind);
    });
    // Recipes have no barcode: the scanner is for ingredients only.
    picker.querySelectorAll("[data-scan-for]").forEach(function (b) {
      b.classList.toggle("hidden", kind !== "ingredient");
    });
    var search = picker.querySelector("input[data-ingredient-search]");
    if (search) {
      // Read by ingredient_search.js; no OpenFoodFacts for recipes.
      search.dataset.searchExtra = "kind=" + kind + "&" + picker.dataset.pickerExtra;
      search.dataset.searchNoOff = kind === "recipe" ? "1" : "0";
      // The texts come translated from the template (data-placeholder-*).
      search.placeholder = (kind === "recipe" ? search.dataset.placeholderRecipe : search.dataset.placeholderIngredient) || search.placeholder;
      search.focus();
    }
    refreshMealPicker(picker);
  });

  // After «New ingredient/recipe» the picker reloads the results.
  ["ingredient-created", "recipe-created"].forEach(function (ev) {
    window.addEventListener(ev, function () {
      document.querySelectorAll("[data-meal-picker]").forEach(refreshMealPicker);
    });
  });

  // The g / pz switch of the picker's add form and of the quantity editors
  // converts the number, before the kcal preview reads it.
  document.addEventListener("change", function (e) {
    var radio = e.target;
    if (!radio.matches || !radio.matches('input[name="qty_unit"]')) return;
    var form = radio.closest("form[data-unit-weight]");
    var qty = form && form.elements.quantity;
    if (qty) qty.value = convertQty(qty.value, radio.value, form.dataset.unitWeight);
  });

  ["input", "change"].forEach(function (type) {
    document.addEventListener(type, function (e) {
      var form = e.target.closest && e.target.closest("form[data-kcal100], form[data-kcal-each]");
      if (form) updatePreview(form);
    });
  });

  // Quantity editors (#206): a row [data-qty-row] holds the quantity
  // [data-qty-open] and its editor form [data-qty-editor]; data-editing on
  // the row shows the editor. One open at a time; Esc or Cancel closes it and
  // gives the focus back to the quantity.
  function closeEditor(row, focus) {
    if (!row || !row.hasAttribute("data-editing")) return;
    row.removeAttribute("data-editing");
    var form = row.querySelector("form[data-qty-editor]");
    if (form) form.reset();
    var open = row.querySelector("[data-qty-open]");
    if (focus && open) open.focus();
  }

  document.addEventListener("click", function (e) {
    var open = e.target.closest("[data-qty-open]");
    if (open) {
      e.preventDefault();
      var row = open.closest("[data-qty-row]");
      document.querySelectorAll("[data-qty-row][data-editing]").forEach(function (r) {
        if (r !== row) closeEditor(r, false);
      });
      row.setAttribute("data-editing", "");
      var qty = row.querySelector("form[data-qty-editor]").elements.quantity;
      qty.focus();
      qty.select();
      return;
    }
    var cancel = e.target.closest("[data-qty-cancel]");
    if (cancel) {
      e.preventDefault();
      closeEditor(cancel.closest("[data-qty-row]"), true);
    }
  });

  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;
    var form = e.target.closest && e.target.closest("form[data-qty-editor]");
    if (!form) return;
    e.preventDefault();
    closeEditor(form.closest("[data-qty-row]"), true);
  });

  // The page back from a save (or an editor left open by an error) marks the
  // row's control with autofocus, which browsers skip in an unfocused
  // window: focus it here too.
  var auto = document.querySelector("[data-qty-row] [autofocus]");
  if (auto) auto.focus();

  // After an editor's HTMX save the day is swapped: the focus goes back to
  // the saved item's quantity.
  // A save that swapped nothing (an error, or a redirect with a flash)
  // forgets the row, so a later unrelated swap does not take the focus.
  var savedRow = "", savedForm = null;
  document.addEventListener("submit", function (e) {
    var form = e.target.closest && e.target.closest("form[data-qty-editor]");
    if (!form) return;
    savedForm = form;
    savedRow = form.closest("[data-qty-row]").dataset.qtyRow;
  });
  document.addEventListener("htmx:afterRequest", function (e) {
    if (e.detail.elt !== savedForm) return;
    if (!e.detail.successful || e.detail.xhr.status === 204) savedRow = "";
    savedForm = null;
  });
  document.addEventListener("htmx:afterSettle", function () {
    if (!savedRow) return;
    var open = document.querySelector('[data-qty-row="' + savedRow + '"] [data-qty-open]');
    savedRow = "";
    if (open) open.focus();
  });
})();
