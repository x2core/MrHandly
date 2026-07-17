/**
 * Tokens are CSS variables so themes swap live (see src/index.css). Colours are
 * RGB channel triplets — `rgb(var(--c) / <alpha-value>)` keeps Tailwind's
 * `/opacity` modifiers working. Radius and border weight are variables too, so
 * a theme's *vibe* (soft corporate vs. boxy industrial) rides along with its
 * palette. Colour carries state and nothing else (CLAUDE.md §7).
 */
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    borderRadius: {
      none: '0',
      sm: 'calc(var(--radius) / 2)',
      DEFAULT: 'var(--radius)',
      md: 'var(--radius)',
      lg: 'calc(var(--radius) * 1.5)',
      full: '9999px',
    },
    borderWidth: {
      DEFAULT: 'var(--border)',
      0: '0',
      2: '2px',
    },
    extend: {
      colors: {
        panel: 'rgb(var(--c-panel) / <alpha-value>)',
        rack: 'rgb(var(--c-rack) / <alpha-value>)',
        rule: 'rgb(var(--c-rule) / <alpha-value>)',
        plate: 'rgb(var(--c-plate) / <alpha-value>)',
        readout: 'rgb(var(--c-readout) / <alpha-value>)',
        signal: 'rgb(var(--c-signal) / <alpha-value>)',
        fault: 'rgb(var(--c-fault) / <alpha-value>)',
      },
      fontFamily: {
        plate: ['"IBM Plex Sans Condensed"', 'sans-serif'],
        sans: ['"IBM Plex Sans"', 'system-ui', 'sans-serif'],
        mono: ['"IBM Plex Mono"', 'ui-monospace', 'monospace'],
      },
      fontSize: {
        plate: ['10px', { lineHeight: '12px', letterSpacing: '0.12em' }],
      },
      spacing: {
        strip: 'var(--strip)',
        row: '30px',
        sidebar: '15rem',
      },
      transitionDuration: {
        led: '120ms',
      },
      boxShadow: {
        card: 'var(--shadow)',
      },
    },
  },
  plugins: [],
}
