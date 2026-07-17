// Minimal toast stack. Copy uses the same verb as the action (Restart →
// Restarted), so the confirmation reads as the completed command (CLAUDE.md §7).

import { createContext, useCallback, useContext, useRef, useState } from 'react'

interface Toast {
  id: number
  text: string
  kind: 'ok' | 'error'
}

const ToastContext = createContext<(text: string, kind?: 'ok' | 'error') => void>(() => {})

export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const next = useRef(0)

  const push = useCallback((text: string, kind: 'ok' | 'error' = 'ok') => {
    const id = next.current++
    setToasts((t) => [...t, { id, text, kind }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3200)
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={[
              'border px-3 py-2 font-mono text-xs shadow-lg',
              t.kind === 'error'
                ? 'border-fault/60 bg-rack text-fault'
                : 'border-rule bg-rack text-readout',
            ].join(' ')}
          >
            {t.text}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
