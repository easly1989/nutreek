// ingredient_search.js — doppio debounce per la live-search ingredienti.
//
// Pattern: un input cerca contemporaneamente nel DB locale (veloce, ogni
// keystroke) e su OpenFoodFacts (lento, solo a digitazione ferma). Senza
// questo split, ogni lettera digitata scatenava una chiamata OFF
// completa: lenta e con risultati basati su parole parziali.
//
// Markup richiesto sull'input:
//   data-ingredient-search
//   data-search-url="/path/to/endpoint"
//   data-search-target="#results-container"
//   data-search-spinner="#spinner-container"   (opzionale)
//   data-search-extra="kind=ingredient&start=..."  (opzionale, params fissi)
//
// L'endpoint deve accettare `q` e `off=1` (vedi searchMixed lato Go).
(function () {
  var DB_DELAY_MS = 150;
  var OFF_DELAY_MS = 1000;

  function init(input) {
    if (input.__nutreekSearchBound) return;
    input.__nutreekSearchBound = true;

    var target = input.dataset.searchTarget;
    if (!input.dataset.searchUrl || !target) return;
    // url / extra / no-off vengono letti dinamicamente per supportare
    // i tab switch (es. meal_item_new ingrediente <-> ricetta).
    function readURL() { return input.dataset.searchUrl; }
    function readExtra() { return input.dataset.searchExtra || ''; }
    function noOFF() { return input.dataset.searchNoOff === '1' || input.dataset.searchNoOff === 'true'; }
    var spinnerSel = input.dataset.searchSpinner || '';

    var dbTimer = null;
    var offTimer = null;
    var offInflight = false;
    var lastDispatchedQ = null;
    // Token incrementale: ignora risposte arrivate fuori ordine (utente ha
    // gia' modificato la query nel frattempo).
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
          // Solo l'ultima richiesta OFF nasconde lo spinner; se nel
          // frattempo l'utente ha ridigitato, seq sara' avanzato e una
          // nuova OFF e' gia' in attesa.
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
        // Reset: nessuna ricerca OFF da attendere, niente spinner.
        hideSpinner();
        offInflight = false;
        dbTimer = setTimeout(function () { dispatch('', false); }, DB_DELAY_MS);
        return;
      }

      var skipOFF = noOFF();
      if (!skipOFF) {
        // Spinner: l'utente ha digitato qualcosa, OFF non e' ancora partito
        // o non e' ancora tornato. Lo nasconderemo al completamento OFF.
        showSpinner();
      } else {
        hideSpinner();
      }
      dbTimer = setTimeout(function () { dispatch(q, false); }, DB_DELAY_MS);
      if (!skipOFF) {
        offTimer = setTimeout(function () {
          offTimer = null;
          // Rifletti l'ultima query nel momento in cui parte la chiamata OFF.
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
    // Codice a barre arrivato da una ricarica senza OFF (dopo «Nuovo
    // ingrediente», cambio scheda): il partial lo segnala, qui parte la
    // ricerca completa, se non e' gia' in arrivo.
    document.body.addEventListener('htmx:afterSwap', function (e) {
      if (!e.target.matches || !e.target.matches(target)) return;
      if (!e.target.querySelector('[data-barcode-pending]')) return;
      if (offTimer || offInflight || noOFF()) return;
      var q = (input.value || '').trim();
      if (q) { showSpinner(); dispatch(q, true); }
    });
    // Tasto Enter: forza subito la ricerca OFF senza aspettare debounce.
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
  // Picker rimontato dinamicamente da HTMX (meal_item_new cambia tab).
  document.body.addEventListener('htmx:afterSwap', function (e) {
    bindAll(e.target);
  });
})();
