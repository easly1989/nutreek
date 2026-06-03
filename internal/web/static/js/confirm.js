(function () {
  function showDialog(message, onConfirm) {
    var existing = document.getElementById('_confirm-overlay');
    if (existing) existing.remove();

    var overlay = document.createElement('div');
    overlay.id = '_confirm-overlay';
    overlay.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.innerHTML =
      '<div class="bg-surface border border-border rounded-lg shadow-xl max-w-sm w-full p-6">' +
        '<p class="text-text text-sm mb-6 leading-relaxed" id="_confirm-msg"></p>' +
        '<div class="flex justify-end gap-3">' +
          '<button id="_confirm-cancel" class="px-4 py-2 text-sm rounded border border-border text-text hover:bg-surface-2 transition">Annulla</button>' +
          '<button id="_confirm-ok" class="px-4 py-2 text-sm rounded bg-danger text-white hover:bg-danger/90 transition">Conferma</button>' +
        '</div>' +
      '</div>';

    overlay.querySelector('#_confirm-msg').textContent = message;
    document.body.appendChild(overlay);

    var cancel = overlay.querySelector('#_confirm-cancel');
    var ok     = overlay.querySelector('#_confirm-ok');

    cancel.addEventListener('click', function () { overlay.remove(); });
    ok.addEventListener('click', function () { overlay.remove(); onConfirm(); });
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) overlay.remove();
    });
    overlay.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); overlay.remove(); }
    });
    cancel.focus();
  }

  // Intercetta submit di form con data-confirm
  document.addEventListener('submit', function (e) {
    var msg = e.target.dataset.confirm;
    if (!msg) return;
    e.preventDefault();
    var form = e.target;
    showDialog(msg, function () {
      delete form.dataset.confirm;
      form.requestSubmit ? form.requestSubmit() : form.submit();
    });
  }, true);

  // Intercetta hx-confirm di HTMX
  document.addEventListener('htmx:confirm', function (e) {
    if (!e.detail.question) return;
    e.preventDefault();
    showDialog(e.detail.question, function () { e.detail.issueRequest(true); });
  });

  // Espone showDialog globalmente per i casi JS inline (recipes_list)
  window._showConfirm = showDialog;
})();
