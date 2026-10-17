(function () {
  'use strict';

  // ---- Stato modulo ----------------------------------------------------------

  let calcWeight  = null; // weight from #f-weight (validated)
  let calcHeight  = null; // height in cm (for IBW)
  let calcSex     = null; // 'M' | 'F' | '' (for IBW)
  let calcBMR     = null; // BMR calcolato live
  let calcTDEE    = null; // TDEE = BMR × activityFactor

  let activePreset    = null; // 'maintenance'|'bulk'|'cut'|null
  let pendingRebalance = null; // { kcal?, protein?, carbs?, fat? } to apply

  // Values of the last preset applied (for the "suggerito: X" hint).
  const suggested = { kcal: null, protein: null, carbs: null, fat: null };

  // Strategy of the active preset. Scientific basis:
  //   - ISSN Position Stand 2017 (protein and exercise):
  //       1.4–2.0 g/kg to build/keep lean mass;
  //       2.3–3.1 g/kg of FFM in a deficit for resistance-trained athletes.
  //   - AMDR (IOM, USDA): fat 20–35% kcal, carbs 45–65%.
  //   - Adjusted Body Weight (Academy of Nutrition & Dietetics):
  //       when BMI ≥ 30 we compute protein on IBW + 0.25*(weight - IBW)
  //       so as not to overestimate the need (lean mass does not scale with fat).
  //
  // kcalFactor: TDEE multiplier
  // proteinGPerKg: g/kg of EFFECTIVE weight (adjusted when BMI≥30)
  // fatPct: % of the kcal from fat
  const presetStrategy = {
    maintenance: { kcalFactor: 1.00, proteinGPerKg: 1.6, fatPct: 0.28 },
    bulk:        { kcalFactor: 1.10, proteinGPerKg: 1.8, fatPct: 0.25 },
    cut:         { kcalFactor: 0.80, proteinGPerKg: 2.0, fatPct: 0.25 },
    default:     { kcalFactor: 1.00, proteinGPerKg: 1.6, fatPct: 0.28 },
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
  // Numbers in the page language: 1.690 / 1,690 (useGrouping "always": without
  // it, 1690 would stay without a separator in Italian).
  const LANG = document.documentElement.lang || 'it';
  function fmt(v, d) {
    return Number(v).toLocaleString(LANG, { minimumFractionDigits: d || 0, maximumFractionDigits: d || 0, useGrouping: 'always' });
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
    const labels = { sedentary: 'Sedentaria', light: 'Leggera', moderate: 'Moderata', active: 'Intensa', athlete: 'da atleta' };
    return labels[sel.value] || '';
  }

  // IBW (Devine-Hamwi, adattamento metrico):
  //   Uomo:  50.0 kg + 0.91 × (altezza_cm - 152.4)
  //   Donna: 45.5 kg + 0.91 × (altezza_cm - 152.4)
  function idealBodyWeight(heightCm, sex) {
    const base = (sex === 'F') ? 45.5 : 50.0;
    const extra = 0.91 * Math.max(0, heightCm - 152.4);
    return base + extra;
  }

  // Effective weight to use for the protein dose.
  // For BMI ≥ 30 (obesity) we use the adjusted body weight (AND formula):
  //   ABW = IBW + 0.25 × (current_weight - IBW)
  // so we avoid overestimating protein based on fat mass.
  function effectiveProteinWeight(weight, height, sex) {
    if (!height) return weight;
    const bmi = weight / Math.pow(height / 100, 2);
    if (bmi < 30 || !sex) return weight;
    const ibw = idealBodyWeight(height, sex);
    return ibw + 0.25 * (weight - ibw);
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
      calcWeight = null; calcHeight = null; calcSex = null;
      calcBMR = null; calcTDEE = null;
      setPresetButtonsEnabled(false);
      return;
    }

    const bmi  = weight / Math.pow(height / 100, 2);
    const base = 10 * weight + 6.25 * height - 5 * age;
    const bmr  = sex === 'M' ? base + 5 : base - 161;

    calcWeight = weight; calcHeight = height; calcSex = sex; calcBMR = bmr;

    const bmiV = el('bmi-value'); const bmiC = el('bmi-category'); const bmrV = el('bmr-value');
    if (bmiV) bmiV.textContent = fmt(bmi, 1);
    if (bmiC) bmiC.textContent = bmiCategory(bmi);
    if (bmrV) bmrV.textContent = fmt(bmr);

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
    if (tdeeV) tdeeV.textContent = fmt(calcTDEE);
    if (tdeeL) {
      const lbl = getActivityLabel();
      tdeeL.textContent = lbl && lbl !== 'default' ? 'con attività ' + lbl.toLowerCase() : 'attività non indicata';
    }
    updateMacroSummary();
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
    const effW = effectiveProteinWeight(calcWeight, calcHeight, calcSex);

    const kcal    = Math.round(tdee * s.kcalFactor);
    const protein = Math.round(effW * s.proteinGPerKg);
    const fat     = Math.round(kcal * s.fatPct / 9);
    let   carbs   = Math.round((kcal - protein * 4 - fat * 9) / 4);
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

    // Highlight the active button.
    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.classList.toggle('is-on', btn.dataset.preset === mode);
      btn.setAttribute('aria-pressed', String(btn.dataset.preset === mode));
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
      btn.classList.remove('is-on');
      btn.setAttribute('aria-pressed', 'false');
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
    if (suggestion.kcal    !== undefined) parts.push(fmt(suggestion.kcal) + ' kcal');
    if (suggestion.protein !== undefined) parts.push('proteine ' + suggestion.protein + ' g');
    if (suggestion.carbs   !== undefined) parts.push('carboidrati ' + suggestion.carbs + ' g');
    if (suggestion.fat     !== undefined) parts.push('grassi ' + suggestion.fat + ' g');
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
    if (r.kcal    !== undefined) suggested.kcal    = r.kcal;
    if (r.protein !== undefined) suggested.protein = r.protein;
    if (r.carbs   !== undefined) suggested.carbs   = r.carbs;
    if (r.fat     !== undefined) suggested.fat     = r.fat;
    ['f-tk', 'f-tp', 'f-tc', 'f-tf'].forEach(function (id) { setHint(id, null); });
    hideRebalancePanel();
    updateMacroSummary();
    validateAll();
  }

  function computeRebalance(changedKey) {
    const kcal    = parseF(el('f-tk') && el('f-tk').value);
    const protein = parseF(el('f-tp') && el('f-tp').value);
    const carbs   = parseF(el('f-tc') && el('f-tc').value);
    const fat     = parseF(el('f-tf') && el('f-tf').value);

    const s    = presetStrategy[activePreset] || presetStrategy.default;
    const effW = effectiveProteinWeight(calcWeight, calcHeight, calcSex);

    if (changedKey === 'kcal') {
      if (!isNaN(kcal) && kcal >= 800) {
        let newP, newF, newC;
        if (effW) {
          newP = Math.round(effW * s.proteinGPerKg);
        } else {
          newP = Math.round(kcal * 0.20 / 4);
        }
        newF = Math.round(kcal * s.fatPct / 9);
        newC = Math.round((kcal - newP * 4 - newF * 9) / 4);
        if (newC < 0) newC = 0;
        return { protein: newP, carbs: newC, fat: newF };
      }
      return null;
    }

    if (changedKey === 'protein') {
      if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && protein >= 0) {
        const newF = Math.round(kcal * s.fatPct / 9);
        const newC = Math.round((kcal - protein * 4 - newF * 9) / 4);
        if (newC >= 0) return { carbs: newC, fat: newF };
      }
      return null;
    }

    if (changedKey === 'fat') {
      if (!isNaN(kcal) && kcal > 0 && !isNaN(protein) && !isNaN(fat) && fat >= 0) {
        const newC = Math.round((kcal - protein * 4 - fat * 9) / 4);
        if (newC >= 0) return { carbs: newC };
      }
      return null;
    }

    if (changedKey === 'carbs') {
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

  // The kcal split: the bar, the percentage under every field and the
  // difference between the macros' kcal and the goal.
  function updateMacroSummary() {
    const split = el('macro-split'), summary = el('macro-summary');
    const kcal    = parseF(el('f-tk') && el('f-tk').value);
    const protein = parseF(el('f-tp') && el('f-tp').value);
    const carbs   = parseF(el('f-tc') && el('f-tc').value);
    const fat     = parseF(el('f-tf') && el('f-tf').value);
    function pct(id, text) { const n = el('pct-' + id); if (n) n.textContent = text || ''; }

    pct('f-tk', calcTDEE && kcal > 0 ? 'fabbisogno × ' + fmt(kcal / calcTDEE, 2) : '');
    const mk = (protein || 0) * 4 + (carbs || 0) * 4 + (fat || 0) * 9;
    const base = kcal > 0 ? kcal : mk;
    pct('f-tp', protein > 0 && base > 0 ? Math.round(protein * 4 / base * 100) + '%' + (calcWeight ? ' · ' + fmt(protein / calcWeight, 1) + ' g/kg' : '') : '');
    pct('f-tc', carbs > 0 && base > 0 ? Math.round(carbs * 4 / base * 100) + '%' : '');
    pct('f-tf', fat > 0 && base > 0 ? Math.round(fat * 9 / base * 100) + '%' : '');

    if (!split || !summary) return;
    if (mk <= 0) { split.classList.add('hidden'); return; }
    el('split-p').style.flexGrow = (protein || 0) * 4;
    el('split-c').style.flexGrow = (carbs || 0) * 4;
    el('split-f').style.flexGrow = (fat || 0) * 9;
    let text = 'I macro danno ' + fmt(mk) + ' kcal';
    if (kcal > 0) {
      const diff = Math.round(mk - kcal);
      text += Math.abs(diff) < 1 ? ", come l'obiettivo." : ': ' + fmt(Math.abs(diff)) + (diff > 0 ? ' in più' : ' in meno') + " dell'obiettivo.";
    } else {
      text += '.';
    }
    summary.textContent = text;
    split.classList.remove('hidden');
  }

  function onMacroInput(key, fieldId) {
    if (suggested[key] !== null) {
      const val = parseF(el(fieldId) && el(fieldId).value);
      if (!isNaN(val) && Math.round(val) !== suggested[key]) {
        setHint(fieldId, 'suggerito: ' + suggested[key]);
      } else {
        setHint(fieldId, null);
      }
    }
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
    ['f-weight', 'f-height', 'f-birth'].forEach(function (id) {
      const f = el(id); if (f) f.addEventListener('input', updateIndices);
    });
    document.querySelectorAll('input[name="sex"]').forEach(function (r) {
      r.addEventListener('change', updateIndices);
    });
    const actSel = el('f-activity');
    if (actSel) actSel.addEventListener('change', updateTDEE);

    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.preventDefault(); applyPreset(this.dataset.preset);
      });
    });

    const resetBtn = el('btn-reset-macros');
    if (resetBtn) resetBtn.addEventListener('click', function (e) { e.preventDefault(); resetMacros(); });

    [['kcal', 'f-tk'], ['protein', 'f-tp'], ['carbs', 'f-tc'], ['fat', 'f-tf']].forEach(function (p) {
      const f = el(p[1]);
      if (f) f.addEventListener('input', function () { onMacroInput(p[0], p[1]); });
    });

    const applyBtn   = el('rebalance-apply');
    const dismissBtn = el('rebalance-dismiss');
    if (applyBtn)   applyBtn.addEventListener('click',   function () { applyRebalance(); });
    if (dismissBtn) dismissBtn.addEventListener('click', function () { hideRebalancePanel(); });

    updateIndices();
    validateAll();
    updateMacroSummary();
  });
}());
