/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/web/templates/**/*.html",
    "./internal/app/**/*.go",
    // Solo i nostri script (non i vendor .min.js): alcuni creano markup con classi.
    "./internal/web/static/js/{confirm,modal,ingredient_search,theme,day_tabs,dnd,tour}.js",
  ],
  theme: {
    extend: {
      // Alias semantici mappati su CSS variables: usare bg-bg, bg-surface,
      // text-text, text-muted, ring-accent, ecc. nei template invece di
      // colori Tailwind hardcoded. Modalita' e colore = data-mode / data-accent su <html>.
      colors: {
        bg: "var(--bg)",
        surface: "var(--surface)",
        "surface-2": "var(--surface-2)",
        border: "var(--border)",
        text: "var(--text)",
        muted: "var(--muted)",
        accent: "var(--accent)",
        "accent-fg": "var(--accent-fg)",
        "accent-soft": "var(--accent-soft)",
        "warn-bg": "var(--warn-bg)",
        "warn-fg": "var(--warn-fg)",
        danger: "var(--danger)",
        "danger-soft": "var(--danger-soft)",
        track: "var(--track)",
        // Codice colore dei macro: proteine, carboidrati, grassi.
        prot: "var(--prot)",
        carb: "var(--carb)",
        fat: "var(--fat)",
      },
      fontFamily: {
        sans: ["Figtree", "ui-sans-serif", "system-ui", "sans-serif"],
      },
      borderColor: {
        DEFAULT: "var(--border)",
      },
    },
  },
  plugins: [],
};
