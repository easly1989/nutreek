(function () {
  'use strict';

  // Il tour parte solo sulla dashboard (path "/") e solo se il server
  // indica che va mostrato (data-show-tour sul body).
  if (window.location.pathname !== '/') return;
  if (!document.body.hasAttribute('data-show-tour')) return;
  if (typeof Shepherd === 'undefined') return;

  var tour = new Shepherd.Tour({
    useModalOverlay: true,
    defaultStepOptions: {
      cancelIcon: { enabled: true },
      scrollTo: { behavior: 'smooth', block: 'center' }
    }
  });

  function btn(label, action, secondary) {
    return {
      text: label,
      action: action,
      classes: secondary ? 'shepherd-button-secondary' : ''
    };
  }

  tour.addStep({
    id: 'benvenuto',
    title: 'Benvenuto in Nutreek!',
    text: 'Questo breve tour ti mostra le funzioni principali della dashboard. Puoi chiuderlo in qualsiasi momento e riavviarlo da "Tour guidato" nel menu laterale.',
    buttons: [
      btn('Salta tour', function () { tour.cancel(); }, true),
      btn('Inizia →', function () { tour.next(); })
    ]
  });

  tour.addStep({
    id: 'navigazione',
    title: 'Navigazione settimana',
    text: 'Usa le frecce per spostarti tra le settimane. "Oggi" torna alla settimana corrente.',
    attachTo: { element: '.flex.items-center.gap-2.text-sm', on: 'bottom' },
    buttons: [btn('← Indietro', function () { tour.back(); }, true), btn('Avanti →', function () { tour.next(); })]
  });

  tour.addStep({
    id: 'timeline',
    title: 'Giorni della settimana',
    text: 'Clicca su un giorno per aprire la scheda con i pasti di quel giorno e aggiungere alimenti.',
    attachTo: { element: '#timeline-pills', on: 'bottom' },
    buttons: [btn('← Indietro', function () { tour.back(); }, true), btn('Avanti →', function () { tour.next(); })]
  });

  tour.addStep({
    id: 'template',
    title: 'Applica template',
    text: 'Se hai creato un template settimanale (Colazione, Pranzo, Cena…), puoi applicarlo qui per strutturare la settimana in un click.',
    attachTo: { element: 'form[action="/week/apply-template"]', on: 'bottom' },
    when: {
      show: function () {
        // Salta questo step se il form non è visibile (settimana già popolata).
        if (!document.querySelector('form[action="/week/apply-template"]')) {
          tour.next();
        }
      }
    },
    buttons: [btn('← Indietro', function () { tour.back(); }, true), btn('Avanti →', function () { tour.next(); })]
  });

  tour.addStep({
    id: 'sidebar',
    title: 'Menu laterale',
    text: 'Da qui accedi a tutte le sezioni: ingredienti, ricette, piano nutrizionale, statistiche. Puoi fissare la sidebar con "Fissa" o ridurla a soli icon con "Riduci".',
    attachTo: { element: '#sidebar', on: 'right' },
    buttons: [
      btn('← Indietro', function () { tour.back(); }, true),
      btn('Fine ✓', function () { tour.complete(); })
    ]
  });

  function dismissTour() {
    // Recupera il CSRF token dal DOM (il form di logout lo ha sempre).
    var tokenEl = document.querySelector('input[name="csrf_token"]');
    var token = tokenEl ? tokenEl.value : '';
    var body = 'csrf_token=' + encodeURIComponent(token);
    // Usa sendBeacon se disponibile (funziona anche su chiusura pagina).
    if (navigator.sendBeacon) {
      var blob = new Blob([body], { type: 'application/x-www-form-urlencoded' });
      navigator.sendBeacon('/tour/dismiss', blob);
    } else {
      fetch('/tour/dismiss', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body,
        keepalive: true
      });
    }
    document.body.removeAttribute('data-show-tour');
  }

  tour.on('complete', dismissTour);
  tour.on('cancel',   dismissTour);

  // Leggero ritardo per aspettare il rendering completo della pagina.
  setTimeout(function () { tour.start(); }, 900);
})();
