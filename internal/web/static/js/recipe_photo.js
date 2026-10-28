// Recipe photo (#207): shrinks the chosen picture in the browser and uploads
// it as a JPEG. Drawing it keeps phone photos upright and turns what the
// server cannot read (WebP, HEIC) into JPEG. Without this script the form
// posts the file as it is.
(function () {
  'use strict';

  var MAX_SIDE = 1600;          // the server stores at most 1200 px
  var MAX_FILE = 10 * 1024 * 1024; // the size the page promises

  // shrink draws file at most MAX_SIDE px wide or tall and returns a JPEG of
  // at most max bytes, lowering the quality when needed, or null.
  async function shrink(file, max) {
    var bmp;
    try {
      bmp = await createImageBitmap(file, { imageOrientation: 'from-image' });
    } catch (_) {
      // Older Safari takes no options; browsers apply EXIF orientation anyway.
      bmp = await createImageBitmap(file);
    }
    var scale = Math.min(1, MAX_SIDE / Math.max(bmp.width, bmp.height));
    var canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bmp.width * scale));
    canvas.height = Math.max(1, Math.round(bmp.height * scale));
    var ctx = canvas.getContext('2d');
    ctx.fillStyle = '#fff'; // transparency turns white, as on the server
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(bmp, 0, 0, canvas.width, canvas.height);
    bmp.close();
    for (var q = 0.9; q > 0.45; q -= 0.1) {
      var blob = await new Promise(function (done) { canvas.toBlob(done, 'image/jpeg', q); });
      if (blob && blob.size <= max) return blob;
    }
    return null;
  }

  document.addEventListener('change', async function (e) {
    var input = e.target;
    if (!input.matches || !input.matches('[data-photo-input]')) return;
    var file = input.files && input.files[0];
    if (!file) return;
    var form = input.form;
    if (form.hasAttribute('aria-busy')) { // one upload at a time, keyboard included
      input.value = '';
      return;
    }
    var box = form.closest('[data-photo]');
    var status = box.querySelector('[data-photo-status]');
    var error = box.querySelector('[data-photo-error]');
    var show = function (msg) { error.textContent = msg; error.hidden = !msg; };
    var max = parseInt(form.dataset.photoMax, 10) || MAX_FILE;

    show('');
    if (file.size > MAX_FILE) {
      show(form.dataset.msgTooLarge);
      input.value = '';
      return;
    }
    form.setAttribute('aria-busy', 'true');
    status.hidden = false;
    var reloading = false;
    try {
      var upload;
      try {
        upload = await shrink(file, max);
      } catch (_) {
        upload = file; // the browser cannot draw it: the server says why
      }
      if (!upload || upload.size > max) {
        show(form.dataset.msgTooLarge);
        return;
      }
      var data = new FormData();
      data.append('csrf_token', form.elements.csrf_token.value);
      data.append('photo', upload, upload === file ? file.name : 'photo.jpg');
      var res = await fetch(form.action, {
        method: 'POST',
        body: data,
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'fetch' }
      });
      if (res.ok) {
        reloading = true; // busy until the page shows the photo
        location.reload();
        return;
      }
      // The app's errors are translated plain text; the demo's body limit
      // answers 413 before the app, untranslated.
      var plain = (res.headers.get('Content-Type') || '').indexOf('text/plain') === 0;
      var text = plain && res.status !== 413 ? (await res.text()).trim() : '';
      show(text || (res.status === 413 ? form.dataset.msgTooLarge : form.dataset.msgFailed));
    } catch (_) {
      show(form.dataset.msgFailed);
    } finally {
      if (!reloading) {
        form.removeAttribute('aria-busy');
        status.hidden = true;
        input.value = '';
      }
    }
  });
}());
