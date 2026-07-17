// One Fleet Strip row: a fixed-height horizontal strip with a silkscreen name
// plate on the left, live meters in the middle, and an LED cluster on the
// right — a channel on a mixing desk of machines (CLAUDE.md §7). Clicking it
// focuses the host below. An offline peer degrades to a labelled disconnected
// plate, never a blank panel or an endless spinner.

import type { HostState } from '../store'
import { useThemeColors } from '../theme-colors'
import { bytes, pct, rate, sinceLabel } from '../format'
import { Sparkline } from './Sparkline'
import { Led } from './Led'

interface Props {
  host: HostState
  selected: boolean
  onSelect: () => void
}

function Meter({
  label,
  value,
  values,
  max,
  stroke,
  fill,
}: {
  label: string
  value: string
  values: number[]
  max?: number | 'auto'
  stroke: string
  fill: string
}) {
  return (
    <div className="flex min-w-0 flex-1 flex-col justify-center border-l border-rule px-3">
      <div className="flex items-baseline justify-between">
        <span className="plate">{label}</span>
        <span className="readout text-xs text-readout tabular-nums">{value}</span>
      </div>
      <Sparkline values={values} stroke={stroke} fill={fill} max={max ?? 1} />
    </div>
  )
}

export function HostStrip({ host, selected, onSelect }: Props) {
  const { peer, online, latest, hist, info } = host
  const c = useThemeColors()
  const cpu = hist.cpu.at(-1) ?? 0
  const mem = hist.mem.at(-1) ?? 0
  const net = hist.net.at(-1) ?? 0

  return (
    <button
      onClick={onSelect}
      className={[
        'flex h-strip w-full items-stretch border-b border-rule text-left outline-none',
        'transition-colors duration-led focus-visible:ring-1 focus-visible:ring-signal',
        selected ? 'bg-rack' : 'bg-panel hover:bg-rack/60',
        online ? '' : 'opacity-70',
      ].join(' ')}
    >
      {/* Left accent marks the focused channel. */}
      <span className={`w-[3px] shrink-0 ${selected ? 'bg-signal' : 'bg-transparent'}`} />

      {/* Name plate. */}
      <div className="flex w-40 shrink-0 flex-col justify-center px-3">
        <span className="font-plate text-[13px] font-semibold uppercase tracking-wider text-readout">
          {peer.name}
        </span>
        <span className="readout text-[10px] text-plate">{peer.address}</span>
      </div>

      {online ? (
        <>
          <Meter label="CPU" value={pct(cpu)} values={hist.cpu} stroke={c.signal} fill={c.signalFill} />
          <Meter label="RAM" value={latest ? bytes(latest.memory.used) : pct(mem)} values={hist.mem} stroke={c.signal} fill={c.signalFill} />
          <Meter label="NET" value={rate(net)} values={hist.net} max="auto" stroke={c.signal} fill={c.signalFill} />
        </>
      ) : (
        <div className="flex flex-1 items-center gap-3 border-l border-rule px-3">
          <span className="plate rounded-sm bg-fault/15 px-1.5 py-0.5 text-fault">Disconnected</span>
          <span className="readout text-[11px] text-plate">last seen {sinceLabel(host.lastSeen)}</span>
        </div>
      )}

      {/* LED cluster. */}
      <div className="flex w-36 shrink-0 flex-col justify-center gap-1 border-l border-rule px-3">
        <Led on={online} kind={online ? 'signal' : 'fault'} label={online ? 'Agent' : 'Down'} />
        <div className="flex gap-3">
          <Led on={online && !!info?.capabilities.systemd} label="Sysd" />
          <Led on={online && !!info?.capabilities.docker} label="Dckr" />
        </div>
      </div>
    </button>
  )
}
