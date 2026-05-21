(function () {
  'use strict';
  document.addEventListener('click', function (e) {
    const btn = e.target.closest('[data-macro-toggle]');
    if (!btn) return;
    const compact = btn.querySelector('[data-macro-compact]');
    const detail  = btn.querySelector('[data-macro-detail]');
    const chevron = btn.querySelector('[data-macro-chevron]');
    if (!compact || !detail) return;
    compact.classList.toggle('hidden');
    detail.classList.toggle('hidden');
    if (chevron) chevron.classList.toggle('rotate-180');
    btn.setAttribute('aria-expanded', String(!detail.classList.contains('hidden')));
  });
}());
