/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: [
    './pages/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
    './app/**/*.{ts,tsx}',
    './src/**/*.{ts,tsx}',
  ],
  theme: {
    container: {
      center: true,
      padding: "2rem",
      screens: {
        "2xl": "1400px",
      },
    },
    extend: {
      colors: {
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))",
        },
        destructive: {
          DEFAULT: "hsl(var(--destructive))",
          foreground: "hsl(var(--destructive-foreground))",
        },
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))",
        },
        accent: {
          DEFAULT: "hsl(var(--accent))",
          foreground: "hsl(var(--accent-foreground))",
        },
        popover: {
          DEFAULT: "hsl(var(--popover))",
          foreground: "hsl(var(--popover-foreground))",
        },
        card: {
          DEFAULT: "hsl(var(--card))",
          foreground: "hsl(var(--card-foreground))",
        },
        // Custom Nutrition Colors
        nutrition: {
          protein: "hsl(var(--nutrition-protein))",
          carbs: "hsl(var(--nutrition-carbs))",
          fat: "hsl(var(--nutrition-fat))",
          fiber: "hsl(var(--nutrition-fiber))",
          calories: "hsl(var(--nutrition-calories))",
        },
        meal: {
          breakfast: "hsl(var(--meal-breakfast))",
          snack: "hsl(var(--meal-snack))",
          lunch: "hsl(var(--meal-lunch))",
          dinner: "hsl(var(--meal-dinner))",
        },
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
      keyframes: {
        "accordion-down": {
          from: { height: 0 },
          to: { height: "var(--radix-accordion-content-height)" },
        },
        "accordion-up": {
          from: { height: "var(--radix-accordion-content-height)" },
          to: { height: 0 },
        },
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
      },
    },
  },
  plugins: [require("tailwindcss-animate")],
  safelist: [
    // Gradient utilities
    'gradient-sidebar',
    'gradient-breakfast',
    'gradient-snack',
    'gradient-lunch',
    'gradient-dinner',
    'gradient-nutrition',
    'gradient-meal',

    // Animation utilities
    'btn-hover',
    'card-hover',
    'hover-lift',
    'hover-glow',

    // Typography utilities
    'text-display',
    'text-heading-1',
    'text-heading-2',
    'text-heading-3',
    'text-body-large',
    'text-body',
    'text-caption',

    // Component utilities
    'card-nutrition',
    'card-meal',
    'btn-nutrition',
    'btn-nutrition-secondary',
    'btn-nutrition-outline',

    // Animation classes
    'animate-fade-in',
    'animate-slide-up',
    'animate-scale-in',
    'animate-bounce-in',

    // Glass morphism
    'glass',
    'glass-dark',

    // Nutrition colors
    'nutrition-protein',
    'nutrition-carbs',
    'nutrition-fat',
    'nutrition-fiber',
    'nutrition-calories',

    // Meal backgrounds
    'meal-breakfast',
    'meal-snack',
    'meal-lunch',
    'meal-dinner'
  ]
}