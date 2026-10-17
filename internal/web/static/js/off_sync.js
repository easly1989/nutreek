// State and refresh of the local OpenFoodFacts copy, in the Administration
// Overview (owner only). The elements to fill have data-off="<field>" and
// can appear more than once (tile and card). The texts come translated in the
// card's data-msg-* and data-confirm-* attributes; {name} marks a value.
(function () {
  'use strict';

  const card = document.getElementById('off-sync-card');
  if (!card) return;

  const csrfToken = card.dataset.csrf || '';
  const locale = card.dataset.locale || undefined;
  const btn = document.getElementById('off-sync-btn');
  const forceCb = document.getElementById('off-force');
  const feedback = document.getElementById('off-feedback');
  let pollTimer = null;

  // msg returns the text of data-msg-<name> with the {key} values filled in.
  function msg(name, vars) {
    let text = card.dataset['msg' + name] || '';
    Object.keys(vars || {}).forEach(function (k) { text = text.replace('{' + k + '}', vars[k]); });
    return text;
  }
  function fmt(n) { return Number(n || 0).toLocaleString(locale, { useGrouping: 'always' }); }
  function set(field, text) {
    document.querySelectorAll('[data-off="' + field + '"]').forEach(function (el) { el.textContent = text; });
  }
  function fmtDate(s) {
    if (!s) return msg('Never');
    const d = new Date(s);
    if (isNaN(d)) return s;
    return d.toLocaleDateString(locale, { day: 'numeric', month: 'long' }) + ', ' +
      d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' });
  }
  function relDate(s) {
    if (!s) return msg('NeverRel');
    const days = Math.floor((Date.now() - new Date(s).getTime()) / 86400000);
    if (isNaN(days)) return '';
    if (days <= 0) return msg('Today');
    if (days === 1) return msg('Yesterday');
    return msg('DaysAgo', { n: days });
  }

  function setFeedback(text, kind) {
    feedback.hidden = !text;
    feedback.classList.toggle('hidden', !text);
    feedback.textContent = text || '';
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
    set('state', s.syncing ? msg('Syncing') : (count ? msg('Ready') : msg('Empty')));
    btn.disabled = !!s.syncing;
    forceCb.disabled = !!s.syncing;
    const err = card.querySelector('[data-off="error"]');
    err.classList.toggle('hidden', !s.last_error);
    err.textContent = s.last_error ? msg('LastError', { e: s.last_error }) : '';
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/profile/off-status', { credentials: 'same-origin' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const state = await res.json();
      applyState(state);
      return state;
    } catch (e) {
      setFeedback(msg('StatusFailed', { e: e.message }), 'err');
      return null;
    }
  }

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(async function () {
      const s = await fetchStatus();
      if (s && !s.syncing) {
        stopPolling();
        setFeedback(msg('Finished', { n: fmt(s.products_count) }), 'ok');
      }
    }, 5000);
  }
  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  async function start() {
    const force = !!forceCb.checked;
    btn.disabled = true;
    setFeedback(msg('Starting'), 'info');
    try {
      const res = await fetch('/profile/off-sync' + (force ? '?force=true' : ''), {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-CSRF-Token': csrfToken },
      });
      const state = await res.json().catch(function () { return null; });
      if (res.status === 202 || res.status === 409) {
        applyState(state);
        setFeedback(res.status === 202 ? msg('Started') : msg('AlreadyRunning'), 'info');
        startPolling();
      } else {
        setFeedback(msg('StartFailed', { status: res.status }), 'err');
        btn.disabled = false;
      }
    } catch (e) {
      setFeedback(msg('NetworkError', { e: e.message }), 'err');
      btn.disabled = false;
    }
  }

  btn.addEventListener('click', function () {
    const text = forceCb.checked ? card.dataset.confirmForce : card.dataset.confirmRefresh;
    if (window._showConfirm) window._showConfirm(text, start);
    else if (window.confirm(text)) start();
  });

  fetchStatus().then(function (s) { if (s && s.syncing) startPolling(); });
})();
