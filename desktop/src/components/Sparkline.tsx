// A canvas sparkline (uPlot). uPlot is ~40KB and built for streaming series;
// it redraws only the canvas, so it survives N hosts × 1 Hz where an SVG chart
// would die (CLAUDE.md §6).

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import uPlot from 'uplot'

interface SparklineProps {
  values: number[]
  /** Line colour. */
  stroke: string
  /** Optional fill under the line. */
  fill?: string
  min?: number
  /** Fixed max, or 'auto' to scale to the data. */
  max?: number | 'auto'
  height?: number
}

function toData(values: number[]): uPlot.AlignedData {
  const xs = values.map((_, i) => i)
  return [xs, values]
}

export function Sparkline({ values, stroke, fill, min = 0, max = 1, height = 26 }: SparklineProps) {
  const wrap = useRef<HTMLDivElement>(null)
  const plot = useRef<uPlot | null>(null)
  const [width, setWidth] = useState(120)

  useLayoutEffect(() => {
    const el = wrap.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const w = Math.floor(entries[0]!.contentRect.width)
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
      cursor: { show: false },
      legend: { show: false },
      scales: { x: { time: false }, y: max === 'auto' ? {} : { range: [min, max] } },
      axes: [{ show: false }, { show: false }],
      series: [
        {},
        { stroke, width: 1.5, fill, points: { show: false } },
      ],
      padding: [3, 0, 1, 0],
    }
    const u = new uPlot(opts, toData(values), el)
    plot.current = u
    return () => {
      u.destroy()
      plot.current = null
    }
    // Recreate only when geometry/appearance changes; data updates below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [width, height, stroke, fill, min, max])

  useEffect(() => {
    plot.current?.setData(toData(values))
  }, [values])

  return <div ref={wrap} style={{ height }} />
}
