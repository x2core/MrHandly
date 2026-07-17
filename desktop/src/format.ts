// Number formatting for the readouts. Everything is tabular mono, so widths are
// stable; these just pick sensible units.

export function pct(v: number): string {
  return `${Math.round(v * 100)}%`
}

const UNITS = ['B', 'K', 'M', 'G', 'T', 'P']

export function bytes(v: number): string {
  let n = v
  let i = 0
  while (n >= 1024 && i < UNITS.length - 1) {
    n /= 1024
    i++
  }
  return `${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)}${UNITS[i]}`
}

export function rate(v: number): string {
  return `${bytes(v)}/s`
}

export function uptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function sinceLabel(ts: number | null): string {
  if (!ts) return 'never'
  const s = Math.floor((Date.now() - ts) / 1000)
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  return `${h}h ago`
}
