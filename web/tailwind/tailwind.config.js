/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/web/templates/**/*.html",
    "./internal/app/**/*.go",
  ],
  theme: {
    extend: {
      // Alias semantici mappati su CSS variables: usare bg-bg, bg-surface,
      // text-text, text-muted, ring-accent, ecc. nei template invece di
      // colori Tailwind hardcoded. Cambia palette = cambia data-theme.
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
      },
      borderColor: {
        DEFAULT: "var(--border)",
      },
    },
  },
  plugins: [],
};
