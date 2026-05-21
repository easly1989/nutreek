(function () {
  'use strict';

  // Calcolati live dai dati fisici, usati dai preset.
  let calcWeight = null;
  let calcBMR    = null;

  // Valori suggeriti dall'ultimo preset applicato (null = nessun preset).
  const suggested = { kcal: null, protein: null, carbs: null, fat: null };

  // ---- Helpers ----------------------------------------------------------------

  function parseF(s) {
    return parseFloat(String(s || '').replace(',', '.'));
  }

  function getSex() {
    const el = document.querySelector('input[name="sex"]:checked');
    return el ? el.value : '';
  }

  function bmiCategory(bmi) {
    if (bmi < 18.5) return 'Sottopeso';
    if (bmi < 25.0) return 'Normopeso';
    if (bmi < 30.0) return 'Sovrappeso';
    return 'Obesità';
  }

  function el(id) { return document.getElementById(id); }

  // ---- BMI / BMR live ---------------------------------------------------------

  function updateIndices() {
    const weight    = parseF(el('f-weight') && el('f-weight').value);
    const height    = parseF(el('f-height') && el('f-height').value);
    const birthYear = parseInt((el('f-birth') && el('f-birth').value) || '', 10);
    const sex       = getSex();
    const year      = new Date().getFullYear();
    const age       = year - birthYear;

    const dyn         = el('indices-dynamic');
    const placeholder = el('indices-placeholder');

    const valid = weight >= 20 && weight <= 300 &&
                  height >= 100 && height <= 250 &&
                  !isNaN(birthYear) && age >= 5 && age <= 130 &&
                  (sex === 'M' || sex === 'F');

    if (!valid) {
      if (dyn)         dyn.classList.add('hidden');
      if (placeholder) placeholder.classList.remove('hidden');
      calcWeight = null;
      calcBMR    = null;
      setPresetButtonsEnabled(false);
      return;
    }

    const bmi  = weight / Math.pow(height / 100, 2);
    const base = 10 * weight + 6.25 * height - 5 * age;
    const bmr  = sex === 'M' ? base + 5 : base - 161;

    calcWeight = weight;
    calcBMR    = bmr;

    const bmiVal  = el('bmi-value');
    const bmiCat  = el('bmi-category');
    const bmrVal  = el('bmr-value');

    if (bmiVal)  bmiVal.textContent  = bmi.toFixed(1);
    if (bmiCat)  bmiCat.textContent  = bmiCategory(bmi);
    if (bmrVal)  bmrVal.textContent  = Math.round(bmr);

    if (dyn)         dyn.classList.remove('hidden');
    if (placeholder) placeholder.classList.add('hidden');
    setPresetButtonsEnabled(true);
  }

  // ---- Preset buttons ---------------------------------------------------------

  function setPresetButtonsEnabled(enabled) {
    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.disabled = !enabled;
    });
    const hint = el('presets-hint');
    if (hint) hint.classList.toggle('hidden', enabled);
  }

  function calcPreset(mode) {
    const tdee    = calcBMR * 1.4;
    let kcal, protein, fat, carbs;

    if (mode === 'bulk') {
      kcal    = Math.round(tdee * 1.10);
      protein = Math.round(calcWeight * 2.0);
      fat     = Math.round(kcal * 0.25 / 9);
    } else if (mode === 'cut') {
      kcal    = Math.round(tdee * 0.80);
      protein = Math.round(calcWeight * 2.2);
      fat     = Math.round(kcal * 0.28 / 9);
    } else {
      // maintenance
      kcal    = Math.round(tdee);
      protein = Math.round(calcWeight * 1.8);
      fat     = Math.round(kcal * 0.25 / 9);
    }
    carbs = Math.round((kcal - protein * 4 - fat * 9) / 4);
    if (carbs < 0) carbs = 0;
    return { kcal: kcal, protein: protein, carbs: carbs, fat: fat };
  }

  function applyPreset(mode) {
    if (!calcWeight || !calcBMR) return;
    const v = calcPreset(mode);

    suggested.kcal    = v.kcal;
    suggested.protein = v.protein;
    suggested.carbs   = v.carbs;
    suggested.fat     = v.fat;

    setFieldValue('f-tk', v.kcal);
    setFieldValue('f-tp', v.protein);
    setFieldValue('f-tc', v.carbs);
    setFieldValue('f-tf', v.fat);

    // Clear all hints and errors, then validate.
    ['f-tk', 'f-tp', 'f-tc', 'f-tf'].forEach(function (id) {
      setHint(id, null);
      setError(id, null);
    });
    validateAll();
  }

  function setFieldValue(id, val) {
    const f = el(id);
    if (f) f.value = val;
  }

  // ---- Hints and errors -------------------------------------------------------

  function setHint(fieldId, text) {
    const node = el('hint-' + fieldId);
    if (!node) return;
    node.textContent = text || '';
    node.classList.toggle('hidden', !text);
  }

  function setError(fieldId, text) {
    const node = el('err-' + fieldId);
    if (!node) return;
    node.textContent = text || '';
    node.classList.toggle('hidden', !text);
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
    validateAll();
  }

  // ---- Validation -------------------------------------------------------------

  function validateAll() {
    const fKcal    = el('f-tk');
    const fProtein = el('f-tp');
    const fCarbs   = el('f-tc');
    const fFat     = el('f-tf');

    const kcal    = fKcal    ? parseF(fKcal.value)    : NaN;
    const protein = fProtein ? parseF(fProtein.value) : NaN;
    const carbs   = fCarbs   ? parseF(fCarbs.value)   : NaN;
    const fat     = fFat     ? parseF(fFat.value)     : NaN;

    setError('f-tk', null);
    setError('f-tp', null);
    setError('f-tc', null);
    setError('f-tf', null);

    let hasError = false;

    function err(fieldId, msg) {
      setError(fieldId, msg);
      hasError = true;
    }

    // Limiti singoli
    if (!isNaN(kcal) && kcal > 0 && kcal < 800) {
      err('f-tk', 'Valore troppo basso (minimo consigliato: 800 kcal).');
    }
    if (!isNaN(protein) && protein > 400) {
      err('f-tp', 'Valore non realistico (massimo 400 g).');
    }
    if (!isNaN(fat) && fat > 0 && fat < 20) {
      err('f-tf', 'Valore troppo basso (minimo: 20 g per i grassi essenziali).');
    }

    // Combinazioni impossibili
    if (!isNaN(kcal) && kcal > 0) {
      if (!isNaN(protein) && protein > 0 && protein * 4 > kcal) {
        err('f-tp', 'Le proteine da sole superano le calorie totali (non possibile).');
      }
      if (!isNaN(fat) && fat > 0 && fat * 9 > kcal) {
        err('f-tf', 'I grassi da soli superano le calorie totali (non possibile).');
      }
      if (!isNaN(protein) && !isNaN(carbs) && !isNaN(fat) &&
          protein >= 0 && carbs >= 0 && fat >= 0) {
        const macroKcal = protein * 4 + carbs * 4 + fat * 9;
        if (macroKcal > kcal * 1.10) {
          err('f-tc', 'La somma dei macro supera le calorie totali di oltre il 10%.');
        }
      }
    }

    const saveBtn = el('profile-save-btn');
    if (saveBtn) saveBtn.disabled = hasError;
  }

  // ---- Init -------------------------------------------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    ['f-weight', 'f-height', 'f-birth'].forEach(function (id) {
      const f = el(id);
      if (f) f.addEventListener('input', updateIndices);
    });
    document.querySelectorAll('input[name="sex"]').forEach(function (radio) {
      radio.addEventListener('change', updateIndices);
    });

    document.querySelectorAll('.preset-btn').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.preventDefault();
        applyPreset(this.dataset.preset);
      });
    });

    [
      ['kcal',    'f-tk'],
      ['protein', 'f-tp'],
      ['carbs',   'f-tc'],
      ['fat',     'f-tf'],
    ].forEach(function (pair) {
      const f = el(pair[1]);
      if (f) f.addEventListener('input', function () { onMacroInput(pair[0], pair[1]); });
    });

    updateIndices();
    validateAll();
  });
}());
