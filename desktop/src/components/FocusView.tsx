// The focus view: click a host and its detail pane opens below as a set of
// tabs — Performance, Processes, Services, Docker, Logs — over the same peer.
// Capability-gated tabs (Services/Docker/Logs) appear only where the host
// supports them. An offline host keeps its layout and shows a disconnected
// plate rather than vanishing.

import { useEffect, useState } from 'react'
import type { HostState } from '../store'
import { bytes, pct, rate, uptime, sinceLabel } from '../format'
import { PerfChart } from './PerfChart'
import { Led } from './Led'
import { ProcessesView } from './ProcessesView'
import { ServicesView } from './ServicesView'
import { DockerView } from './DockerView'
import { LogViewer } from './LogViewer'

const SIGNAL = '#E8A33D'
const SIGNAL_FILL = 'rgba(232,163,61,0.14)'

type Tab = 'performance' | 'processes' | 'services' | 'docker' | 'logs'

interface FocusProps {
  host: HostState
  onRename: () => void
  onRemove: () => void
}

export function FocusView({ host, onRename, onRemove }: FocusProps) {
  const { peer, info, latest, online } = host
  const systemd = !!info?.capabilities.systemd
  const docker = !!info?.capabilities.docker

  const tabs: { id: Tab; label: string; show: boolean }[] = [
    { id: 'performance', label: 'Performance', show: true },
    { id: 'processes', label: 'Processes', show: online },
    { id: 'services', label: 'Services', show: online && systemd },
    { id: 'docker', label: 'Docker', show: online && docker },
    { id: 'logs', label: 'Logs', show: online && (systemd || docker) },
  ]

  const [tab, setTab] = useState<Tab>('performance')
  // Reset to a valid tab when the host (or its capabilities) change.
  useEffect(() => {
    if (!tabs.find((t) => t.id === tab && t.show)) setTab('performance')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [peer.address, online, systemd, docker])

  return (
    <div className="flex min-h-0 flex-1 flex-col border-t border-rule">
      {/* Header plate. */}
      <div className="flex shrink-0 items-center justify-between border-b border-rule px-4 py-2">
        <div className="flex items-baseline gap-3">
          <span className="font-plate text-lg font-semibold uppercase tracking-wider text-readout">{peer.name}</span>
          <span className="readout text-xs text-plate">
            {peer.address}:{peer.port}
          </span>
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

      {/* Tab bar. */}
      <div className="flex shrink-0 border-b border-rule bg-rack">
        {tabs
          .filter((t) => t.show)
          .map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={[
                'border-r border-rule px-4 py-2 font-plate text-[11px] uppercase tracking-wider transition-colors duration-led',
                tab === t.id ? 'bg-panel text-readout' : 'text-plate hover:text-readout',
              ].join(' ')}
            >
              <span className="flex items-center gap-2">
                {tab === t.id && <span className="h-[6px] w-[6px] rounded-full bg-signal" />}
                {t.label}
              </span>
            </button>
          ))}
      </div>

      {/* Tab content. */}
      <div className="flex min-h-0 flex-1 flex-col">
        {tab === 'performance' && <PerformanceTab host={host} />}
        {tab === 'processes' && <ProcessesView address={peer.address} />}
        {tab === 'services' && <ServicesView address={peer.address} />}
        {tab === 'docker' && <DockerView address={peer.address} />}
        {tab === 'logs' && <LogViewer address={peer.address} dockerAvailable={docker} />}
      </div>
    </div>
  )
}

function PerformanceTab({ host }: { host: HostState }) {
  const { hist, latest } = host
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="grid flex-1 grid-cols-1 gap-px bg-rule lg:grid-cols-3">
        <Panel title="CPU" value={pct(hist.cpu.at(-1) ?? 0)}>
          <PerfChart values={hist.cpu} stroke={SIGNAL} fill={SIGNAL_FILL} max={1} />
        </Panel>
        <Panel title="Memory" value={latest ? bytes(latest.memory.used) : '—'}>
          <PerfChart values={hist.mem} stroke={SIGNAL} fill={SIGNAL_FILL} max={1} />
        </Panel>
        <Panel title="Network" value={rate(hist.net.at(-1) ?? 0)}>
          <PerfChart values={hist.net} stroke={SIGNAL} fill={SIGNAL_FILL} max="auto" yFormat={bytes} />
        </Panel>
      </div>
      {latest && (
        <div className="flex shrink-0 items-center gap-6 border-t border-rule px-4 py-2">
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
  'border border-rule px-2 py-1 font-plate text-[10px] uppercase tracking-wider text-plate transition-colors duration-led hover:text-readout'

function Readout({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-l border-rule px-3 py-1">
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
