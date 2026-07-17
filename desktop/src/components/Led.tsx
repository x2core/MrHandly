// A status LED. Colour carries state and nothing else (CLAUDE.md §7): signal
// (amber) for a healthy/running indicator, fault (red) for failure, otherwise a
// dark unlit dot. The only motion in the app is this 120ms crossfade.

interface LedProps {
  on: boolean
  kind?: 'signal' | 'fault'
  label: string
}

export function Led({ on, kind = 'signal', label }: LedProps) {
  const lit = on ? (kind === 'fault' ? 'bg-fault' : 'bg-signal') : 'bg-rule'
  const glow = on
    ? kind === 'fault'
      ? 'shadow-[var(--glow)_rgb(var(--c-fault))]'
      : 'shadow-[var(--glow)_rgb(var(--c-signal))]'
    : ''
  return (
    <span className="flex items-center gap-1" title={label}>
      <span className={`h-[7px] w-[7px] rounded-full transition-colors duration-led ${lit} ${glow}`} />
      <span className="plate text-[9px]">{label}</span>
    </span>
  )
}
