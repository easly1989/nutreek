// workout_form.js — the workout dialog (#251): a search that narrows the activity
// list, an estimate of the kcal while the minutes are typed, and Enter in the
// fields of a new activity creates it instead of saving the workout.
(function () {
  "use strict";

  function form(el) { return el && el.closest ? el.closest("form[data-workout-form]") : null; }

  function filter(input) {
    var f = form(input);
    if (!f) return;
    var q = input.value.trim().toLowerCase(), shown = 0;
    f.querySelectorAll(".wk-row").forEach(function (row) {
      var hit = q === "" || row.textContent.toLowerCase().indexOf(q) !== -1;
      row.hidden = !hit;
      if (hit) shown++;
    });
    var empty = f.querySelector("[data-wk-empty]");
    if (empty) empty.hidden = shown !== 0;
  }

  // kcal for the picked activity and the minutes; the weight scales the list's
  // figure (made for 70 kg, as domain.WorkoutKcal does on the server).
  function estimate(f) {
    var out = f.querySelector("[data-wk-estimate]");
    var pick = f.querySelector('input[name="activity_id"]:checked');
    var minutes = parseFloat(f.querySelector('input[name="duration_minutes"]').value);
    if (!out) return;
    if (!pick || !(minutes > 0)) { out.textContent = ""; return; }
    var perHour = parseFloat(pick.dataset.kcal), weight = parseFloat(out.dataset.weight);
    if (weight > 0) perHour *= weight / 70;
    var kcal = Math.round(perHour / 60 * minutes);
    out.textContent = out.dataset.tpl.replace("{kcal}", kcal.toLocaleString(document.documentElement.lang || undefined));
  }

  document.body.addEventListener("input", function (evt) {
    var t = evt.target;
    if (t.matches && t.matches("[data-wk-search]")) filter(t);
    var f = form(t);
    if (f) estimate(f);
  });
  document.body.addEventListener("change", function (evt) {
    var f = form(evt.target);
    if (f) estimate(f);
  });
  document.body.addEventListener("keydown", function (evt) {
    var t = evt.target;
    if (evt.key !== "Enter" || !t.closest) return;
    if (t.matches("[data-wk-search]")) { evt.preventDefault(); return; }
    var box = t.closest("[data-wk-new]");
    if (box && t.matches("input")) {
      evt.preventDefault();
      var btn = box.querySelector("button");
      if (btn) btn.click();
    }
  });
  // The dialog arrives with the form already filled (a recent pair, an edit).
  document.body.addEventListener("htmx:afterSwap", function (evt) {
    var t = evt.detail && evt.detail.target;
    var f = t && t.querySelector && t.querySelector("form[data-workout-form]");
    if (!f) return;
    estimate(f);
    // Show the picked activity in the list (a new one comes last), once the dialog is open.
    setTimeout(function () {
      var row = f.querySelector('input[name="activity_id"]:checked');
      var list = f.querySelector("[data-wk-list]");
      row = row && row.closest(".wk-row");
      if (!row || !list) return;
      var r = row.getBoundingClientRect(), l = list.getBoundingClientRect();
      list.scrollTop += r.top - l.top - (l.height - r.height) / 2;
    }, 0);
  });
})();
