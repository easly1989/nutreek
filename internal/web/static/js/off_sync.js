(function () {
  'use strict';

  const card = document.getElementById('off-sync-card');
  if (!card) return;

  const csrfToken   = card.dataset.csrf || '';
  const btn         = document.getElementById('off-sync-btn');
  const forceCb     = document.getElementById('off-force');
  const feedback    = document.getElementById('off-feedback');
  const elCount     = document.getElementById('off-products-count');
  const elSynced    = document.getElementById('off-last-synced');
  const elState     = document.getElementById('off-state');
  const elFull      = document.getElementById('off-full-count');
  const elPartial   = document.getElementById('off-partial-count');
  const errRow      = document.getElementById('off-error-row');
  const errMsg      = document.getElementById('off-error-msg');
  const sampleRow   = document.getElementById('off-sample-row');
  const sampleList  = document.getElementById('off-sample-list');

  let pollTimer = null;

  function fmtDate(s) {
    if (!s) return 'mai';
    try {
      const d = new Date(s);
      return d.toLocaleString('it-IT', { dateStyle: 'short', timeStyle: 'short' });
    } catch (_) { return s; }
  }

  function setFeedback(msg, kind) {
    if (!msg) { feedback.classList.add('hidden'); feedback.textContent = ''; return; }
    feedback.textContent = msg;
    feedback.classList.remove('hidden', 'text-danger', 'text-accent', 'text-muted');
    feedback.classList.add(kind === 'err' ? 'text-danger' : kind === 'ok' ? 'text-accent' : 'text-muted');
  }

  function applyState(s) {
    if (!s) return;
    elCount.textContent  = (s.products_count ?? 0).toLocaleString('it-IT');
    elSynced.textContent = fmtDate(s.last_synced_at);

    if (typeof s.diag_full_nutrients === 'number') {
      elFull.textContent    = s.diag_full_nutrients.toLocaleString('it-IT');
      elPartial.textContent = (s.diag_partial_nutrients ?? 0).toLocaleString('it-IT');
    }

    if (s.diag_sample_full && s.diag_sample_full.length > 0) {
      sampleRow.classList.remove('hidden');
      sampleList.innerHTML = s.diag_sample_full.map(p =>
        `<li>${p.name} (${p.barcode}): ${p.kcal.toFixed(0)} kcal · P ${p.protein.toFixed(1)}g · C ${p.carbs.toFixed(1)}g · F ${p.fat.toFixed(1)}g</li>`
      ).join('');
    } else {
      sampleRow.classList.add('hidden');
    }

    if (s.syncing) {
      elState.textContent = 'Sync in corso…';
      btn.disabled = true;
      forceCb.disabled = true;
    } else {
      elState.textContent = 'Inattivo';
      btn.disabled = false;
      forceCb.disabled = false;
    }

    if (s.last_error) {
      errRow.classList.remove('hidden');
      errMsg.textContent = s.last_error;
    } else {
      errRow.classList.add('hidden');
    }
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/profile/off-status', { credentials: 'same-origin' });
      if (!res.ok) throw new Error('status ' + res.status);
      const state = await res.json();
      applyState(state);
      return state;
    } catch (e) {
      setFeedback('Errore lettura stato: ' + e.message, 'err');
      return null;
    }
  }

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(async () => {
      const s = await fetchStatus();
      if (s && !s.syncing) {
        stopPolling();
        const full = s.diag_full_nutrients ?? 0;
        setFeedback(
          `Sync completato — ${(s.products_count || 0).toLocaleString('it-IT')} prodotti, ${full.toLocaleString('it-IT')} con macro completi.`,
          'ok'
        );
      }
    }, 5000);
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  btn.addEventListener('click', async () => {
    const force = !!forceCb.checked;
    const msg = force
      ? 'Forzare il re-download completo (~900MB, ~10 min)?'
      : 'Avviare un sync OFF? (~10 min se il dump remoto è cambiato, istantaneo se no)';
    if (!window.confirm(msg)) return;

    btn.disabled = true;
    setFeedback('Avvio sync…', 'info');

    try {
      const url = '/profile/off-sync' + (force ? '?force=true' : '');
      const res = await fetch(url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-CSRF-Token': csrfToken },
      });
      const state = await res.json().catch(() => null);

      if (res.status === 202) {
        applyState(state);
        setFeedback('Sync avviato in background. Aggiornamento ogni 5s…', 'info');
        startPolling();
      } else if (res.status === 409) {
        applyState(state);
        setFeedback('Un sync è già in corso.', 'info');
        startPolling();
      } else {
        setFeedback('Errore avvio (HTTP ' + res.status + ')', 'err');
        btn.disabled = false;
      }
    } catch (e) {
      setFeedback('Errore di rete: ' + e.message, 'err');
      btn.disabled = false;
    }
  });

  fetchStatus().then((s) => {
    if (s && s.syncing) startPolling();
  });
})();
