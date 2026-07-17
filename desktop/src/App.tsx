// The app shell. It wires the bridge's events into the fleet store and lays out
// the three persistent surfaces: the top rail, the always-visible Fleet Strip,
// and the focus/Performance view for the selected host.

import { useEffect, useState } from 'react'
import { bridge } from './bridge'
import { useFleet } from './store'
import { Sidebar } from './components/Sidebar'
import { FleetStrip } from './components/FleetStrip'
import { FocusView } from './components/FocusView'
import { PeerDialog, type DialogMode } from './components/PeerDialog'
import { CommandPalette } from './components/CommandPalette'
import { ToastProvider } from './components/Toast'

export default function App() {
  const setPeers = useFleet((s) => s.setPeers)
  const applyStatus = useFleet((s) => s.applyStatus)
  const applyMetrics = useFleet((s) => s.applyMetrics)
  const selected = useFleet((s) => s.selected)
  const hosts = useFleet((s) => s.hosts)

  const [dialog, setDialog] = useState<DialogMode | null>(null)

  useEffect(() => {
    let active = true
    void bridge.listPeers().then((peers) => active && setPeers(peers))
    const offPeers = bridge.onPeers(setPeers)
    const offStatus = bridge.onStatus(applyStatus)
    const offMetrics = bridge.onMetrics(applyMetrics)
    return () => {
      active = false
      offPeers()
      offStatus()
      offMetrics()
    }
  }, [setPeers, applyStatus, applyMetrics])

  const focused = selected ? hosts[selected] : undefined

  const removeSelected = () => {
    if (!focused) return
    if (window.confirm(`Remove ${focused.peer.name} (${focused.peer.address})?`)) {
      void bridge.removePeer(focused.peer.address)
    }
  }

  return (
    <ToastProvider>
      <div className="flex h-full">
        <Sidebar onAdd={() => setDialog({ kind: 'add' })} />
        <main className="flex min-w-0 flex-1 flex-col">
          <FleetStrip />
          {focused ? (
            <FocusView
              host={focused}
              onRename={() => setDialog({ kind: 'rename', peer: focused.peer })}
              onRemove={removeSelected}
            />
          ) : (
            <div className="flex flex-1 items-center justify-center">
              <span className="plate text-plate">Select a host to inspect it</span>
            </div>
          )}
        </main>
        <PeerDialog mode={dialog} onClose={() => setDialog(null)} />
        <CommandPalette />
      </div>
    </ToastProvider>
  )
}
