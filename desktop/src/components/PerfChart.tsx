// A larger uPlot chart for the focus view's Performance panel — same data
// source as the strip sparklines, just more of it and with a baseline grid.

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import uPlot from 'uplot'
import { useThemeColors } from '../theme-colors'

interface Props {
  values: number[]
  stroke: string
  fill?: string
  max?: number | 'auto'
  height?: number
  /** Optional y-axis tick formatter (e.g. bytes for network). */
  yFormat?: (v: number) => string
}

export function PerfChart({ values, stroke, fill, max = 1, height = 130, yFormat }: Props) {
  const wrap = useRef<HTMLDivElement>(null)
  const plot = useRef<uPlot | null>(null)
  const [width, setWidth] = useState(320)
  const theme = useThemeColors()

  useLayoutEffect(() => {
    const el = wrap.current
    if (!el) return
    const ro = new ResizeObserver((e) => {
      const w = Math.floor(e[0]!.contentRect.width)
      if (w > 0) setWidth(w)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const opts: uPlot.Options = {
      width,
      height,
      cursor: { show: true, x: false, y: false },
      legend: { show: false },
      scales: { x: { time: false }, y: max === 'auto' ? {} : { range: [0, max] } },
      axes: [
        { show: false },
        {
          stroke: theme.axis,
          grid: { stroke: theme.grid, width: 1 },
          ticks: { show: false },
          size: 40,
          font: '10px "IBM Plex Mono"',
          values: yFormat ? (_u, splits) => splits.map((v) => yFormat(v)) : undefined,
        },
      ],
      series: [{}, { stroke, width: 1.5, fill, points: { show: false } }],
      padding: [6, 6, 2, 0],
    }
    const u = new uPlot(opts, [values.map((_, i) => i), values], el)
    plot.current = u
    return () => {
      u.destroy()
      plot.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [width, height, stroke, fill, max, yFormat, theme.axis, theme.grid])

  useEffect(() => {
    plot.current?.setData([values.map((_, i) => i), values])
  }, [values])

  return <div ref={wrap} style={{ height }} />
}
