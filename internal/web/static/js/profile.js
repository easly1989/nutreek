(function () {
  'use strict';

  // ---- Stato modulo ----------------------------------------------------------

  let calcWeight  = null; // peso da #f-weight (validato)
  let calcBMR     = null; // BMR calcolato live
  let calcTDEE    = null; // TDEE = BMR × activityFactor

  let activePreset    = null; // 'maintenance'|'bulk'|'cut'|null
  let pendingRebalance = null; // { kcal?, protein?, carbs?, fat? } da applicare

  // Valori dell'ultimo preset applicato (per l'hint "suggerito: X").
  const suggested = { kcal: null, protein: null, carbs: null, fat: null };

  // Strategia del preset attivo (per rebalance senza preset esplicito).
  // fatPct = % kcal dal grasso; proteinGPerKg = g/kg peso
  const presetStrategy = {
    maintenance: { proteinGPerKg: 1.8, fatPct: 0.25 },
    bulk:        { proteinGPerKg: 2.0, fatPct: 0.25 },
    cut:         { proteinGPerKg: 2.2, fatPct: 0.28 },
    default:     { proteinGPerKg: 1.8, fatPct: 0.25 },
  };

  // ---- Helpers ----------------------------------------------------------------

  function el(id) { return document.getElementById(id); }
  function parseF(s) { return parseFloat(String(s || '').replace(',', '.')); }
  function getSex() {
    const r = document.querySelector('input[name="sex"]:checked');
    return r ? r.value : '';
  }
  function bmiCategory(bmi) {
    if (bmi < 18.5) return 'Sottopeso';
    if (bmi < 25.0) return 'Normopeso';
    if (bmi < 30.0) return 'Sovrappeso';
    return 'Obesità';
  }
  function getActivityFactor() {
    const sel = el('f-activity');
    if (!sel) return 1.4;
    const factors = { sedentary: 1.2, light: 1.375, moderate: 1.55, active: 1.725, athlete: 1.9 };
    return factors[sel.value] || 1.4;
  }
  function getActivityLabel() {
    const sel = el('f-activity');
    if (!sel || !sel.value) return 'default';
    const labels = { sedentary: 'Sedentario', light: 'Leggera', moderate: 'Moderata', active: 'Intensa', athlete: 'Atleta' };
    return labels[sel.value] || '';
  }

  // ---- BMI / BMR / TDEE live --------------------------------------------------

  function updateIndices() {
    const weight    = parseF(el('f-weight') && el('f-weight').value);
    const height    = parseF(el('f-height') && el('f-height').value);
    const birthYear = parseInt((el('f-birth') && el('f-birth').value) || '', 10);
    const sex       = getSex();
    const age       = new Date().getFullYear() - birthYear;

    const dyn   = el('indices-dynamic');
    const ph    = el('indices-placeholder');
    const valid = weight >= 20 && weight <= 300 &&
                  height >= 100 && height <= 250 &&
                  !isNaN(birthYear) && age >= 5 && age <= 130 &&
                  (sex === 'M' || sex === 'F');

    if (!valid) {
      if (dyn) dyn.classList.add('hidden');
      if (ph)  ph.classList.remove('hidden');
      calcWeight = null; calcBMR = null; calcTDEE = null;
      setPresetButtonsEnabled(false);
      return;
    }

    const bmi  = weight / Math.pow(height / 100, 2);
    const base = 10 * weight + 6.25 * height - 5 * age;
    const bmr  = sex === 'M' ? base + 5 : base - 161;

    calcWeight = weight; calcBMR = bmr;

    const bmiV = el('bmi-value'); const bmiC = el('bmi-category'); const bmrV = el('bmr-value');
    if (bmiV) bmiV.textContent = bmi.toFixed(1);
    if (bmiC) bmiC.textContent = bmiCategory(bmi);
    if (bmrV) bmrV.textContent = Math.round(bmr);

    if (dyn) dyn.classList.remove('hidden');
    if (ph)  ph.classList.add('hidden');

    updateTDEE();
    setPresetButtonsEnabled(true);
  }

  function updateTDEE() {
    const tdeeV = el('tdee-value');
    const tdeeL = el('tdee-label');
    if (!calcBMR) { calcTDEE = null; return; }
    const factor = getActivityFactor();
    calcTDEE = calcBMR * factor;
    if (tdeeV) tdeeV.textContent = Math.round(calcTDEE);
    if (tdeeL) {
      const lbl = getActivityLabel();
      tdeeL.textContent = lbl ? 'fabbisogno (' + lbl.toLowerCase() + ')' : 'fabbisogno stimato';
    }
  }

  // ---- Preset buttons ---------------------------------------------------------

  function setPresetButtonsEnabled(enabled) {
    document.querySelectorAll('.preset-btn').forEach(function (btn) { btn.disabled = !enabled; });
    const hint = el('presets-hint');
    if (hint) hint.classList.toggle('hidden', enabled);
  }

  function calcPresetValues(mode) {
    const tdee = calcTDEE || (calcBMR * 1.4);
    const s    = presetStrategy[mode] || presetStrategy.default;
    let kcal, protein, fat, carbs;

    if (mode === 'bulk')        kcal = Math.round(tdee * 1.10);
    else if (mode === 'cut')    kcal = Math.round(tdee * 0.80);
    else                        kcal = Math.round(tdee);

    protein = Math.round(calcWeight * s.proteinGPerKg);
    fat     = Math.round(kcal * s.fatPct / 9);
    carbs   = Math.round((kcal - protein * 4 - fat * 9) / 4);
    if (carbs < 0) carbs = 0;

    return { kcal: kcal, protein: protein, carbs: carbs, fat: fat };
  }

  function applyPreset(mode) {
    if (!calcWeight || !calcBMR) return;
    const v = calcPresetValues(mode);
    activePreset = mode;

    suggested.kcal    = v.kcal;
    suggested.protein = v.protein;
    suggested.carbs   = v.carbs;
    suggested.fat     = v.fat;

    setFieldValue('f-tk', v.kcal);
    setFieldValue('f-tp', v.protein);
    setFieldValue('f-tc', v.carbs);
    setFieldValue('f-tf', v.fat);

    // Evidenzia il pulsante attivo.
    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.classList.toggle('bg-accent-soft', btn.dataset.preset === mode);
      btn.classList.toggle('border-accent',  btn.dataset.preset === mode);
    });

    ['f-tk', 'f-tp', 'f-tc', 'f-tf'].forEach(function (id) {
      setHint(id, null); setError(id, null);
    });
    hideRebalancePanel();
    updateMacroSummary();
    validateAll();
  }

  function setFieldValue(id, val) { const f = el(id); if (f) f.value = val; }

  // ---- Reset macro ------------------------------------------------------------

  function resetMacros() {
    ['f-tk', 'f-tp', 'f-tc', 'f-tf'].forEach(function (id) {
      const f = el(id); if (f) f.value = '';
      setHint(id, null); setError(id, null);
    });
    suggested.kcal = suggested.protein = suggested.carbs = suggested.fat = null;
    activePreset = null;
    pendingRebalance = null;

    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.classList.remove('bg-accent-soft', 'border-accent');
    });
    hideRebalancePanel();
    updateMacroSummary();
    const saveBtn = el('profile-save-btn');
    if (saveBtn) saveBtn.disabled = false;
  }

  // ---- Rebalance panel --------------------------------------------------------

  function hideRebalancePanel() {
    const panel = el('rebalance-panel');
    if (panel) panel.classList.add('hidden');
    pendingRebalance = null;
  }

  function showRebalancePanel(suggestion) {
    const panel = el('rebalance-panel');
    const text  = el('rebalance-text');
    if (!panel || !text) return;
    pendingRebalance = suggestion;
    const parts = [];
    if (suggestion.kcal    !== undefined) parts.push('Kcal ' + suggestion.kcal);
    if (suggestion.protein !== undefined) parts.push('P ' + suggestion.protein + 'g');
    if (suggestion.carbs   !== undefined) parts.push('C ' + suggestion.carbs + 'g');
    if (suggestion.fat     !== undefined) parts.push('F ' + suggestion.fat + 'g');
    text.textContent = parts.join(' · ');
    panel.classList.remove('hidden');
  }

  function applyRebalance() {
    if (!pendingRebalance) return;
    const r = pendingRebalance;
    if (r.kcal    !== undefined) setFieldValue('f-tk', r.kcal);
    if (r.protein !== undefined) setFieldValue('f-tp', r.protein);
    if (r.carbs   !== undefined) setFieldValue('f-tc', r.carbs);
    if (r.fat     !== undefined) setFieldValue('f-tf', r.fat);
    // Aggiorna i suggeriti con i nuovi valori applicati.
    if (r.kcal    !== undefined) suggested.kcal    = r.kcal;
    if (r.protein !== undefined) suggested.protein = r.protein;
    if (r.carbs   !== undefined) suggested.carbs   = r.carbs;
    if (r.fat     !== undefined) suggested.fat     = r.fat;
    ['f-tk', 'f-tp', 'f-tc', 'f-tf'].forEach(function (id) { setHint(id, null); });
    hideRebalancePanel();
    updateMacroSummary();
    validateAll();
  }

  // Calcola la suggestion di rebalance quando l'utente modifica un campo macro.
  // Fissa il valore del campo modificato e ricalcola gli altri.
  function computeRebalance(changedKey) {
    const kcal    = parseF(el('f-tk') && el('f-tk').value);
    const protein = parseF(el('f-tp') && el('f-tp').value);
    const carbs   = parseF(el('f-tc') && el('f-tc').value);
    const fat     = parseF(el('f-tf') && el('f-tf').value);

    const s = presetStrategy[activePreset] || presetStrategy.default;

    if (changedKey === 'kcal') {
      // Ricalcola P/F/C a partire dalle nuove Kcal.
      if (!isNaN(kcal) && kcal >= 800) {
        let newP, newF, newC;
        if (calcWeight) {
          newP = Math.round(calcWeight * s.proteinGPerKg);
        } else {
          newP = Math.round(kcal * 0.20 / 4); // 20% proteine come default
        }
        newF = Math.round(kcal * s.fatPct / 9);
        newC = Math.round((kcal - newP * 4 - newF * 9) / 4);
        if (newC < 0) newC = 0;
        return { protein: newP, carbs: newC, fat: newF };
      }
      return null;
    }

    if (changedKey === 'protein') {
      // Ricalcola F (% kcal) e C (filler), mantenendo Kcal.
      if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && protein >= 0) {
        const newF = Math.round(kcal * s.fatPct / 9);
        const newC = Math.round((kcal - protein * 4 - newF * 9) / 4);
        if (newC >= 0) return { carbs: newC, fat: newF };
      }
      return null;
    }

    if (changedKey === 'fat') {
      // Ricalcola C come filler, mantenendo Kcal e P.
      if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && !isNaN(fat) && fat >= 0) {
        const newC = Math.round((kcal - protein * 4 - fat * 9) / 4);
        if (newC >= 0) return { carbs: newC };
      }
      return null;
    }

    if (changedKey === 'carbs') {
      // Ricalcola F come filler, mantenendo Kcal e P.
      if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && !isNaN(carbs) && carbs >= 0) {
        const remaining = kcal - protein * 4 - carbs * 4;
        const newF = Math.round(remaining / 9);
        if (newF >= 0) return { fat: newF };
      }
      return null;
    }

    return null;
  }

  // ---- Hints, errors, summary ------------------------------------------------

  function setHint(fieldId, text) {
    const n = el('hint-' + fieldId);
    if (!n) return;
    n.textContent = text || '';
    n.classList.toggle('hidden', !text);
  }

  function setError(fieldId, text) {
    const n = el('err-' + fieldId);
    if (!n) return;
    n.textContent = text || '';
    n.classList.toggle('hidden', !text);
  }

  function updateMacroSummary() {
    const summary = el('macro-summary');
    if (!summary) return;
    const kcal    = parseF(el('f-tk') && el('f-tk').value);
    const protein = parseF(el('f-tp') && el('f-tp').value);
    const carbs   = parseF(el('f-tc') && el('f-tc').value);
    const fat     = parseF(el('f-tf') && el('f-tf').value);

    if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && !isNaN(carbs) && !isNaN(fat)) {
      const macroKcal = protein * 4 + carbs * 4 + fat * 9;
      const pP = Math.round(protein * 4 / kcal * 100);
      const pC = Math.round(carbs   * 4 / kcal * 100);
      const pF = Math.round(fat     * 9 / kcal * 100);
      summary.textContent = 'P: ' + pP + '% · C: ' + pC + '% · F: ' + pF + '% · totale macro: ' + Math.round(macroKcal) + ' kcal';
      summary.classList.remove('hidden');
    } else {
      summary.classList.add('hidden');
    }
  }

  function onMacroInput(key, fieldId) {
    // Hint "suggerito: X" se diverge dal valore del preset.
    if (suggested[key] !== null) {
      const val = parseF(el(fieldId) && el(fieldId).value);
      if (!isNaN(val) && Math.round(val) !== suggested[key]) {
        setHint(fieldId, 'suggerito: ' + suggested[key]);
      } else {
        setHint(fieldId, null);
      }
    }
    // Pannello rebalance.
    const suggestion = computeRebalance(key);
    if (suggestion) {
      showRebalancePanel(suggestion);
    } else {
      hideRebalancePanel();
    }
    updateMacroSummary();
    validateAll();
  }

  // ---- Validazione client-side ------------------------------------------------

  function validateAll() {
    const kcal    = parseF(el('f-tk') && el('f-tk').value);
    const protein = parseF(el('f-tp') && el('f-tp').value);
    const carbs   = parseF(el('f-tc') && el('f-tc').value);
    const fat     = parseF(el('f-tf') && el('f-tf').value);

    setError('f-tk', null); setError('f-tp', null);
    setError('f-tc', null); setError('f-tf', null);
    let hasError = false;

    function err(id, msg) { setError(id, msg); hasError = true; }

    if (!isNaN(kcal) && kcal > 0 && kcal < 800)
      err('f-tk', 'Valore troppo basso (min 800 kcal).');
    if (!isNaN(protein) && protein > 400)
      err('f-tp', 'Valore non realistico (max 400 g).');
    if (!isNaN(fat) && fat > 0 && fat < 20)
      err('f-tf', 'Valore troppo basso (min 20 g per i grassi essenziali).');

    if (!isNaN(kcal) && kcal > 0) {
      if (!isNaN(protein) && protein > 0 && protein * 4 > kcal)
        err('f-tp', 'Le proteine da sole superano le calorie totali.');
      if (!isNaN(fat) && fat > 0 && fat * 9 > kcal)
        err('f-tf', 'I grassi da soli superano le calorie totali.');
      if (!isNaN(protein) && !isNaN(carbs) && !isNaN(fat) && protein >= 0 && carbs >= 0 && fat >= 0) {
        const macroKcal = protein * 4 + carbs * 4 + fat * 9;
        if (macroKcal > kcal * 1.10)
          err('f-tc', 'La somma dei macro supera le calorie totali di oltre il 10%.');
      }
    }

    const saveBtn = el('profile-save-btn');
    if (saveBtn) saveBtn.disabled = hasError;
  }

  // ---- Init -------------------------------------------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    // Dati fisici → indici live.
    ['f-weight', 'f-height', 'f-birth'].forEach(function (id) {
      const f = el(id); if (f) f.addEventListener('input', updateIndices);
    });
    document.querySelectorAll('input[name="sex"]').forEach(function (r) {
      r.addEventListener('change', updateIndices);
    });
    // Livello attività → aggiorna solo TDEE (BMR non cambia).
    const actSel = el('f-activity');
    if (actSel) actSel.addEventListener('change', updateTDEE);

    // Pulsanti preset.
    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.preventDefault(); applyPreset(this.dataset.preset);
      });
    });

    // Reset macro.
    const resetBtn = el('btn-reset-macros');
    if (resetBtn) resetBtn.addEventListener('click', function (e) { e.preventDefault(); resetMacros(); });

    // Campi macro → hint + rebalance + validazione.
    [['kcal', 'f-tk'], ['protein', 'f-tp'], ['carbs', 'f-tc'], ['fat', 'f-tf']].forEach(function (p) {
      const f = el(p[1]);
      if (f) f.addEventListener('input', function () { onMacroInput(p[0], p[1]); });
    });

    // Rebalance panel buttons.
    const applyBtn   = el('rebalance-apply');
    const dismissBtn = el('rebalance-dismiss');
    if (applyBtn)   applyBtn.addEventListener('click',   function () { applyRebalance(); });
    if (dismissBtn) dismissBtn.addEventListener('click', function () { hideRebalancePanel(); });

    // Calcolo iniziale (dati già presenti dal server).
    updateIndices();
    validateAll();
    updateMacroSummary();
  });
}());
