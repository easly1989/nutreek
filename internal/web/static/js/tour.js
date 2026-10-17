(function () {
  'use strict';

  // The tour starts only on the dashboard (path "/") and only when the server
  // says it must be shown (data-show-tour on the body).
  if (window.location.pathname !== '/') return;
  if (!document.body.hasAttribute('data-show-tour')) return;
  if (typeof Shepherd === 'undefined') return;

  // The texts come from the page, in the user's language.
  var tx;
  try { tx = JSON.parse(document.getElementById('tour-steps').textContent); } catch (e) { return; }

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
    title: tx.welcome.title,
    text: tx.welcome.text,
    buttons: [
      btn(tx.buttons.skip, function () { tour.cancel(); }, true),
      btn(tx.buttons.start, function () { tour.next(); })
    ]
  });

  tour.addStep({
    id: 'navigazione',
    title: tx.week.title,
    text: tx.week.text,
    attachTo: { element: '#week-nav', on: 'bottom' },
    buttons: [btn(tx.buttons.back, function () { tour.back(); }, true), btn(tx.buttons.next, function () { tour.next(); })]
  });

  tour.addStep({
    id: 'timeline',
    title: tx.days.title,
    text: tx.days.text,
    attachTo: { element: '#timeline-pills', on: 'bottom' },
    buttons: [btn(tx.buttons.back, function () { tour.back(); }, true), btn(tx.buttons.next, function () { tour.next(); })]
  });

  tour.addStep({
    id: 'template',
    title: tx.template.title,
    text: tx.template.text,
    attachTo: { element: 'form[action="/week/apply-template"]', on: 'bottom' },
    when: {
      show: function () {
        // Skip this step when the form is not visible (week already filled).
        if (!document.querySelector('form[action="/week/apply-template"]')) {
          tour.next();
        }
      }
    },
    buttons: [btn(tx.buttons.back, function () { tour.back(); }, true), btn(tx.buttons.next, function () { tour.next(); })]
  });

  tour.addStep({
    id: 'sidebar',
    title: tx.sections.title,
    text: tx.sections.text,
    attachTo: { element: window.matchMedia('(min-width: 1024px)').matches ? '#sidebar' : '#bottom-nav', on: window.matchMedia('(min-width: 1024px)').matches ? 'right' : 'top' },
    buttons: [
      btn(tx.buttons.back, function () { tour.back(); }, true),
      btn(tx.buttons.finish, function () { tour.complete(); })
    ]
  });

  function dismissTour() {
    // Get the CSRF token from the DOM (the logout form always has it).
    var tokenEl = document.querySelector('input[name="csrf_token"]');
    var token = tokenEl ? tokenEl.value : '';
    var body = 'csrf_token=' + encodeURIComponent(token);
    // Use sendBeacon when available (it works on page close too).
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

  // A small delay to wait for the page to finish rendering.
  setTimeout(function () { tour.start(); }, 900);
})();
