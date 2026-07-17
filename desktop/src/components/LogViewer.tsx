// Shared log viewer for journald and Docker logs. Virtualized, follow-tail with
// break-on-scroll, a level filter, and plain-text rows (CLAUDE.md §7, ROADMAP
// M5). The source picker lists the host's units and containers.

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { bridge, type LogKind } from '../bridge'
import type { LogRecord } from '../types'

interface Source {
  kind: LogKind
  target: string
  label: string
}

const CAP = 1000

export function LogViewer({ address, dockerAvailable }: { address: string; dockerAvailable: boolean }) {
  const [sources, setSources] = useState<Source[]>([])
  const [selected, setSelected] = useState<Source | null>(null)
  const [lines, setLines] = useState<LogRecord[]>([])
  const [minPriority, setMinPriority] = useState(7) // 7 = show everything
  const [follow, setFollow] = useState(true)

  // Build the source list from units + containers.
  useEffect(() => {
    const off = bridge.subscribeServices(address, (svcs) => {
      setSources((prev) => {
        const containers = prev.filter((s) => s.kind === 'container')
        const units = svcs.map((s) => ({ kind: 'service' as const, target: s.name, label: s.name }))
        return [...units, ...containers]
      })
    })
    if (dockerAvailable) {
      void bridge.fetchContainers(address).then((cs) =>
        setSources((prev) => {
          const units = prev.filter((s) => s.kind === 'service')
          const containers = cs.map((c) => ({ kind: 'container' as const, target: c.id, label: `⬢ ${c.name}` }))
          return [...units, ...containers]
        }),
      )
    }
    return off
  }, [address, dockerAvailable])

  useEffect(() => {
    if (!selected && sources.length) setSelected(sources[0]!)
  }, [sources, selected])

  // Stream the selected source.
  useEffect(() => {
    if (!selected) return
    setLines([])
    const off = bridge.subscribeLogs(address, selected.kind, selected.target, (line) => {
      setLines((prev) => {
        const next = prev.length >= CAP ? prev.slice(prev.length - CAP + 1) : prev.slice()
        next.push(line)
        return next
      })
    })
    return off
  }, [address, selected])

  const filtered = useMemo(
    () => lines.filter((l) => l.priority < 0 || l.priority <= minPriority),
    [lines, minPriority],
  )

  const parentRef = useRef<HTMLDivElement>(null)
  const virt = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 18,
    overscan: 20,
  })

  // Follow-tail: stick to the bottom unless the user scrolled up.
  useLayoutEffect(() => {
    if (follow && filtered.length) virt.scrollToIndex(filtered.length - 1)
  }, [filtered.length, follow, virt])

  const onScroll = () => {
    const el = parentRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    if (atBottom !== follow) setFollow(atBottom)
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-3 border-b border-rule px-3 py-1">
        <span className="plate">Source</span>
        <select
          value={selected ? `${selected.kind}:${selected.target}` : ''}
          onChange={(e) => {
            const [kind, ...rest] = e.target.value.split(':')
            const target = rest.join(':')
            const found = sources.find((s) => s.kind === kind && s.target === target)
            if (found) setSelected(found)
          }}
          className="border border-rule bg-panel px-2 py-0.5 font-mono text-xs text-readout outline-none"
        >
          {sources.map((s) => (
            <option key={`${s.kind}:${s.target}`} value={`${s.kind}:${s.target}`}>
              {s.label}
            </option>
          ))}
        </select>

        <span className="plate ml-2">Level</span>
        <select
          value={minPriority}
          onChange={(e) => setMinPriority(Number(e.target.value))}
          className="border border-rule bg-panel px-2 py-0.5 font-mono text-xs text-readout outline-none"
        >
          <option value={7}>all</option>
          <option value={4}>warn+</option>
          <option value={3}>error+</option>
        </select>

        <button
          onClick={() => setFollow((f) => !f)}
          className={`ml-auto border px-2 py-0.5 font-plate text-[10px] uppercase tracking-wider transition-colors duration-led ${
            follow ? 'border-signal text-signal' : 'border-rule text-plate hover:text-readout'
          }`}
        >
          Follow
        </button>
      </div>

      <div ref={parentRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto bg-panel px-2 py-1">
        <div style={{ height: virt.getTotalSize(), position: 'relative' }}>
          {virt.getVirtualItems().map((vi) => {
            const l = filtered[vi.index]!
            const color =
              l.priority >= 0 && l.priority <= 3
                ? 'text-fault'
                : l.priority === 4
                  ? 'text-signal'
                  : l.source === 'stderr'
                    ? 'text-fault'
                    : 'text-readout'
            return (
              <div
                key={vi.index}
                className={`readout absolute left-0 flex w-full gap-2 whitespace-pre text-[11px] leading-[18px] ${color}`}
                style={{ height: vi.size, transform: `translateY(${vi.start}px)` }}
              >
                <span className="shrink-0 text-plate">{fmtTime(l.timestamp)}</span>
                <span className="truncate">{l.message}</span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function fmtTime(ts: number): string {
  if (!ts) return '--:--:--'
  const d = new Date(ts)
  return d.toTimeString().slice(0, 8)
}
