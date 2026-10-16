/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/web/templates/**/*.html",
    "./internal/app/**/*.go",
    // Only our scripts (not the vendored .min.js): some create markup with classes.
    "./internal/web/static/js/{confirm,modal,ingredient_search,theme,day_tabs,dnd,tour,picker}.js",
  ],
  theme: {
    extend: {
      // Semantic aliases mapped onto CSS variables: use bg-bg, bg-surface,
      // text-text, text-muted, ring-accent, etc. in the templates instead of
      // hardcoded Tailwind colors. Mode and color = data-mode / data-accent on <html>.
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
        // Macro color code: protein, carbs, fat.
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
