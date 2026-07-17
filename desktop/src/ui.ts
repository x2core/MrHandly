// Small UI store: which detail tab the sidebar's view-nav has selected, and
// the sidebar collapsed state. Kept out of the fleet store since it is pure
// view state.

import { create } from 'zustand'

export type FocusTab = 'performance' | 'processes' | 'services' | 'docker' | 'logs' | 'terminal'

interface UiStore {
  tab: FocusTab
  setTab: (t: FocusTab) => void
  collapsed: boolean
  toggleCollapsed: () => void
}

export const useUi = create<UiStore>((set) => ({
  tab: 'performance',
  setTab: (tab) => set({ tab }),
  collapsed: false,
  toggleCollapsed: () => set((s) => ({ collapsed: !s.collapsed })),
}))
