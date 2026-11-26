// Passkeys (#336): signing in with one, and adding one from Settings. The server makes the
// challenge and checks the answer; this only carries them between it and the browser.
(function () {
  "use strict";
  if (!window.PublicKeyCredential || !navigator.credentials) return;

  function toBuf(s) {
    s = s.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) s += "=";
    var bin = atob(s), out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out.buffer;
  }
  function toB64(buf) {
    var b = new Uint8Array(buf), s = "";
    for (var i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  function csrf() {
    var f = document.querySelector('input[name="csrf_token"]');
    return f ? f.value : "";
  }
  function post(url, body) {
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: body === undefined ? "{}" : JSON.stringify(body)
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.error || "failed");
        return j;
      });
    });
  }
  function fail(el, msg) {
    var box = document.querySelector("[data-passkey-error]");
    if (box) { box.textContent = msg || box.dataset.default || ""; box.hidden = false; }
    if (el) el.disabled = false;
  }

  // Sign in.
  var loginBtn = document.querySelector("[data-passkey-login]");
  if (loginBtn) {
    loginBtn.hidden = false;
    loginBtn.addEventListener("click", function () {
      loginBtn.disabled = true;
      post("/login/passkey/begin").then(function (opts) {
        var pk = opts.publicKey;
        pk.challenge = toBuf(pk.challenge);
        (pk.allowCredentials || []).forEach(function (c) { c.id = toBuf(c.id); });
        return navigator.credentials.get({ publicKey: pk });
      }).then(function (cred) {
        var r = cred.response;
        return post("/login/passkey/finish", {
          id: cred.id, rawId: toB64(cred.rawId), type: cred.type,
          response: {
            authenticatorData: toB64(r.authenticatorData), clientDataJSON: toB64(r.clientDataJSON),
            signature: toB64(r.signature), userHandle: r.userHandle ? toB64(r.userHandle) : ""
          }
        });
      }).then(function (j) { window.location = j.redirect || "/"; })
        .catch(function () { fail(loginBtn); });
    });
  }

  // Add one.
  var form = document.querySelector("[data-passkey-add]");
  if (form) {
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      var btn = form.querySelector("button[type=submit], button:not([type])");
      if (btn) btn.disabled = true;
      post("/settings/security/passkeys/begin", {
        name: form.elements["name"].value, password: form.elements["password"].value
      }).then(function (opts) {
        var pk = opts.publicKey;
        pk.challenge = toBuf(pk.challenge);
        pk.user.id = toBuf(pk.user.id);
        (pk.excludeCredentials || []).forEach(function (c) { c.id = toBuf(c.id); });
        return navigator.credentials.create({ publicKey: pk });
      }).then(function (cred) {
        var r = cred.response;
        return post("/settings/security/passkeys/finish", {
          id: cred.id, rawId: toB64(cred.rawId), type: cred.type,
          response: {
            attestationObject: toB64(r.attestationObject), clientDataJSON: toB64(r.clientDataJSON),
            transports: r.getTransports ? r.getTransports() : []
          }
        });
      }).then(function (j) { window.location = j.redirect || "/settings/sicurezza"; })
        .catch(function (err) { fail(btn, err && err.message !== "failed" && err.name !== "NotAllowedError" ? err.message : ""); });
    });
  }
})();
