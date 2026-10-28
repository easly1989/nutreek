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
//   data-search-reset="empty"   (optional: clearing the box empties the target
//                                instead of re-requesting q="")
//
// The endpoint must accept `q` and `off=1` (see searchMixed on the Go side).
(function () {
  var DB_DELAY_MS = 150;
  var OFF_DELAY_MS = 1000;
  var uid = 0;

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
    // Incremental token: a response is swapped in only while it belongs to the
    // latest dispatch (the user has not changed the query in the meantime).
    var seq = 0;
    var seqHeader = 'X-Search-Seq';
    var inputId = ++uid;

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
      var headers = {};
      headers[seqHeader] = inputId + ':' + mySeq;
      window.htmx.ajax('GET', buildURL(q, includeOFF), {
        target: target,
        swap: 'innerHTML',
        headers: headers,
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

    // Back to what the page shows before any search: empty when the input says
    // so (the recipe page), else the list for q="". Bumping seq first drops any
    // response still on its way.
    function reset() {
      clearTimeout(dbTimer);
      clearTimeout(offTimer); offTimer = null;
      ++seq;
      hideSpinner();
      offInflight = false;
      var el = document.querySelector(target);
      if (el && input.dataset.searchReset === 'empty') { el.innerHTML = ''; return; }
      dbTimer = setTimeout(function () { dispatch('', false); }, DB_DELAY_MS);
    }

    // Drop the response of a request that is no longer the latest. Requests
    // made elsewhere (the initial load, a refresh) carry no tag: they are
    // dropped when their q no longer matches the box.
    document.body.addEventListener('htmx:beforeSwap', function (e) {
      if (!input.isConnected) return;
      var cfg = e.detail.requestConfig || {};
      var tag = cfg.headers && cfg.headers[seqHeader];
      if (tag) {
        if (tag.indexOf(inputId + ':') === 0 && tag !== inputId + ':' + seq) {
          e.detail.shouldSwap = false;
        }
        return;
      }
      if (!e.detail.target || !e.detail.target.matches || !e.detail.target.matches(target)) return;
      var qs = String(cfg.path || '').split('?')[1];
      var q = qs && new URLSearchParams(qs).get('q');
      if (q !== null && q !== undefined && q.trim() !== (input.value || '').trim()) {
        e.detail.shouldSwap = false;
      }
    });

    function onInput() {
      var q = (input.value || '').trim();
      clearTimeout(dbTimer);
      clearTimeout(offTimer); offTimer = null;

      if (q === '') { reset(); return; }

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
      if (!input.isConnected) return;
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
      if (q === '') { reset(); return; }
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
