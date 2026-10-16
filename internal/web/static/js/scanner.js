// scanner.js — scanner del codice a barre nei picker «Aggiungi alimento» (#59).
//
// Un bottone [data-scan-for="#input"] apre la finestra #scanner con la
// fotocamera posteriore. Letto un codice valido, la finestra si chiude, il
// codice finisce nel campo di ricerca indicato e parte subito la ricerca
// (evento "search", vedi ingredient_search.js): il server riconosce il codice
// e risponde con il prodotto gia' aperto, oppure con «non trovato».
//
// La lettura usa BarcodeDetector nativo (Chrome Android); dove manca carica
// il polyfill ZXing da /static/vendor/barcode-detector (iPhone, Firefox).
// Fotocamera e service worker esistono solo in contesto sicuro (https o
// localhost): altrove i bottoni [.scan-only] restano nascosti.
(function () {
  "use strict";

  var FORMATS = ["ean_13", "ean_8", "upc_a"];
  var VENDOR = "/static/vendor/barcode-detector/";
  var canScan = window.isSecureContext && navigator.mediaDevices &&
    typeof navigator.mediaDevices.getUserMedia === "function";
  if (canScan) document.documentElement.classList.add("can-scan");

  var dlg, video, stream, detector, timer, target;

  // Stessa regola di off.NormalizeBarcode: EAN-13, EAN-8, UPC-A (a 13 cifre)
  // con la cifra di controllo giusta. "" se non e' un codice.
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
        // Il .wasm da qui, non dal CDN predefinito del pacchetto.
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
    clearTimeout(timer);
    if (stream) stream.getTracks().forEach(function (t) { t.stop(); });
    stream = null;
    if (video) video.srcObject = null;
  }

  function loop() {
    if (!stream) return;
    detector.detect(video).then(function (found) {
      for (var i = 0; i < found.length; i++) {
        var code = normalize(found[i].rawValue);
        if (code) { done(code); return; }
      }
      timer = setTimeout(loop, 200);
    }).catch(function () { timer = setTimeout(loop, 400); });
  }

  function start() {
    show("cam");
    navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: "environment" } }, audio: false })
      .then(function (s) {
        if (!dlg.open) { s.getTracks().forEach(function (t) { t.stop(); }); return; }
        stream = s;
        video.srcObject = s;
        return video.play().then(loadDetector).then(loop);
      })
      .catch(function (err) {
        stop();
        var name = err && err.name;
        show("problem", name === "NotAllowedError" || name === "SecurityError"
          ? "Il browser non ha dato il permesso. Consentilo dall'icona accanto all'indirizzo, oppure scrivi il codice qui sotto."
          : name === "NotFoundError" || name === "OverconstrainedError"
            ? "Non trovo una fotocamera su questo dispositivo. Scrivi il codice qui sotto."
            : "Lo scanner non parte su questo browser. Scrivi il codice qui sotto.");
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

  // Il campo di ricerca di un picker: quello che scrive nella lista che
  // contiene il bottone (per «Scansiona un altro» e «Riprova»).
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

  // Chiusa la finestra (X, Esc, sfondo) la fotocamera si spegne.
  document.addEventListener("close", function (e) {
    if (e.target.id === "scanner") stop();
  }, true);
  // X e sfondo: il contenuto riempie la finestra, quindi un click che ha
  // come bersaglio il <dialog> stesso arriva dallo sfondo.
  document.addEventListener("click", function (e) {
    if (e.target.id === "scanner" || (e.target.closest && e.target.closest("[data-scan-close]"))) {
      document.getElementById("scanner").close();
    }
  });
})();
