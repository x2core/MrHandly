// The command palette (⌘K) — a first-class surface, not a shortcut. The
// original grievance was that the default UI makes every task a form; the
// palette is the answer: `restart nginx on lab-02` without leaving the keyboard
// (CLAUDE.md §7). It resolves hosts, units and containers across the whole
// fleet in one fuzzy index, and confirms destructive actions inline.

import { useEffect, useMemo, useState } from 'react'
import { Command } from 'cmdk'
import { bridge } from '../bridge'
import { useFleet } from '../store'
import { useToast } from './Toast'
import type { Container, Service } from '../types'

type Verb = 'start' | 'stop' | 'restart'
const PAST: Record<Verb, string> = { start: 'Started', stop: 'Stopped', restart: 'Restarted' }
const DESTRUCTIVE: Verb[] = ['stop', 'restart']

interface ActionCmd {
  id: string
  verb: Verb
  kind: 'service' | 'container'
  target: string // unit name or container id
  label: string // display target (unit / container name)
  hostName: string
  address: string
}

interface PendingConfirm {
  run: () => Promise<void>
  text: string
}

export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null)
  const [services, setServices] = useState<Record<string, Service[]>>({})
  const [containers, setContainers] = useState<Record<string, Container[]>>({})

  const order = useFleet((s) => s.order)
  const hosts = useFleet((s) => s.hosts)
  const select = useFleet((s) => s.select)
  const toast = useToast()

  // ⌘K / Ctrl-K toggles the palette.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Load actionable targets for online hosts when the palette opens.
  useEffect(() => {
    if (!open) return
    setConfirm(null)
    let active = true
    for (const address of order) {
      const h = hosts[address]
      if (!h?.online) continue
      void bridge.fetchServices(address).then((s) => active && setServices((m) => ({ ...m, [address]: s })))
      if (h.info?.capabilities.docker) {
        void bridge.fetchContainers(address).then((c) => active && setContainers((m) => ({ ...m, [address]: c })))
      }
    }
    return () => {
      active = false
    }
  }, [open, order, hosts])

  const actions = useMemo<ActionCmd[]>(() => {
    const out: ActionCmd[] = []
    for (const address of order) {
      const h = hosts[address]
      if (!h?.online) continue
      const hostName = h.peer.name
      for (const svc of services[address] ?? []) {
        if (!svc.writable) continue
        for (const verb of ['restart', 'stop', 'start'] as Verb[]) {
          out.push({ id: `${address}:svc:${svc.name}:${verb}`, verb, kind: 'service', target: svc.name, label: svc.name, hostName, address })
        }
      }
      for (const ct of containers[address] ?? []) {
        if (!ct.writable) continue
        for (const verb of ['restart', 'stop', 'start'] as Verb[]) {
          out.push({ id: `${address}:ctr:${ct.id}:${verb}`, verb, kind: 'container', target: ct.id, label: ct.name, hostName, address })
        }
      }
    }
    return out
  }, [order, hosts, services, containers])

  const run = async (a: ActionCmd) => {
    try {
      if (a.kind === 'service') await bridge.serviceAction(a.address, a.target, a.verb)
      else await bridge.containerAction(a.address, a.target, a.verb)
      toast(`${PAST[a.verb]} ${a.label} on ${a.hostName}`)
    } catch (e) {
      toast(`Failed: ${a.verb} ${a.label} on ${a.hostName}`, 'error')
      void e
    }
  }

  const onAction = (a: ActionCmd) => {
    if (DESTRUCTIVE.includes(a.verb)) {
      setConfirm({
        run: () => run(a),
        text: `${cap(a.verb)} ${a.label} on ${a.hostName}?`,
      })
      return
    }
    setOpen(false)
    void run(a)
  }

  return (
    <Command.Dialog
      open={open}
      onOpenChange={setOpen}
      label="Command palette"
      className="fixed left-1/2 top-[18%] z-50 w-[560px] -translate-x-1/2 border border-rule bg-rack shadow-2xl"
      overlayClassName="fixed inset-0 z-40 bg-black/60"
    >
      {confirm ? (
        <div className="p-4">
          <p className="font-plate uppercase tracking-wider text-readout">{confirm.text}</p>
          <p className="plate mt-1">This action changes host state.</p>
          <div className="mt-4 flex justify-end gap-2">
            <button onClick={() => setConfirm(null)} className={btnGhost}>
              Cancel
            </button>
            <button
              autoFocus
              onClick={() => {
                const c = confirm
                setConfirm(null)
                setOpen(false)
                void c.run()
              }}
              className={btnDanger}
            >
              Confirm
            </button>
          </div>
        </div>
      ) : (
        <>
          <Command.Input
            autoFocus
            placeholder="restart nginx on lab-02 …"
            className="w-full border-b border-rule bg-panel px-4 py-3 font-mono text-sm text-readout outline-none placeholder:text-plate/60"
          />
          <Command.List className="max-h-[340px] overflow-y-auto p-1">
            <Command.Empty className="plate px-3 py-6 text-center">No matches</Command.Empty>

            <Command.Group heading="Hosts" className="[&_[cmdk-group-heading]]:plate [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-1">
              {order.map((address) => {
                const h = hosts[address]
                if (!h) return null
                return (
                  <Command.Item
                    key={`focus:${address}`}
                    value={`focus ${h.peer.name} ${address}`}
                    onSelect={() => {
                      select(address)
                      setOpen(false)
                    }}
                    className={itemCls}
                  >
                    <span className="text-plate">Focus</span>
                    <span className="text-readout">{h.peer.name}</span>
                    <span className="ml-auto text-plate">{address}</span>
                  </Command.Item>
                )
              })}
            </Command.Group>

            {actions.length > 0 && (
              <Command.Group heading="Actions" className="[&_[cmdk-group-heading]]:plate [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-1">
                {actions.map((a) => (
                  <Command.Item
                    key={a.id}
                    value={`${a.verb} ${a.label} on ${a.hostName}`}
                    onSelect={() => onAction(a)}
                    className={itemCls}
                  >
                    <span className={DESTRUCTIVE.includes(a.verb) ? 'text-fault' : 'text-signal'}>{cap(a.verb)}</span>
                    <span className="text-readout">{a.label}</span>
                    <span className="text-plate">on</span>
                    <span className="text-readout">{a.hostName}</span>
                    <span className="ml-auto text-plate">{a.kind === 'container' ? '⬢' : 'unit'}</span>
                  </Command.Item>
                ))}
              </Command.Group>
            )}
          </Command.List>
          <div className="flex items-center justify-between border-t border-rule px-3 py-1">
            <span className="plate text-[9px]">↑↓ navigate · ↵ run · esc close</span>
            <span className="plate text-[9px]">⌘K</span>
          </div>
        </>
      )}
    </Command.Dialog>
  )
}

function cap(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

const itemCls =
  'flex cursor-pointer items-center gap-2 px-3 py-2 font-mono text-sm text-readout data-[selected=true]:bg-panel data-[selected=true]:text-readout'
const btnGhost = 'border border-rule px-3 py-1.5 font-plate text-xs uppercase tracking-wider text-plate hover:text-readout'
const btnDanger =
  'border border-fault bg-fault/10 px-3 py-1.5 font-plate text-xs uppercase tracking-wider text-fault hover:bg-fault/20'
