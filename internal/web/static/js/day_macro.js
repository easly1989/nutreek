(function () {
  'use strict';
  document.addEventListener('click', function (e) {
    const btn = e.target.closest('[data-macro-toggle]');
    if (!btn) return;
    const compact = btn.querySelector('[data-macro-compact]');
    const detail  = btn.querySelector('[data-macro-detail]');
    if (!compact || !detail) return;
    compact.classList.toggle('hidden');
    detail.classList.toggle('hidden');
    btn.setAttribute('aria-expanded', String(!detail.classList.contains('hidden')));
    btn.blur();
  });
}());
