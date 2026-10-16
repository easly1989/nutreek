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
      '<div class="menu-panel max-w-sm w-full p-5">' +
        '<p class="text-text mb-5 leading-relaxed" id="_confirm-msg"></p>' +
        '<div class="flex justify-end gap-2">' +
          '<button id="_confirm-cancel" class="btn-secondary">Annulla</button>' +
          '<button id="_confirm-ok" class="btn bg-danger text-surface hover:opacity-90">Conferma</button>' +
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

  // Intercept the submit of forms with data-confirm
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

  // Intercept HTMX's hx-confirm
  document.addEventListener('htmx:confirm', function (e) {
    if (!e.detail.question) return;
    e.preventDefault();
    showDialog(e.detail.question, function () { e.detail.issueRequest(true); });
  });

  // Expose showDialog globally for inline JS cases (recipes_list)
  window._showConfirm = showDialog;
})();
