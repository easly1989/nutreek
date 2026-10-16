// picker.js — rows of the ingredient picker (the picker_results partial).
//
// Every row is a <details data-picker-row>: opening it closes the others and
// moves the cursor to the quantity. The form carries data-kcal100 (kcal per
// 100 g/ml) and data-unit-weight (grams of one piece, 0 when unknown), or for
// recipes data-kcal-each (kcal per serving): from these we compute the kcal
// preview while typing.
(function () {
  "use strict";

  function parseQty(v) {
    var n = parseFloat(String(v || "").replace(",", "."));
    return isFinite(n) && n > 0 ? n : 0;
  }

  function updatePreview(form) {
    var out = form.querySelector("[data-kcal-preview]");
    if (!out) return;
    var qty = parseQty(form.elements.quantity && form.elements.quantity.value);
    var each = parseFloat(form.dataset.kcalEach);
    if (isFinite(each)) {
      out.textContent = qty ? Math.round(qty * each).toLocaleString("it-IT") + " kcal" : "";
      return;
    }
    var kcal100 = parseFloat(form.dataset.kcal100);
    if (!isFinite(kcal100) || qty === 0) { out.textContent = ""; return; }
    var unit = form.querySelector('input[name="qty_unit"]:checked') || form.querySelector('input[name="qty_unit"]');
    var grams = qty;
    if (unit && unit.value === "pz") grams = qty * (parseFloat(form.dataset.unitWeight) || 0);
    out.textContent = Math.round(grams * kcal100 / 100).toLocaleString("it-IT") + " kcal";
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

  // A meal's picker ([data-meal-picker], page or dialog): Ingrediente /
  // Ricetta switches the live search and reloads the results.
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
      search.placeholder = kind === "recipe" ? "Cerca una ricetta" : "Cerca tra i tuoi ingredienti e su OpenFoodFacts";
      search.focus();
    }
    refreshMealPicker(picker);
  });

  // After «Nuovo ingrediente/ricetta» the picker reloads the results.
  ["ingredient-created", "recipe-created"].forEach(function (ev) {
    window.addEventListener(ev, function () {
      document.querySelectorAll("[data-meal-picker]").forEach(refreshMealPicker);
    });
  });

  ["input", "change"].forEach(function (type) {
    document.addEventListener(type, function (e) {
      var form = e.target.closest && e.target.closest("form[data-kcal100], form[data-kcal-each]");
      if (form) updatePreview(form);
    });
  });
})();
