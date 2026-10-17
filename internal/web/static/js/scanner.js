// scanner.js — the barcode scanner in the «Aggiungi alimento» pickers (#59).
//
// A [data-scan-for="#input"] button opens the #scanner dialog with the rear
// camera. Once a valid code is read, the dialog closes, the code goes into
// the given search field and the search starts right away (the "search"
// event, see ingredient_search.js): the server recognizes the code and
// answers with the product already open, or with «non trovato».
//
// Reading uses the native BarcodeDetector (Chrome Android); where it is
// missing it loads the ZXing polyfill from /static/vendor/barcode-detector
// (iPhone, Firefox). Camera and service worker exist only in a secure context
// (https or localhost): elsewhere the [.scan-only] buttons stay hidden.
(function () {
  "use strict";

  var FORMATS = ["ean_13", "ean_8", "upc_a"];
  var VENDOR = "/static/vendor/barcode-detector/";
  var canScan = window.isSecureContext && navigator.mediaDevices &&
    typeof navigator.mediaDevices.getUserMedia === "function";
  if (canScan) document.documentElement.classList.add("can-scan");

  var dlg, video, stream, detector, timer, target;
  // Every opening is a session: a camera or readings that arrive after the
  // close (or from an earlier opening) are discarded.
  var session = 0;

  // The same rule as off.NormalizeBarcode: EAN-13, EAN-8, UPC-A (as 13 digits)
  // with the right check digit. "" when it is not a code.
  function normalize(s) {
    var code = String(s || "").replace(/\s+/g, "");
    if (code.length === 12) code = "0" + code;
    if (!/^\d+$/.test(code) || (code.length !== 8 && code.length !== 13)) return "";
    var sum = 0;
    for (var i = 0; i < code.length - 1; i++) {
      var d = +code[i];
      sum += (code.length - 1 - i) % 2 === 1 ? d * 3 : d;
    }
    return (10 - sum % 10) % 10 === +code[code.length - 1] ? code : "";
  }

  function part(name) { return dlg.querySelector("[data-scan-" + name + "]"); }

  function show(state, text) {
    part("cam").classList.toggle("hidden", state !== "cam");
    part("problem").classList.toggle("hidden", state !== "problem");
    if (text) part("problem-text").textContent = text;
  }

  function loadPonyfill() {
    if (window.BarcodeDetectionAPI) return Promise.resolve(window.BarcodeDetectionAPI);
    return new Promise(function (resolve, reject) {
      var s = document.createElement("script");
      s.src = VENDOR + "ponyfill.js";
      s.onload = function () { resolve(window.BarcodeDetectionAPI); };
      s.onerror = reject;
      document.head.appendChild(s);
    });
  }

  function loadDetector() {
    if (detector) return Promise.resolve(detector);
    var native = "BarcodeDetector" in window
      ? window.BarcodeDetector.getSupportedFormats().catch(function () { return []; })
      : Promise.resolve([]);
    return native.then(function (supported) {
      var formats = FORMATS.filter(function (f) { return supported.indexOf(f) >= 0; });
      if (formats.indexOf("ean_13") >= 0) return new window.BarcodeDetector({ formats: formats });
      return loadPonyfill().then(function (api) {
        // The .wasm from here, not from the package's default CDN.
        api.prepareZXingModule({
          overrides: {
            locateFile: function (path, prefix) {
              return path.slice(-5) === ".wasm" ? VENDOR + path : prefix + path;
            },
          },
        });
        return new api.BarcodeDetector({ formats: FORMATS });
      });
    }).then(function (d) { detector = d; return d; });
  }

  function stop() {
    session++;
    clearTimeout(timer);
    if (stream) stream.getTracks().forEach(function (t) { t.stop(); });
    stream = null;
    if (video) video.srcObject = null;
  }

  function loop(my) {
    if (my !== session) return;
    detector.detect(video).then(function (found) {
      if (my !== session) return;
      for (var i = 0; i < found.length; i++) {
        var code = normalize(found[i].rawValue);
        if (code) { done(code); return; }
      }
      timer = setTimeout(loop, 200, my);
    }).catch(function () { if (my === session) timer = setTimeout(loop, 400, my); });
  }

  function start() {
    stop();
    var my = session;
    show("cam");
    navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: "environment" } }, audio: false })
      .then(function (s) {
        if (my !== session) { s.getTracks().forEach(function (t) { t.stop(); }); return; }
        stream = s;
        video.srcObject = s;
        return video.play().then(loadDetector).then(function () { loop(my); });
      })
      .catch(function (err) {
        if (my !== session) return;
        stop();
        var name = err && err.name;
        // The texts come translated from the dialog's data-msg-* attributes.
        show("problem", name === "NotAllowedError" || name === "SecurityError"
          ? dlg.dataset.msgDenied
          : name === "NotFoundError" || name === "OverconstrainedError"
            ? dlg.dataset.msgNotFound
            : dlg.dataset.msgUnsupported);
      });
  }

  function done(code) {
    stop();
    if (navigator.vibrate) navigator.vibrate(60);
    var input = target;
    dlg.close();
    if (!input || !input.isConnected) return;
    input.value = code;
    // "search" = ricerca immediata, OpenFoodFacts compreso (ingredient_search.js).
    input.dispatchEvent(new Event("search"));
    var list = input.dataset.searchTarget && document.querySelector(input.dataset.searchTarget);
    if (list) list.scrollTop = 0;
  }

  function open(input) {
    dlg = dlg || document.getElementById("scanner");
    if (!dlg || !input) return;
    video = part("video");
    target = input;
    part("code").value = "";
    part("error").classList.add("hidden");
    dlg.showModal();
    start();
  }

  // A picker's search field: the one that writes into the list holding the
  // button (for «Scansiona un altro» and «Riprova»).
  function inputFor(btn) {
    for (var el = btn; el && el !== document.body; el = el.parentElement) {
      if (!el.id) continue;
      var input = document.querySelector('input[data-search-target="#' + el.id + '"]');
      if (input) return input;
    }
    return null;
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest && e.target.closest("[data-scan-for], [data-scan-again]");
    if (!btn) return;
    e.preventDefault();
    open(btn.dataset.scanFor ? document.querySelector(btn.dataset.scanFor) : inputFor(btn));
  });

  document.addEventListener("submit", function (e) {
    if (!e.target.matches || !e.target.matches("[data-scan-manual]")) return;
    e.preventDefault();
    var code = normalize(part("code").value);
    if (!code) { part("error").classList.remove("hidden"); return; }
    done(code);
  });

  // Once the dialog is closed (X, Esc, backdrop) the camera turns off.
  document.addEventListener("close", function (e) {
    if (e.target.id === "scanner") stop();
  }, true);
  // X and backdrop: the content fills the dialog, so a click whose target is
  // the <dialog> itself comes from the backdrop.
  document.addEventListener("click", function (e) {
    if (e.target.id === "scanner" || (e.target.closest && e.target.closest("[data-scan-close]"))) {
      document.getElementById("scanner").close();
    }
  });
})();
