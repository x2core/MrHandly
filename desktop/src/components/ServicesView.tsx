// The Services view — systemd units with state LEDs. Allowlisted actions are
// enabled; the rest are visibly disabled with a reason, so the operator sees
// the boundary rather than a hidden control (CLAUDE.md §7, ROADMAP M5).

import { useEffect, useMemo, useState } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { bridge } from '../bridge'
import type { Service } from '../types'
import { DataTable } from './DataTable'

function ActionButton({
  label,
  disabled,
  danger,
  onClick,
}: {
  label: string
  disabled: boolean
  danger?: boolean
  onClick: () => void
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      title={disabled ? 'not in the write allowlist' : undefined}
      className={[
        'border px-1.5 py-0.5 font-plate text-[9px] uppercase tracking-wider transition-colors duration-led',
        disabled
          ? 'cursor-not-allowed border-rule/60 text-plate/40'
          : danger
            ? 'border-rule text-plate hover:border-fault hover:text-fault'
            : 'border-rule text-plate hover:text-readout',
      ].join(' ')}
    >
      {label}
    </button>
  )
}

export function ServicesView({ address }: { address: string }) {
  const [services, setServices] = useState<Service[]>([])

  useEffect(() => {
    const off = bridge.subscribeServices(address, setServices)
    return off
  }, [address])

  const columns = useMemo<ColumnDef<Service, unknown>[]>(
    () => [
      {
        id: 'led',
        header: '',
        size: 26,
        enableSorting: false,
        cell: (c) => {
          const s = c.row.original
          const on = s.active_state === 'active'
          const failed = s.active_state === 'failed'
          const cls = failed ? 'bg-fault shadow-[var(--glow)_rgb(var(--c-fault))]' : on ? 'bg-signal shadow-[var(--glow)_rgb(var(--c-signal))]' : 'bg-rule'
          return <span className={`inline-block h-[7px] w-[7px] rounded-full ${cls}`} />
        },
      },
      { accessorKey: 'name', header: 'Unit', size: 200, cell: (c) => c.getValue<string>() },
      {
        accessorKey: 'description',
        header: 'Description',
        size: 300,
        enableSorting: false,
        cell: (c) => <span className="text-plate">{c.getValue<string>()}</span>,
      },
      { accessorKey: 'sub_state', header: 'State', size: 90, cell: (c) => c.getValue<string>() },
      {
        id: 'actions',
        header: 'Actions',
        size: 190,
        enableSorting: false,
        cell: (c) => {
          const s = c.row.original
          const run = (action: string) => () => void bridge.serviceAction(address, s.name, action)
          return (
            <div className="flex gap-1">
              <ActionButton label="Start" disabled={!s.writable} onClick={run('start')} />
              <ActionButton label="Stop" disabled={!s.writable} danger onClick={run('stop')} />
              <ActionButton label="Restart" disabled={!s.writable} onClick={run('restart')} />
            </div>
          )
        },
      },
    ],
    [address],
  )

  return <DataTable data={services} columns={columns} rowKey={(s) => s.name} estimateRow={30} />
}
