// Resolve theme CSS variables into concrete colour strings for the canvas
// charts (uPlot needs real colours, not CSS vars). Recomputes when the theme
// changes so meters repaint in the active palette.

import { useMemo } from 'react'
import { useTheme } from './theme'

export interface ChartColors {
  signal: string
  signalFill: string
  grid: string
  axis: string
}

export function useThemeColors(): ChartColors {
  const id = useTheme((s) => s.id)
  return useMemo<ChartColors>(() => {
    const g = getComputedStyle(document.documentElement)
    const ch = (v: string) => g.getPropertyValue(v).trim()
    return {
      signal: `rgb(${ch('--c-signal')})`,
      signalFill: `rgb(${ch('--c-signal')} / 0.16)`,
      grid: `rgb(${ch('--c-rule')} / 0.55)`,
      axis: `rgb(${ch('--c-plate')})`,
    }
    // id drives the recompute even though it is not read directly.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])
}
