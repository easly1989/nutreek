// Stato e aggiornamento della copia locale di OpenFoodFacts, nella
// Panoramica dell'Amministrazione (solo owner). Gli elementi da riempire
// hanno data-off="<campo>" e possono comparire piu' volte (tile e card).
(function () {
  'use strict';

  const card = document.getElementById('off-sync-card');
  if (!card) return;

  const csrfToken = card.dataset.csrf || '';
  const btn = document.getElementById('off-sync-btn');
  const forceCb = document.getElementById('off-force');
  const feedback = document.getElementById('off-feedback');
  let pollTimer = null;

  function fmt(n) { return Number(n || 0).toLocaleString('it-IT', { useGrouping: 'always' }); }
  function set(field, text) {
    document.querySelectorAll('[data-off="' + field + '"]').forEach(function (el) { el.textContent = text; });
  }
  function fmtDate(s) {
    if (!s) return 'mai';
    const d = new Date(s);
    if (isNaN(d)) return s;
    return d.toLocaleDateString('it-IT', { day: 'numeric', month: 'long' }) + ', ' +
      d.toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit' });
  }
  function relDate(s) {
    if (!s) return 'mai aggiornati';
    const days = Math.floor((Date.now() - new Date(s).getTime()) / 86400000);
    if (isNaN(days)) return '';
    if (days <= 0) return 'aggiornati oggi';
    if (days === 1) return 'aggiornati ieri';
    return 'aggiornati ' + days + ' giorni fa';
  }

  function setFeedback(msg, kind) {
    feedback.hidden = !msg;
    feedback.classList.toggle('hidden', !msg);
    feedback.textContent = msg || '';
    feedback.classList.toggle('text-danger', kind === 'err');
    feedback.classList.toggle('text-accent', kind === 'ok');
    feedback.classList.toggle('text-muted', kind !== 'err' && kind !== 'ok');
  }

  function applyState(s) {
    if (!s) return;
    const count = s.products_count || 0;
    set('count', fmt(count));
    set('synced', fmtDate(s.last_synced_at));
    set('synced-rel', relDate(s.last_synced_at));
    if (typeof s.diag_full_nutrients === 'number') {
      set('full', fmt(s.diag_full_nutrients) + (count ? ' (' + Math.round(s.diag_full_nutrients / count * 100) + '%)' : ''));
    }
    set('state', s.syncing ? 'Aggiornamento in corso…' : (count ? 'Pronto' : 'Vuoto'));
    btn.disabled = !!s.syncing;
    forceCb.disabled = !!s.syncing;
    const err = card.querySelector('[data-off="error"]');
    err.classList.toggle('hidden', !s.last_error);
    err.textContent = s.last_error ? 'Ultimo errore: ' + s.last_error : '';
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/profile/off-status', { credentials: 'same-origin' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const state = await res.json();
      applyState(state);
      return state;
    } catch (e) {
      setFeedback('Non riesco a leggere lo stato (' + e.message + ').', 'err');
      return null;
    }
  }

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(async function () {
      const s = await fetchStatus();
      if (s && !s.syncing) {
        stopPolling();
        setFeedback('Aggiornamento finito: ' + fmt(s.products_count) + ' prodotti.', 'ok');
      }
    }, 5000);
  }
  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  async function start() {
    const force = !!forceCb.checked;
    btn.disabled = true;
    setFeedback('Avvio…', 'info');
    try {
      const res = await fetch('/profile/off-sync' + (force ? '?force=true' : ''), {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-CSRF-Token': csrfToken },
      });
      const state = await res.json().catch(function () { return null; });
      if (res.status === 202 || res.status === 409) {
        applyState(state);
        setFeedback(res.status === 202 ? 'Aggiornamento avviato: lo stato si rinnova ogni 5 secondi.' : 'Un aggiornamento è già in corso.', 'info');
        startPolling();
      } else {
        setFeedback('Avvio non riuscito (HTTP ' + res.status + ').', 'err');
        btn.disabled = false;
      }
    } catch (e) {
      setFeedback('Errore di rete: ' + e.message, 'err');
      btn.disabled = false;
    }
  }

  btn.addEventListener('click', function () {
    const msg = forceCb.checked
      ? "Scaricare di nuovo tutto il dump (circa 900 MB, una decina di minuti)?"
      : "Aggiornare OpenFoodFacts? Se il dump non è cambiato finisce subito, altrimenti ci vuole una decina di minuti.";
    if (window._showConfirm) window._showConfirm(msg, start);
    else if (window.confirm(msg)) start();
  });

  fetchStatus().then(function (s) { if (s && s.syncing) startPolling(); });
})();
