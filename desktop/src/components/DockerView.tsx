// The Docker view — containers + images, rendered only where the host reports
// docker capability. Polls one-shot fetches while mounted (no deltas needed).

import { useEffect, useMemo, useState } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { bridge } from '../bridge'
import type { Container, Image } from '../types'
import { bytes } from '../format'
import { DataTable } from './DataTable'

export function DockerView({ address }: { address: string }) {
  const [containers, setContainers] = useState<Container[]>([])
  const [images, setImages] = useState<Image[]>([])

  useEffect(() => {
    let active = true
    const poll = () => {
      void bridge.fetchContainers(address).then((c) => active && setContainers(c)).catch(() => {})
      void bridge.fetchImages(address).then((i) => active && setImages(i)).catch(() => {})
    }
    poll()
    const h = setInterval(poll, 3000)
    return () => {
      active = false
      clearInterval(h)
    }
  }, [address])

  const containerCols = useMemo<ColumnDef<Container, unknown>[]>(
    () => [
      {
        id: 'led',
        header: '',
        size: 26,
        enableSorting: false,
        cell: (c) => {
          const on = c.row.original.state === 'running'
          return (
            <span
              className={`inline-block h-[7px] w-[7px] rounded-full ${on ? 'bg-signal shadow-[0_0_6px_#E8A33D]' : 'bg-rule'}`}
            />
          )
        },
      },
      { accessorKey: 'name', header: 'Name', size: 150, cell: (c) => c.getValue<string>() },
      { accessorKey: 'image', header: 'Image', size: 200, cell: (c) => <span className="text-plate">{c.getValue<string>()}</span> },
      { accessorKey: 'status', header: 'Status', size: 220, enableSorting: false, cell: (c) => c.getValue<string>() },
      {
        id: 'actions',
        header: 'Actions',
        size: 190,
        enableSorting: false,
        cell: (c) => {
          const ct = c.row.original
          const run = (a: string) => () => void bridge.containerAction(address, ct.id, a)
          const cls =
            'border border-rule px-1.5 py-0.5 font-plate text-[9px] uppercase tracking-wider text-plate hover:text-readout disabled:opacity-40'
          return (
            <div className="flex gap-1">
              <button className={cls} disabled={!ct.writable} onClick={run('start')}>Start</button>
              <button className={cls} disabled={!ct.writable} onClick={run('stop')}>Stop</button>
              <button className={cls} disabled={!ct.writable} onClick={run('restart')}>Restart</button>
            </div>
          )
        },
      },
    ],
    [address],
  )

  const imageCols = useMemo<ColumnDef<Image, unknown>[]>(
    () => [
      {
        accessorKey: 'tags',
        header: 'Tag',
        size: 300,
        enableSorting: false,
        cell: (c) => {
          const tags = c.getValue<string[]>()
          return tags.length ? tags.join(', ') : <span className="text-plate">&lt;none&gt;</span>
        },
      },
      { accessorKey: 'size', header: 'Size', size: 100, cell: (c) => bytes(c.getValue<number>()) },
      { accessorKey: 'id', header: 'ID', size: 200, enableSorting: false, cell: (c) => <span className="text-plate">{c.getValue<string>().replace('sha256:', '').slice(0, 12)}</span> },
    ],
    [],
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center border-b border-rule px-3 py-1">
        <span className="plate">Containers</span>
      </div>
      <div className="flex min-h-0 flex-1">
        <DataTable data={containers} columns={containerCols} rowKey={(c) => c.id} />
      </div>
      <div className="flex items-center border-y border-rule px-3 py-1">
        <span className="plate">Images</span>
      </div>
      <div className="flex min-h-0 flex-1">
        <DataTable data={images} columns={imageCols} rowKey={(i) => i.id} />
      </div>
    </div>
  )
}
