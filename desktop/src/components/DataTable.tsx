// One table primitive, configured three ways (Processes, Services, Docker).
// TanStack Table drives sorting + columns; TanStack Virtual keeps long lists at
// 60fps by rendering only the visible rows (CLAUDE.md §6, ROADMAP M5).

import { useRef } from 'react'
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useState } from 'react'

interface Props<T> {
  data: T[]
  columns: ColumnDef<T, unknown>[]
  initialSort?: SortingState
  rowKey: (row: T) => string
  estimateRow?: number
}

export function DataTable<T>({ data, columns, initialSort = [], rowKey, estimateRow = 30 }: Props<T>) {
  const [sorting, setSorting] = useState<SortingState>(initialSort)
  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getRowId: rowKey,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  const rows = table.getRowModel().rows
  const parentRef = useRef<HTMLDivElement>(null)
  const virt = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateRow,
    overscan: 12,
  })

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Header */}
      <div className="flex border-b border-rule bg-rack">
        {table.getHeaderGroups().map((hg) =>
          hg.headers.map((h) => {
            const sort = h.column.getIsSorted()
            return (
              <button
                key={h.id}
                onClick={h.column.getToggleSortingHandler()}
                style={{ width: h.getSize() }}
                className={[
                  'plate flex h-row items-center gap-1 px-3 text-left',
                  h.column.getCanSort() ? 'hover:text-readout' : 'cursor-default',
                ].join(' ')}
              >
                {flexRender(h.column.columnDef.header, h.getContext())}
                {sort && <span className="text-signal">{sort === 'asc' ? '▲' : '▼'}</span>}
              </button>
            )
          }),
        )}
      </div>

      {/* Virtualized body */}
      <div ref={parentRef} className="min-h-0 flex-1 overflow-y-auto">
        <div style={{ height: virt.getTotalSize(), position: 'relative' }}>
          {virt.getVirtualItems().map((vi) => {
            const row = rows[vi.index]!
            return (
              <div
                key={row.id}
                className="absolute left-0 flex w-full items-center border-b border-rule/60 hover:bg-rack"
                style={{ height: vi.size, transform: `translateY(${vi.start}px)` }}
              >
                {row.getVisibleCells().map((cell) => (
                  <div
                    key={cell.id}
                    style={{ width: cell.column.getSize() }}
                    className="readout truncate px-3 text-xs text-readout"
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </div>
                ))}
              </div>
            )
          })}
        </div>
      </div>

      <div className="border-t border-rule px-3 py-1">
        <span className="plate text-[9px]">{rows.length} rows</span>
      </div>
    </div>
  )
}
