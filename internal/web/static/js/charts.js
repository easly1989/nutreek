// Statistics and Weight charts (#123), in SVG: 2px lines, bars with a
// rounded end, a recessive grid, a direct value only on the last point,
// tooltips on hover. The colors are the CSS tokens, so they follow
// light/dark and the chosen color without redrawing.
//
// Data: <script type="application/json" id="chart-data">; every chart is
// an element with data-chart="<name>" (see DRAW at the bottom).
(function () {
  "use strict";
  var data;
  try {
    data = JSON.parse(document.getElementById("chart-data").textContent);
  } catch (e) {
    return;
  }

  // The page language picks the separators (1.894 / 1,894). useGrouping
  // "always": in Italian 2280 would stay without a dot.
  var LANG = document.documentElement.lang || "it";
  function fmt(v, d) {
    return v.toLocaleString(LANG, { minimumFractionDigits: 0, maximumFractionDigits: d || 0, useGrouping: "always" });
  }
  function esc(s) {
    return String(s).replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; });
  }

  // «Round» ticks covering [min, max].
  // dec = decimal digits of the labels: the step does not go below their
  // precision, so two ticks never show the same number.
  function ticks(min, max, count, dec) {
    if (max <= min) max = min + 1;
    var raw = (max - min) / (count || 4), p = Math.pow(10, Math.floor(Math.log10(raw)));
    var step = [1, 2, 2.5, 5, 10].map(function (m) { return m * p; }).find(function (s) { return s >= raw; });
    step = Math.max(step, Math.pow(10, -(dec || 0)));
    if (!dec && step % 1) step = Math.ceil(step);
    var lo = Math.floor(min / step) * step, hi = Math.ceil(max / step) * step, out = [];
    for (var t = lo; t <= hi + step / 2; t += step) out.push(Math.round(t * 1000) / 1000);
    return out;
  }
  function present(vs) { return vs.filter(function (v) { return v != null; }); }

  var PAD = { l: 44, r: 52, t: 12, b: 26 };

  // Base comune: griglia, etichette, area sensibile. o = {el, labels, ticks,
  // unit, dec, h, target, targetLabel, x(i) opzionale}.
  function frame(o) {
    var w = Math.max(o.el.clientWidth, 260), h = o.h, n = o.labels.length;
    var lo = o.ticks[0], hi = o.ticks[o.ticks.length - 1];
    var y = function (v) { return PAD.t + (h - PAD.t - PAD.b) * (1 - (v - lo) / (hi - lo)); };
    var slot = (w - PAD.l - PAD.r) / n;
    var x = o.bars ? function (i) { return PAD.l + slot * (i + 0.5); }
      : function (i) { return PAD.l + (w - PAD.l - PAD.r) * (n === 1 ? 0.5 : i / (n - 1)); };
    var every = Math.max(1, Math.ceil(n / Math.floor((w - PAD.l - PAD.r) / 56)));
    var s = "";
    o.ticks.forEach(function (t) {
      var yy = y(t);
      s += '<line x1="' + PAD.l + '" x2="' + (w - PAD.r) + '" y1="' + yy + '" y2="' + yy + '" class="ch-grid"/>';
      s += '<text x="' + (PAD.l - 8) + '" y="' + (yy + 4) + '" text-anchor="end">' + fmt(t, o.dec) + (o.unit || "") + "</text>";
    });
    o.labels.forEach(function (lab, i) {
      if ((n - 1 - i) % every === 0) s += '<text x="' + x(i) + '" y="' + (h - 6) + '" text-anchor="middle">' + esc(lab) + "</text>";
    });
    return { w: w, h: h, n: n, x: x, y: y, slot: slot, svg: s };
  }
  function targetLine(f, o) {
    if (!o.target) return "";
    var ty = f.y(o.target);
    return '<line x1="' + PAD.l + '" x2="' + (f.w - PAD.r) + '" y1="' + ty + '" y2="' + ty + '" class="ch-target"/>' +
      '<text x="' + (f.w - PAD.r + 6) + '" y="' + (ty + 4) + '">' + esc(o.targetLabel) + "</text>";
  }

  // Tooltip: the column nearest the pointer; tip(i) gives the HTML.
  function hover(el, f, tip, guide) {
    var box = document.createElement("div");
    box.className = "ch-tip";
    box.hidden = true;
    el.appendChild(box);
    var svg = el.querySelector("svg"), mark = svg.querySelector(".ch-hover");
    function show(ev) {
      var r = svg.getBoundingClientRect(), px = (ev.clientX - r.left) * f.w / r.width;
      var best = 0;
      for (var i = 1; i < f.n; i++) if (Math.abs(f.x(i) - px) < Math.abs(f.x(best) - px)) best = i;
      var html = tip(best);
      if (!html) { hide(); return; }
      box.innerHTML = html;
      box.hidden = false;
      var left = f.x(best) * r.width / f.w;
      box.style.left = Math.min(Math.max(left - box.offsetWidth / 2, 0), el.clientWidth - box.offsetWidth) + "px";
      if (mark) {
        mark.setAttribute("x1", f.x(best));
        mark.setAttribute("x2", f.x(best));
        mark.style.display = guide ? "" : "none";
      }
      svg.querySelectorAll("[data-i]").forEach(function (b) { b.classList.toggle("is-dim", +b.dataset.i !== best); });
    }
    function hide() {
      box.hidden = true;
      if (mark) mark.style.display = "none";
      svg.querySelectorAll(".is-dim").forEach(function (b) { b.classList.remove("is-dim"); });
    }
    svg.addEventListener("pointermove", show);
    svg.addEventListener("pointerdown", show);
    svg.addEventListener("pointerleave", hide);
  }
  function tipRow(color, label, value) {
    return '<span class="ch-tip-row"><i style="background:' + color + '"></i>' + esc(label) + "<b>" + value + "</b></span>";
  }

  // o = {el, labels, values, h, color, unit, dec, area, target, targetLabel,
  //      lo, hi (minimum limits of the scale), name, tipUnit}
  function lineChart(o) {
    var vs = present(o.values);
    if (vs.length === 0) return;
    var lo = Math.min.apply(null, vs.concat(o.target ? [o.target] : [])), hi = Math.max.apply(null, vs.concat(o.target ? [o.target] : []));
    if (o.lo != null) lo = Math.min(lo, o.lo);
    if (o.hi != null) hi = Math.max(hi, o.hi);
    var pad = (hi - lo) * 0.08 || 1;
    o.ticks = ticks(o.lo != null ? lo : lo - pad, o.hi != null ? hi : hi + pad, 3, o.dec);
    var f = frame(o), s = f.svg + targetLine(f, o), pts = [], last = null;
    o.values.forEach(function (v, i) { if (v != null) { pts.push(f.x(i) + "," + f.y(v)); last = [f.x(i), f.y(v), v]; } });
    if (o.area && pts.length > 1) {
      var base = f.y(o.ticks[0]);
      s += '<polygon points="' + pts[0].split(",")[0] + "," + base + " " + pts.join(" ") + " " + last[0] + "," + base + '" style="fill:' + o.color + '" opacity=".1"/>';
    }
    s += '<polyline points="' + pts.join(" ") + '" fill="none" style="stroke:' + o.color + '" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>';
    if (pts.length === 1) s += '<circle cx="' + last[0] + '" cy="' + last[1] + '" r="4" style="fill:' + o.color + '"/>';
    s += '<circle cx="' + last[0] + '" cy="' + last[1] + '" r="4.5" style="fill:' + o.color + ';stroke:var(--surface)" stroke-width="2"/>';
    // Final value: when it falls on the goal's label, it moves above or below.
    var ly = last[1] + 4;
    if (o.target && Math.abs(f.y(o.target) - last[1]) < 14) ly = last[1] < f.y(o.target) ? f.y(o.target) - 12 : f.y(o.target) + 18;
    s += '<text x="' + (last[0] + 9) + '" y="' + ly + '" class="ch-value">' + fmt(last[2], o.dec) + "</text>";
    s += '<line class="ch-hover" y1="' + PAD.t + '" y2="' + (f.h - PAD.b) + '" style="display:none"/>';
    o.el.innerHTML = '<svg class="chart" viewBox="0 0 ' + f.w + " " + f.h + '" role="img" aria-label="' + esc(o.name) + '">' + s + "</svg>";
    hover(o.el, f, function (i) {
      var v = o.values[i];
      if (v == null) return "";
      return "<b>" + esc(o.labels[i]) + "</b>" + tipRow(o.color, o.name, fmt(v, o.dec) + (o.tipUnit || ""));
    }, true);
  }

  // o = {el, labels, series:[{values, color, name}], h, unit, dec, target,
  //      targetLabel, tipUnit, total}
  // More series = stacked bars, with a 2px gap between the segments.
  function barChart(o) {
    var n = o.labels.length, totals = [];
    for (var i = 0; i < n; i++) totals.push(o.series.reduce(function (s, ser) { return s + (ser.values[i] || 0); }, 0));
    var hi = Math.max.apply(null, totals.concat(o.target ? [o.target] : []));
    if (hi <= 0) return;
    o.bars = true;
    o.ticks = ticks(0, hi * 1.05, 3, o.dec);
    var f = frame(o), s = f.svg, bw = Math.min(28, f.slot * 0.6), k = o.series.length;
    for (i = 0; i < n; i++) {
      var base = 0, bar = "";
      o.series.forEach(function (ser, si) {
        var v = ser.values[i];
        if (!v) return;
        var y0 = f.y(base), y1 = f.y(base + v), gap = base > 0 ? 2 : 0, bx = f.x(i) - bw / 2;
        var top = si === k - 1 || o.series.slice(si + 1).every(function (x) { return !x.values[i]; });
        var rad = top ? Math.min(4, (y0 - gap - y1) / 2, bw / 2) : 0;
        bar += '<path d="M' + bx + "," + (y0 - gap) + "V" + (y1 + rad) + "Q" + bx + "," + y1 + " " + (bx + rad) + "," + y1 +
          "H" + (bx + bw - rad) + "Q" + (bx + bw) + "," + y1 + " " + (bx + bw) + "," + (y1 + rad) + "V" + (y0 - gap) + 'Z" style="fill:' + ser.color + '"/>';
        base += v;
      });
      if (bar) s += '<g data-i="' + i + '">' + bar + "</g>";
    }
    s += targetLine(f, o);
    o.el.innerHTML = '<svg class="chart" viewBox="0 0 ' + f.w + " " + f.h + '" role="img" aria-label="' + esc(o.name) + '">' + s + "</svg>";
    hover(o.el, f, function (i) {
      if (!totals[i]) return "<b>" + esc(o.labels[i]) + "</b><span class=\"ch-tip-row\">nessun dato</span>";
      var html = "<b>" + esc(o.labels[i]) + "</b>";
      if (k > 1) {
        o.series.forEach(function (ser) { html += tipRow(ser.color, ser.name, fmt(ser.values[i] || 0, o.dec) + (o.tipUnit || "")); });
        html += '<span class="ch-tip-row ch-tip-total">' + esc(o.total) + "<b>" + fmt(totals[i], o.dec) + (o.tipUnit || "") + "</b></span>";
      } else {
        html += tipRow(o.series[0].color, o.series[0].name, fmt(totals[i], o.dec) + (o.tipUnit || ""));
      }
      return html;
    }, false);
  }

  var ACC = "var(--accent)";
  var DRAW = {
    tdee: function (el) {
      lineChart({ el: el, h: 220, labels: data.labels, values: data.tdee, color: ACC, name: "Fabbisogno stimato", tipUnit: " kcal",
        target: data.target, targetLabel: fmt(data.target) });
    },
    macro: function (el) {
      barChart({ el: el, h: 220, labels: data.labels, name: "Calorie e macro al giorno", tipUnit: " kcal", total: "Totale",
        target: data.target, targetLabel: fmt(data.target), series: [
          { values: data.prot, color: "var(--ch-prot)", name: "Proteine" },
          { values: data.carb, color: "var(--ch-carb)", name: "Carboidrati" },
          { values: data.fat, color: "var(--ch-fat)", name: "Grassi" }] });
    },
    weight: function (el) {
      lineChart({ el: el, h: 220, labels: data.weightLabels, values: data.weight, color: ACC, name: "Peso", dec: 1, tipUnit: " kg", area: true });
    },
    adherence: function (el) {
      lineChart({ el: el, h: 190, labels: data.labels, values: data.adherence, color: ACC, name: "Aderenza", unit: "%", tipUnit: "%", lo: 60, hi: 100 });
    },
    workout: function (el) {
      barChart({ el: el, h: 160, labels: data.labels, name: "Allenamenti", tipUnit: " kcal", series: [{ values: data.workout, color: ACC, name: "Bruciate" }] });
    },
    cheats: function (el) {
      barChart({ el: el, h: 160, labels: data.labels, name: "Sgarri", series: [{ values: data.cheats, color: ACC, name: "Pasti" }] });
    },
    water: function (el) {
      barChart({ el: el, h: 160, labels: data.labels, name: "Acqua", dec: 1, tipUnit: " l", series: [{ values: data.water, color: ACC, name: "Al giorno" }],
        target: data.waterTarget, targetLabel: fmt(data.waterTarget, 1) + " l" });
    }
  };

  function drawAll() {
    document.querySelectorAll("[data-chart]").forEach(function (el) {
      var draw = DRAW[el.dataset.chart];
      if (draw) draw(el);
    });
  }
  drawAll();
  // Labels stay at 12px: redraw at the new width.
  var last = window.innerWidth, timer;
  window.addEventListener("resize", function () {
    if (window.innerWidth === last) return;
    last = window.innerWidth;
    clearTimeout(timer);
    timer = setTimeout(drawAll, 150);
  });
})();
