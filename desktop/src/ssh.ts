// SSH session state. This is deliberately separate from the agent: the Oikos
// agent has no shell (CLAUDE.md §4.4). A terminal is a direct SSH PTY from this
// desktop app using the operator's own credentials — a different channel with a
// different trust model.
//
// Only non-secret metadata (username, port, auth method, key path) is kept
// here and persisted. The secret (password / key passphrase) is entered at
// connect time and handed straight to the backend — never stored in the store.

import { create } from 'zustand'

export type SshAuth = 'key' | 'password'

export interface SshCreds {
  username: string
  port: number
  auth: SshAuth
  keyPath?: string
}

const KEY = 'oikos.ssh'

function load(): Record<string, SshCreds> {
  if (typeof localStorage === 'undefined') return {}
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? '{}')
  } catch {
    return {}
  }
}

function persist(creds: Record<string, SshCreds>) {
  if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, JSON.stringify(creds))
}

interface SshStore {
  creds: Record<string, SshCreds>
  connected: Record<string, boolean>
  setCreds: (address: string, creds: SshCreds) => void
  forget: (address: string) => void
  setConnected: (address: string, v: boolean) => void
}

export const useSsh = create<SshStore>((set) => ({
  creds: load(),
  connected: {},
  setCreds: (address, creds) =>
    set((s) => {
      const next = { ...s.creds, [address]: creds }
      persist(next)
      return { creds: next }
    }),
  forget: (address) =>
    set((s) => {
      const next = { ...s.creds }
      delete next[address]
      persist(next)
      const conn = { ...s.connected }
      delete conn[address]
      return { creds: next, connected: conn }
    }),
  setConnected: (address, v) => set((s) => ({ connected: { ...s.connected, [address]: v } })),
}))
