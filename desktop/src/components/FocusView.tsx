// The focus pane for the selected host. The detail tab is driven by the
// sidebar's view-nav (useUi); this renders the matching surface. An offline
// host keeps its layout and shows a disconnected plate rather than vanishing.

import type { HostState } from '../store'
import { useUi, type FocusTab } from '../ui'
import { useThemeColors } from '../theme-colors'
import { bytes, pct, rate, uptime, sinceLabel } from '../format'
import { PerfChart } from './PerfChart'
import { Led } from './Led'
import { ProcessesView } from './ProcessesView'
import { ServicesView } from './ServicesView'
import { DockerView } from './DockerView'
import { LogViewer } from './LogViewer'
import { Terminal } from './Terminal'

interface FocusProps {
  host: HostState
  onRename: () => void
  onRemove: () => void
}

function allowed(tab: FocusTab, systemd: boolean, docker: boolean, online: boolean): boolean {
  if (!online) return tab === 'performance'
  if (tab === 'services') return systemd
  if (tab === 'docker') return docker
  if (tab === 'logs') return systemd || docker
  return true
}

export function FocusView({ host, onRename, onRemove }: FocusProps) {
  const { peer, info, latest, online } = host
  const systemd = !!info?.capabilities.systemd
  const docker = !!info?.capabilities.docker

  const wanted = useUi((s) => s.tab)
  const tab = allowed(wanted, systemd, docker, online) ? wanted : 'performance'

  return (
    <div className="flex min-h-0 flex-1 flex-col border-t border-rule">
      {/* Header plate. */}
      <div className="u-pad flex shrink-0 items-center justify-between border-b border-rule">
        <div className="flex items-baseline gap-3">
          <span className="font-plate text-lg font-semibold uppercase tracking-wider text-readout">{peer.name}</span>
          <span className="readout text-xs text-plate">
            {peer.address}:{peer.port}
          </span>
          <span className="plate rounded border border-rule px-1.5 py-0.5 text-[9px]">{cap(tab)}</span>
        </div>
        <div className="flex items-center gap-4">
          <Led on={online} kind={online ? 'signal' : 'fault'} label={online ? 'Online' : 'Offline'} />
          {!online && <span className="readout text-[11px] text-plate">last seen {sinceLabel(host.lastSeen)}</span>}
          <div className="flex gap-2">
            <button onClick={onRename} className={headerBtn}>
              Rename
            </button>
            <button onClick={onRemove} className={`${headerBtn} hover:border-fault hover:text-fault`}>
              Remove
            </button>
          </div>
        </div>
      </div>

      {/* Identity readouts. */}
      <div className="grid shrink-0 grid-cols-2 border-b border-rule sm:grid-cols-3 lg:grid-cols-6">
        <Readout label="Host" value={info?.host.hostname ?? peer.name} />
        <Readout label="Distro" value={info?.host.distro ?? '—'} />
        <Readout label="Kernel" value={info?.host.kernel ?? '—'} />
        <Readout label="CPUs" value={info ? String(info.host.cpus) : '—'} />
        <Readout label="Memory" value={info ? bytes(info.host.total_memory) : '—'} />
        <Readout label="Uptime" value={latest ? uptime(latest.uptime_seconds) : '—'} />
      </div>

      {/* Tab content. */}
      <div className="flex min-h-0 flex-1 flex-col">
        {tab === 'performance' && <PerformanceTab host={host} />}
        {tab === 'processes' && <ProcessesView address={peer.address} />}
        {tab === 'services' && <ServicesView address={peer.address} />}
        {tab === 'docker' && <DockerView address={peer.address} />}
        {tab === 'logs' && <LogViewer address={peer.address} dockerAvailable={docker} />}
        {tab === 'terminal' && <Terminal address={peer.address} hostName={peer.name} />}
      </div>
    </div>
  )
}

function PerformanceTab({ host }: { host: HostState }) {
  const { hist, latest } = host
  const c = useThemeColors()
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="grid flex-1 grid-cols-1 gap-px bg-rule lg:grid-cols-3">
        <Panel title="CPU" value={pct(hist.cpu.at(-1) ?? 0)}>
          <PerfChart values={hist.cpu} stroke={c.signal} fill={c.signalFill} max={1} />
        </Panel>
        <Panel title="Memory" value={latest ? bytes(latest.memory.used) : '—'}>
          <PerfChart values={hist.mem} stroke={c.signal} fill={c.signalFill} max={1} />
        </Panel>
        <Panel title="Network" value={rate(hist.net.at(-1) ?? 0)}>
          <PerfChart values={hist.net} stroke={c.signal} fill={c.signalFill} max="auto" yFormat={bytes} />
        </Panel>
      </div>
      {latest && (
        <div className="u-pad flex shrink-0 items-center gap-6 border-t border-rule">
          <span className="plate">Load</span>
          <span className="readout text-sm">
            {latest.load.one.toFixed(2)} · {latest.load.five.toFixed(2)} · {latest.load.fifteen.toFixed(2)}
          </span>
          <span className="plate ml-auto">Swap</span>
          <span className="readout text-sm">
            {latest.memory.swap_total > 0
              ? `${bytes(latest.memory.swap_used)} / ${bytes(latest.memory.swap_total)}`
              : 'none'}
          </span>
        </div>
      )}
    </div>
  )
}

const headerBtn =
  'rounded border border-rule px-2 py-1 font-plate text-[10px] uppercase tracking-wider text-plate transition-colors duration-led hover:text-readout'

function cap(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function Readout({ label, value }: { label: string; value: string }) {
  return (
    <div className="u-pad flex flex-col gap-0.5 border-l border-rule">
      <span className="plate">{label}</span>
      <span className="readout text-sm text-readout">{value}</span>
    </div>
  )
}

function Panel({ title, value, children }: { title: string; value: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col bg-panel">
      <div className="flex items-baseline justify-between px-3 pt-2">
        <span className="plate">{title}</span>
        <span className="readout text-sm text-readout">{value}</span>
      </div>
      <div className="px-1 pb-1">{children}</div>
    </div>
  )
}
