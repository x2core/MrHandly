// The fleet store. It fans the Rust core's events into per-host runtime state
// with short metric history for the sparklines. This is the app's only
// aggregation point on the JS side; it holds no sockets.

import { create } from 'zustand'
import type { Info, Metrics, MetricsEvent, Peer, StatusEvent } from './types'

/** Samples retained per host (~2 min at 1 Hz) for the sparklines and charts. */
export const HISTORY = 120

export interface History {
  t: number[]
  cpu: number[] // 0..1
  mem: number[] // 0..1 (used / total)
  net: number[] // bytes/sec (rx+tx)
}

export interface HostState {
  peer: Peer
  online: boolean
  info: Info | null
  latest: Metrics | null
  lastSeen: number | null
  hist: History
  prevNet: { total: number; t: number } | null
}

interface FleetState {
  order: string[]
  hosts: Record<string, HostState>
  selected: string | null
  setPeers: (peers: Peer[]) => void
  applyStatus: (e: StatusEvent) => void
  applyMetrics: (e: MetricsEvent) => void
  select: (address: string | null) => void
}

function emptyHost(peer: Peer): HostState {
  return {
    peer,
    online: false,
    info: null,
    latest: null,
    lastSeen: null,
    hist: { t: [], cpu: [], mem: [], net: [] },
    prevNet: null,
  }
}

function push(hist: History, t: number, cpu: number, mem: number, net: number): History {
  const cap = (a: number[], v: number) => {
    const next = a.length >= HISTORY ? a.slice(a.length - HISTORY + 1) : a.slice()
    next.push(v)
    return next
  }
  return { t: cap(hist.t, t), cpu: cap(hist.cpu, cpu), mem: cap(hist.mem, mem), net: cap(hist.net, net) }
}

function netTotal(m: Metrics): number {
  let total = 0
  for (const dev of Object.values(m.network)) total += dev.rx_bytes + dev.tx_bytes
  return total
}

export const useFleet = create<FleetState>((set) => ({
  order: [],
  hosts: {},
  selected: null,

  setPeers: (peers) =>
    set((state) => {
      const hosts: Record<string, HostState> = {}
      for (const p of peers) {
        const existing = state.hosts[p.address]
        hosts[p.address] = existing ? { ...existing, peer: p } : emptyHost(p)
      }
      const order = peers.map((p) => p.address)
      const selected = state.selected && hosts[state.selected] ? state.selected : (order[0] ?? null)
      return { hosts, order, selected }
    }),

  applyStatus: (e) =>
    set((state) => {
      const h = state.hosts[e.address]
      if (!h) return {}
      return {
        hosts: {
          ...state.hosts,
          [e.address]: { ...h, online: e.online, info: e.info ?? h.info },
        },
      }
    }),

  applyMetrics: (e) =>
    set((state) => {
      const h = state.hosts[e.address]
      if (!h) return {}
      const m = e.metrics
      const cpu = m.cpu.usage
      const mem = m.memory.total > 0 ? m.memory.used / m.memory.total : 0

      const total = netTotal(m)
      const tSec = m.timestamp / 1000
      let net = 0
      if (h.prevNet) {
        const dt = tSec - h.prevNet.t
        if (dt > 0) net = Math.max(0, (total - h.prevNet.total) / dt)
      }

      return {
        hosts: {
          ...state.hosts,
          [e.address]: {
            ...h,
            online: true,
            latest: m,
            lastSeen: Date.now(),
            hist: push(h.hist, m.timestamp, cpu, mem, net),
            prevNet: { total, t: tSec },
          },
        },
      }
    }),

  select: (address) => set({ selected: address }),
}))
