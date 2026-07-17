// The Fleet Strip: a persistent bank at the top, one fixed-height row per host,
// always visible and never scrolled away. This is the signature element — the
// whole fleet at a glance, the thing Proxmox never gives you (CLAUDE.md §7).

import { useFleet } from '../store'
import { HostStrip } from './HostStrip'

export function FleetStrip() {
  const order = useFleet((s) => s.order)
  const hosts = useFleet((s) => s.hosts)
  const selected = useFleet((s) => s.selected)
  const select = useFleet((s) => s.select)

  if (order.length === 0) {
    return (
      <div className="flex h-strip items-center border-b border-rule px-4">
        <span className="plate text-plate">No peers — add a machine to begin</span>
      </div>
    )
  }

  return (
    <div className="shrink-0 border-b border-rule">
      <div className="flex items-center justify-between px-4 py-1">
        <span className="plate">Fleet</span>
        <span className="plate text-[9px]">{order.length} hosts</span>
      </div>
      <div>
        {order.map((addr) => {
          const host = hosts[addr]
          if (!host) return null
          return (
            <HostStrip
              key={addr}
              host={host}
              selected={selected === addr}
              onSelect={() => select(addr)}
            />
          )
        })}
      </div>
    </div>
  )
}
