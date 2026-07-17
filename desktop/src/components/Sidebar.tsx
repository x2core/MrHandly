// The left sidebar — the app's primary navigation. Brand at the top, a live
// host navigator (LED + name + load), a contextual view-nav for the focused
// host, and an appearance picker + actions in the footer.

import { isMock } from '../bridge'
import { useFleet } from '../store'
import { useUi, type FocusTab } from '../ui'
import { THEMES, useTheme } from '../theme'
import { pct } from '../format'
import { IconBox, IconGrid, IconGear, IconPlus, IconPulse, IconRows, IconTerminal } from './icons'

export function Sidebar({ onAdd }: { onAdd: () => void }) {
  const order = useFleet((s) => s.order)
  const hosts = useFleet((s) => s.hosts)
  const selected = useFleet((s) => s.selected)
  const select = useFleet((s) => s.select)

  const online = order.filter((a) => hosts[a]?.online).length

  return (
    <aside className="flex h-full w-sidebar shrink-0 flex-col border-r border-rule bg-rack">
      {/* Brand */}
      <div className="u-pad flex items-center gap-2 border-b border-rule">
        <span className="grid h-7 w-7 place-items-center rounded bg-signal/15 text-signal">
          <IconGrid />
        </span>
        <div className="leading-tight">
          <div className="font-plate text-sm font-semibold uppercase tracking-[0.2em] text-readout">Oikos</div>
          <div className="plate text-[9px]">fleet control{isMock ? ' · demo' : ''}</div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {/* Host navigator */}
        <NavHeading label="Hosts" trailing={`${online}/${order.length}`} />
        <div className="px-2 pb-2">
          {order.length === 0 && <div className="plate px-2 py-2">No peers yet</div>}
          {order.map((addr) => {
            const h = hosts[addr]
            if (!h) return null
            const active = selected === addr
            const cpu = h.hist.cpu.at(-1) ?? 0
            return (
              <button
                key={addr}
                onClick={() => select(addr)}
                className={[
                  'u-pad-x group mb-0.5 flex w-full items-center gap-2 rounded py-2 text-left transition-colors duration-led',
                  active ? 'bg-panel text-readout' : 'text-plate hover:bg-panel/60 hover:text-readout',
                ].join(' ')}
              >
                <Dot online={h.online} />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-plate text-[12px] font-semibold uppercase tracking-wide text-readout">
                    {h.peer.name}
                  </div>
                  <div className="readout truncate text-[10px] text-plate">{h.peer.address}</div>
                </div>
                {h.online ? (
                  <span className="readout text-[10px] tabular-nums text-plate">{pct(cpu)}</span>
                ) : (
                  <span className="plate text-[9px] text-fault">down</span>
                )}
              </button>
            )
          })}
        </div>

        {/* View navigator (contextual to the focused host) */}
        <ViewNav />
      </div>

      {/* Footer: appearance + actions */}
      <div className="border-t border-rule">
        <NavHeading label="Appearance" />
        <ThemePicker />
        <div className="u-pad flex items-center gap-2 border-t border-rule">
          <button
            onClick={onAdd}
            className="flex flex-1 items-center justify-center gap-1.5 rounded border border-signal bg-signal/10 py-2 font-plate text-[11px] uppercase tracking-wider text-signal transition-colors duration-led hover:bg-signal/20"
          >
            <IconPlus />
            Add peer
          </button>
          <span className="plate rounded border border-rule px-2 py-2 text-[9px]">⌘K</span>
        </div>
      </div>
    </aside>
  )
}

const VIEW_ITEMS: { id: FocusTab; label: string; Icon: (p: { className?: string }) => JSX.Element; needs?: 'systemd' | 'docker' | 'either' }[] = [
  { id: 'performance', label: 'Performance', Icon: IconPulse },
  { id: 'processes', label: 'Processes', Icon: IconRows },
  { id: 'services', label: 'Services', Icon: IconGear, needs: 'systemd' },
  { id: 'docker', label: 'Docker', Icon: IconBox, needs: 'docker' },
  { id: 'logs', label: 'Logs', Icon: IconTerminal, needs: 'either' },
]

function ViewNav() {
  const selected = useFleet((s) => s.selected)
  const host = useFleet((s) => (s.selected ? s.hosts[s.selected] : undefined))
  const tab = useUi((s) => s.tab)
  const setTab = useUi((s) => s.setTab)

  if (!selected || !host?.online) return null
  const systemd = !!host.info?.capabilities.systemd
  const docker = !!host.info?.capabilities.docker
  const allowed = (needs?: string) =>
    !needs || (needs === 'systemd' && systemd) || (needs === 'docker' && docker) || (needs === 'either' && (systemd || docker))

  return (
    <>
      <NavHeading label={`View · ${host.peer.name}`} />
      <div className="px-2 pb-3">
        {VIEW_ITEMS.filter((v) => allowed(v.needs)).map((v) => {
          const active = tab === v.id
          return (
            <button
              key={v.id}
              onClick={() => setTab(v.id)}
              className={[
                'u-pad-x mb-0.5 flex w-full items-center gap-2.5 rounded py-1.5 text-left transition-colors duration-led',
                active ? 'bg-signal/12 text-signal' : 'text-plate hover:bg-panel/60 hover:text-readout',
              ].join(' ')}
            >
              <v.Icon />
              <span className="font-plate text-[11px] uppercase tracking-wider">{v.label}</span>
              {active && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-signal" />}
            </button>
          )
        })}
      </div>
    </>
  )
}

function ThemePicker() {
  const id = useTheme((s) => s.id)
  const setTheme = useTheme((s) => s.set)
  const current = THEMES.find((t) => t.id === id)

  return (
    <div className="u-pad">
      <div className="flex items-center gap-1.5">
        {THEMES.map((t) => (
          <button
            key={t.id}
            onClick={() => setTheme(t.id)}
            title={`${t.label} — ${t.blurb}`}
            className={[
              'relative h-7 w-full overflow-hidden rounded border transition-all duration-led',
              id === t.id ? 'border-signal ring-1 ring-signal' : 'border-rule hover:border-plate',
            ].join(' ')}
            style={{ background: t.swatch[0] }}
          >
            <span className="absolute inset-x-0 bottom-0 h-2" style={{ background: t.swatch[1] }} />
            <span className="absolute right-1 top-1 h-2 w-2 rounded-full" style={{ background: t.swatch[2] }} />
          </button>
        ))}
      </div>
      <div className="mt-1.5 flex items-baseline justify-between">
        <span className="font-plate text-[11px] uppercase tracking-wider text-readout">{current?.label}</span>
      </div>
      <p className="plate mt-0.5 text-[9px] normal-case leading-snug">{current?.blurb}</p>
    </div>
  )
}

function NavHeading({ label, trailing }: { label: string; trailing?: string }) {
  return (
    <div className="u-pad-x flex items-center justify-between pb-1 pt-3">
      <span className="plate text-[9px]">{label}</span>
      {trailing && <span className="plate text-[9px]">{trailing}</span>}
    </div>
  )
}

function Dot({ online }: { online: boolean }) {
  return (
    <span
      className={[
        'h-[7px] w-[7px] shrink-0 rounded-full transition-colors duration-led',
        online ? 'bg-signal shadow-[var(--glow)_rgb(var(--c-signal))]' : 'bg-rule',
      ].join(' ')}
    />
  )
}
