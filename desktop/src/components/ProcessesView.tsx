// The Processes view — the Task Manager panel. Virtualized, sorted by CPU or
// RSS. It subscribes on mount and unsubscribes on unmount, so the agent only
// samples the (expensive) process table while this tab is open (CLAUDE.md §8).

import { useEffect, useState } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { bridge } from '../bridge'
import type { Process } from '../types'
import { bytes, pct } from '../format'
import { DataTable } from './DataTable'

const columns: ColumnDef<Process, unknown>[] = [
  { accessorKey: 'pid', header: 'PID', size: 70, cell: (c) => c.getValue<number>() },
  {
    accessorKey: 'name',
    header: 'Command',
    size: 360,
    enableSorting: false,
    cell: (c) => <span title={c.getValue<string>()}>{c.getValue<string>()}</span>,
  },
  { accessorKey: 'state', header: 'St', size: 44, cell: (c) => c.getValue<string>() },
  {
    accessorKey: 'cpu',
    header: 'CPU',
    size: 120,
    cell: (c) => {
      const v = c.getValue<number>()
      return (
        <div className="flex items-center gap-2">
          <div className="h-1.5 w-12 bg-rule">
            <div className="h-full bg-signal" style={{ width: `${Math.min(100, v * 100)}%` }} />
          </div>
          <span className="tabular-nums">{pct(v)}</span>
        </div>
      )
    },
  },
  { accessorKey: 'rss', header: 'RSS', size: 90, cell: (c) => bytes(c.getValue<number>()) },
  { accessorKey: 'threads', header: 'Thr', size: 60, cell: (c) => c.getValue<number>() },
]

export function ProcessesView({ address }: { address: string }) {
  const [procs, setProcs] = useState<Process[]>([])

  useEffect(() => {
    const off = bridge.subscribeProcesses(address, setProcs)
    return off
  }, [address])

  return (
    <DataTable
      data={procs}
      columns={columns}
      rowKey={(p) => String(p.pid)}
      initialSort={[{ id: 'cpu', desc: true }]}
    />
  )
}
