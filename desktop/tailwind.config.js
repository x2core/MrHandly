/**
 * Design tokens first (CLAUDE.md §7). The register is rack-mounted instrument:
 * warm graphite, silkscreen label plates, meters and status LEDs on a grid.
 * Colour carries state and nothing else.
 */
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    // A tight, instrument-like scale. No decorative radii, hairline rules only.
    borderRadius: {
      none: '0',
      DEFAULT: '2px',
      sm: '1px',
    },
    extend: {
      colors: {
        panel: '#1B1A18', // app background
        rack: '#232220', // strip / row surface
        rule: '#35332F', // 1px hairlines — the only separator device
        plate: '#8A8578', // silkscreen label text
        readout: '#EDEAE3', // primary text and numbers
        signal: '#E8A33D', // meter fill and running LED ONLY
        fault: '#D2453C', // failed / offline ONLY
      },
      fontFamily: {
        // Label plates: condensed, uppercase, tracked — a real silkscreen look.
        plate: ['"IBM Plex Sans Condensed"', 'sans-serif'],
        sans: ['"IBM Plex Sans"', 'system-ui', 'sans-serif'],
        // All numbers/PIDs/unit names: tabular mono, non-negotiable at 1 Hz.
        mono: ['"IBM Plex Mono"', 'ui-monospace', 'monospace'],
      },
      fontSize: {
        plate: ['10px', { lineHeight: '12px', letterSpacing: '0.12em' }],
      },
      spacing: {
        strip: '56px', // Fleet Strip row height
        row: '30px', // dense table rows (28–32px)
      },
      transitionDuration: {
        led: '120ms', // LED state crossfade; motion is otherwise essentially none
      },
    },
  },
  plugins: [],
}
