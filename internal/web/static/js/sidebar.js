// Hide or show the desktop sidebar. The choice is kept in this browser (the
// head of base.html reads it before the first paint, so there is no jump).
(function () {
  var KEY = 'nutreek.sidebar';
  var root = document.documentElement;
  var hide = document.getElementById('sidebar-hide');
  var show = document.getElementById('sidebar-show');
  if (!hide || !show) return;

  function set(hidden) {
    if (hidden) root.setAttribute('data-side', 'hidden');
    else root.removeAttribute('data-side');
    try { localStorage.setItem(KEY, hidden ? 'hidden' : 'shown'); } catch (e) { /* private window: not remembered */ }
    // The button that was clicked disappears: keep the keyboard where it was.
    (hidden ? show : hide).focus();
  }

  hide.addEventListener('click', function () { set(true); });
  show.addEventListener('click', function () { set(false); });
})();
