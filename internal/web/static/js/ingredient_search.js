// ingredient_search.js — a double debounce for the ingredient live search.
//
// Pattern: one input searches both the local DB (fast, every keystroke) and
// OpenFoodFacts (slow, only once typing stops). Without this split, every
// letter typed fired a full OFF call: slow, and with results based on
// partial words.
//
// Markup required on the input:
//   data-ingredient-search
//   data-search-url="/path/to/endpoint"
//   data-search-target="#results-container"
//   data-search-spinner="#spinner-container"   (optional)
//   data-search-extra="kind=ingredient&start=..."  (optional, fixed params)
//
// The endpoint must accept `q` and `off=1` (see searchMixed on the Go side).
(function () {
  var DB_DELAY_MS = 150;
  var OFF_DELAY_MS = 1000;

  function init(input) {
    if (input.__nutreekSearchBound) return;
    input.__nutreekSearchBound = true;

    var target = input.dataset.searchTarget;
    if (!input.dataset.searchUrl || !target) return;
    // url / extra / no-off are read dynamically to support tab switches
    // (e.g. meal_item_new ingredient <-> recipe).
    function readURL() { return input.dataset.searchUrl; }
    function readExtra() { return input.dataset.searchExtra || ''; }
    function noOFF() { return input.dataset.searchNoOff === '1' || input.dataset.searchNoOff === 'true'; }
    var spinnerSel = input.dataset.searchSpinner || '';

    var dbTimer = null;
    var offTimer = null;
    var offInflight = false;
    var lastDispatchedQ = null;
    // Incremental token: ignore responses arriving out of order (the user
    // has already changed the query in the meantime).
    var seq = 0;

    function spinner() {
      return spinnerSel ? document.querySelector(spinnerSel) : null;
    }
    function showSpinner() {
      var s = spinner();
      if (s) s.classList.remove('hidden');
    }
    function hideSpinner() {
      var s = spinner();
      if (s) s.classList.add('hidden');
    }

    function buildURL(q, includeOFF) {
      var params = new URLSearchParams();
      params.set('q', q);
      if (includeOFF) params.set('off', '1');
      var extra = readExtra();
      if (extra) {
        var ex = new URLSearchParams(extra);
        ex.forEach(function (v, k) { params.set(k, v); });
      }
      return readURL() + '?' + params.toString();
    }

    function dispatch(q, includeOFF) {
      var mySeq = ++seq;
      if (includeOFF) offInflight = true;
      window.htmx.ajax('GET', buildURL(q, includeOFF), {
        target: target,
        swap: 'innerHTML',
      }).then(function () {
        if (includeOFF) {
          offInflight = false;
          // Only the last OFF request hides the spinner; if the user typed
          // again in the meantime, seq has moved on and a new OFF request is
          // already pending.
          if (mySeq === seq) hideSpinner();
        }
      }).catch(function () {
        if (includeOFF) {
          offInflight = false;
          if (mySeq === seq) hideSpinner();
        }
      });
    }

    function onInput() {
      var q = (input.value || '').trim();
      clearTimeout(dbTimer);
      clearTimeout(offTimer); offTimer = null;

      if (q === '') {
        // Reset: no OFF search to wait for, no spinner.
        hideSpinner();
        offInflight = false;
        dbTimer = setTimeout(function () { dispatch('', false); }, DB_DELAY_MS);
        return;
      }

      var skipOFF = noOFF();
      if (!skipOFF) {
        // Spinner: the user typed something, OFF has not started yet or has
        // not come back yet. We hide it when OFF completes.
        showSpinner();
      } else {
        hideSpinner();
      }
      dbTimer = setTimeout(function () { dispatch(q, false); }, DB_DELAY_MS);
      if (!skipOFF) {
        offTimer = setTimeout(function () {
          offTimer = null;
          // Mirror the latest query at the moment the OFF call starts.
          var latest = (input.value || '').trim();
          if (latest === '') {
            hideSpinner();
            return;
          }
          lastDispatchedQ = latest;
          dispatch(latest, true);
        }, OFF_DELAY_MS);
      }
    }

    input.addEventListener('input', onInput);
    // A barcode that arrived with a reload without OFF (after «Nuovo
    // ingrediente», a tab change): the partial flags it, and the full search
    // starts here, unless it is already on its way.
    document.body.addEventListener('htmx:afterSwap', function (e) {
      if (!e.target.matches || !e.target.matches(target)) return;
      if (!e.target.querySelector('[data-barcode-pending]')) return;
      if (offTimer || offInflight || noOFF()) return;
      var q = (input.value || '').trim();
      if (q) { showSpinner(); dispatch(q, true); }
    });
    // Enter key: force the OFF search right away without waiting for the debounce.
    input.addEventListener('search', function () {
      var q = (input.value || '').trim();
      clearTimeout(dbTimer);
      clearTimeout(offTimer); offTimer = null;
      if (q === '') { hideSpinner(); return; }
      if (noOFF()) { dispatch(q, false); return; }
      showSpinner();
      dispatch(q, true);
    });
  }

  function bindAll(root) {
    (root || document).querySelectorAll('input[data-ingredient-search]')
      .forEach(init);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { bindAll(); });
  } else {
    bindAll();
  }
  // Picker remounted dynamically by HTMX (meal_item_new changes tab).
  document.body.addEventListener('htmx:afterSwap', function (e) {
    bindAll(e.target);
  });
})();
