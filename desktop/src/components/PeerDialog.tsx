// Add or rename a peer. Adding a machine is a name and a wg0 address — nothing
// else (CLAUDE.md §7 / ROADMAP M4). Radix Dialog gives real focus trapping and
// keyboard behaviour; the styling is ours.

import * as Dialog from '@radix-ui/react-dialog'
import { useEffect, useState } from 'react'
import { bridge } from '../bridge'
import { DEFAULT_PORT } from '../constants'
import type { Peer } from '../types'

export type DialogMode = { kind: 'add' } | { kind: 'rename'; peer: Peer }

interface Props {
  mode: DialogMode | null
  onClose: () => void
}

export function PeerDialog({ mode, onClose }: Props) {
  const isRename = mode?.kind === 'rename'
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [port, setPort] = useState(DEFAULT_PORT)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!mode) return
    setError(null)
    if (mode.kind === 'rename') {
      setName(mode.peer.name)
      setAddress(mode.peer.address)
      setPort(mode.peer.port)
    } else {
      setName('')
      setAddress('')
      setPort(DEFAULT_PORT)
    }
  }, [mode])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    try {
      if (mode?.kind === 'rename') {
        await bridge.renamePeer(mode.peer.address, name.trim())
      } else {
        if (!name.trim() || !address.trim()) {
          setError('Name and address are required')
          return
        }
        await bridge.addPeer(name.trim(), address.trim(), port)
      }
      onClose()
    } catch (err) {
      setError(String(err))
    }
  }

  return (
    <Dialog.Root open={mode !== null} onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-1/3 w-[380px] -translate-x-1/2 -translate-y-1/2 border border-rule bg-rack p-4 shadow-xl focus:outline-none">
          <Dialog.Title className="font-plate uppercase tracking-wider text-readout">
            {isRename ? 'Rename peer' : 'Add peer'}
          </Dialog.Title>
          <Dialog.Description className="plate mt-1">
            {isRename ? 'Change the display name' : 'A name and a wg0 address'}
          </Dialog.Description>

          <form onSubmit={submit} className="mt-4 flex flex-col gap-3">
            <Field label="Name">
              <input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={inputCls}
                placeholder="lab-03"
              />
            </Field>
            <Field label="Address">
              <input
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                disabled={isRename}
                className={`${inputCls} disabled:opacity-50`}
                placeholder="10.44.0.6"
              />
            </Field>
            <Field label="Port">
              <input
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value) || DEFAULT_PORT)}
                disabled={isRename}
                className={`${inputCls} disabled:opacity-50`}
              />
            </Field>

            {error && <p className="readout text-xs text-fault">{error}</p>}

            <div className="mt-2 flex justify-end gap-2">
              <Dialog.Close asChild>
                <button type="button" className={btnGhost}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className={btnPrimary}>
                {isRename ? 'Rename' : 'Add'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="plate">{label}</span>
      {children}
    </label>
  )
}

const inputCls =
  'border border-rule bg-panel px-2 py-1.5 font-mono text-sm text-readout tabular-nums outline-none focus:border-signal'
const btnGhost = 'border border-rule px-3 py-1.5 font-plate text-xs uppercase tracking-wider text-plate hover:text-readout'
const btnPrimary =
  'border border-signal bg-signal/10 px-3 py-1.5 font-plate text-xs uppercase tracking-wider text-signal hover:bg-signal/20'
