// Theme engine. Each theme is a full moodboard — not just colours but the
// vibe: corner radius, border weight, and density. Colours are CSS custom
// properties (RGB channel triplets so Tailwind's `/alpha` still works), swapped
// by stamping `data-theme` on <html>. See index.css for the values.

import { create } from 'zustand'

export type ThemeId = 'rack' | 'hacker' | 'corporate' | 'dashboard' | 'industrial'

export interface ThemeMeta {
  id: ThemeId
  label: string
  blurb: string
  /** Small swatch used in the picker: [bg, surface, accent, text]. */
  swatch: [string, string, string, string]
  light?: boolean
}

export const THEMES: ThemeMeta[] = [
  {
    id: 'rack',
    label: 'Rack',
    blurb: 'Warm graphite instrument. Dense, hard edges.',
    swatch: ['#1B1A18', '#232220', '#E8A33D', '#EDEAE3'],
  },
  {
    id: 'hacker',
    label: 'Strawberry Hacker',
    blurb: 'Phosphor green on black, strawberry alarms.',
    swatch: ['#080C08', '#0D140E', '#35FF6B', '#C8FFD4'],
  },
  {
    id: 'corporate',
    label: 'Corporate',
    blurb: 'Premium blue-grey. Calm, rounded, comfortable.',
    swatch: ['#14181F', '#1B212B', '#4F8FF0', '#E7ECF3'],
  },
  {
    id: 'dashboard',
    label: 'Dashboard',
    blurb: 'White commercial SaaS. Airy, soft cards.',
    swatch: ['#F5F7FA', '#FFFFFF', '#2563EB', '#1E293B'],
    light: true,
  },
  {
    id: 'industrial',
    label: 'Industrial',
    blurb: 'Low brown, thick borders, boxy signage.',
    swatch: ['#211C17', '#2C251E', '#C67730', '#E8DDCB'],
  },
]

const STORAGE_KEY = 'oikos.theme'

function initialTheme(): ThemeId {
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem(STORAGE_KEY) as ThemeId | null
    if (saved && THEMES.some((t) => t.id === saved)) return saved
  }
  return 'rack'
}

// Apply synchronously so getComputedStyle reads fresh vars right after a swap.
function apply(id: ThemeId) {
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.theme = id
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, id)
  }
}

const startId = initialTheme()
apply(startId)

interface ThemeStore {
  id: ThemeId
  set: (id: ThemeId) => void
}

export const useTheme = create<ThemeStore>((set) => ({
  id: startId,
  set: (id) => {
    apply(id)
    set({ id })
  },
}))
