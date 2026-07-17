// SSH terminal — a direct shell to the host over the operator's own SSH access.
// This is NOT the Oikos agent: the agent has no exec endpoint by design
// (CLAUDE.md §4.4). The terminal only opens once SSH credentials are added;
// until then it shows a connect card.

import { useEffect, useRef, useState } from 'react'
import { bridge } from '../bridge'
import { useSsh, type SshAuth } from '../ssh'
import { IconCommand } from './icons'

interface Line {
  text: string
  kind: 'in' | 'out' | 'err' | 'sys'
}

export function Terminal({ address, hostName }: { address: string; hostName: string }) {
  const creds = useSsh((s) => s.creds[address])
  const connected = useSsh((s) => s.connected[address])

  if (!creds || !connected) {
    return <ConnectCard address={address} hostName={hostName} />
  }
  return <Shell address={address} hostName={hostName} />
}

// ── Connect card ──────────────────────────────────────────────────────────

function ConnectCard({ address, hostName }: { address: string; hostName: string }) {
  const existing = useSsh((s) => s.creds[address])
  const setCreds = useSsh((s) => s.setCreds)
  const setConnected = useSsh((s) => s.setConnected)

  const [username, setUsername] = useState(existing?.username ?? '')
  const [port, setPort] = useState(existing?.port ?? 22)
  const [auth, setAuth] = useState<SshAuth>(existing?.auth ?? 'key')
  const [keyPath, setKeyPath] = useState(existing?.keyPath ?? '~/.ssh/id_ed25519')
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const connect = async () => {
    setBusy(true)
    setError(null)
    const res = await bridge.sshConnect(address, { username: username.trim(), port, auth, secret, keyPath })
    setBusy(false)
    if (!res.ok) {
      setError(res.error ?? 'connection failed')
      return
    }
    setCreds(address, { username: username.trim(), port, auth, keyPath: auth === 'key' ? keyPath : undefined })
    setSecret('')
    setConnected(address, true)
  }

  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="w-[420px] rounded border border-rule bg-rack shadow-card">
        <div className="u-pad flex items-center gap-2 border-b border-rule">
          <span className="grid h-7 w-7 place-items-center rounded bg-signal/15 text-signal">
            <IconCommand />
          </span>
          <div className="leading-tight">
            <div className="font-plate text-sm font-semibold uppercase tracking-wider text-readout">SSH Session</div>
            <div className="plate text-[9px]">
              connect a shell to {hostName} · {address}
            </div>
          </div>
        </div>

        <div className="u-pad flex flex-col gap-3">
          <div className="grid grid-cols-[1fr_88px] gap-2">
            <Field label="Username">
              <input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="oikos" className={inp} autoFocus />
            </Field>
            <Field label="Port">
              <input type="number" value={port} onChange={(e) => setPort(Number(e.target.value) || 22)} className={inp} />
            </Field>
          </div>

          <div className="flex gap-2">
            <Toggle active={auth === 'key'} onClick={() => setAuth('key')} label="Key" />
            <Toggle active={auth === 'password'} onClick={() => setAuth('password')} label="Password" />
          </div>

          {auth === 'key' ? (
            <Field label="Private key">
              <input value={keyPath} onChange={(e) => setKeyPath(e.target.value)} className={inp} placeholder="~/.ssh/id_ed25519" />
            </Field>
          ) : null}

          <Field label={auth === 'key' ? 'Key passphrase (if any)' : 'Password'}>
            <input type="password" value={secret} onChange={(e) => setSecret(e.target.value)} className={inp} placeholder="••••••••" />
          </Field>

          {error && <p className="readout text-xs text-fault">{error}</p>}

          <button
            onClick={connect}
            disabled={busy}
            className="mt-1 rounded border border-signal bg-signal/10 py-2 font-plate text-xs uppercase tracking-wider text-signal transition-colors duration-led hover:bg-signal/20 disabled:opacity-50"
          >
            {busy ? 'Connecting…' : 'Connect'}
          </button>

          <p className="plate text-[9px] normal-case leading-snug text-plate">
            Uses your own SSH access — a direct session, not the Oikos agent. The agent has no shell.
          </p>
        </div>
      </div>
    </div>
  )
}

// ── Shell ─────────────────────────────────────────────────────────────────

function Shell({ address, hostName }: { address: string; hostName: string }) {
  const creds = useSsh((s) => s.creds[address])!
  const setConnected = useSsh((s) => s.setConnected)
  const forget = useSsh((s) => s.forget)

  const prompt = `${creds.username}@${hostName}:~$`
  const [lines, setLines] = useState<Line[]>([
    { kind: 'sys', text: `Connected to ${hostName} (${address}:${creds.port}) via SSH.` },
    { kind: 'sys', text: `Type 'help' for demo commands, 'exit' to disconnect.` },
    { kind: 'out', text: '' },
  ])
  const [input, setInput] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [hIdx, setHIdx] = useState(-1)
  const [busy, setBusy] = useState(false)

  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [lines, busy])

  const push = (...ls: Line[]) => setLines((prev) => [...prev, ...ls])

  const run = async (raw: string) => {
    const line = raw
    push({ kind: 'in', text: `${prompt} ${line}` })
    if (line.trim()) setHistory((h) => [...h, line])
    setHIdx(-1)
    setInput('')

    const cmd = line.trim()
    if (cmd === 'clear') {
      setLines([])
      return
    }
    if (cmd === 'exit' || cmd === 'logout') {
      setConnected(address, false)
      return
    }
    setBusy(true)
    const { output, exit } = await bridge.sshExec(address, line)
    setBusy(false)
    if (output) push({ kind: exit === 0 ? 'out' : 'err', text: output })
  }

  const onKey = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      void run(input)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (!history.length) return
      const i = hIdx < 0 ? history.length - 1 : Math.max(0, hIdx - 1)
      setHIdx(i)
      setInput(history[i] ?? '')
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (hIdx < 0) return
      const i = hIdx + 1
      if (i >= history.length) {
        setHIdx(-1)
        setInput('')
      } else {
        setHIdx(i)
        setInput(history[i] ?? '')
      }
    } else if (e.key === 'l' && e.ctrlKey) {
      e.preventDefault()
      setLines([])
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Title bar */}
      <div className="u-pad-x flex h-9 shrink-0 items-center gap-2 border-b border-rule bg-rack">
        <span className="h-[7px] w-[7px] rounded-full bg-signal shadow-[var(--glow)_rgb(var(--c-signal))]" />
        <span className="readout text-xs text-readout">
          {creds.username}@{hostName}
        </span>
        <span className="plate">ssh · {address}:{creds.port}</span>
        <span className="plate ml-2 text-[9px]">{Math.round(8 + Math.random() * 20)}ms</span>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={() => setLines([])} className={barBtn}>
            Clear
          </button>
          <button onClick={() => setConnected(address, false)} className={barBtn}>
            Disconnect
          </button>
          <button onClick={() => forget(address)} className={`${barBtn} hover:border-fault hover:text-fault`}>
            Forget
          </button>
        </div>
      </div>

      {/* Scrollback */}
      <div
        ref={scrollRef}
        onClick={() => inputRef.current?.focus()}
        className="min-h-0 flex-1 overflow-y-auto bg-panel px-3 py-2 font-mono text-[12px] leading-[18px]"
      >
        {lines.map((l, i) => (
          <div
            key={i}
            className={[
              'whitespace-pre-wrap break-words',
              l.kind === 'in' ? 'text-readout' : l.kind === 'err' ? 'text-fault' : l.kind === 'sys' ? 'text-plate' : 'text-readout/90',
            ].join(' ')}
          >
            {l.text}
          </div>
        ))}

        {/* Live input line */}
        <div className="flex items-center">
          <span className="shrink-0 text-signal">{prompt}&nbsp;</span>
          <input
            ref={inputRef}
            autoFocus
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKey}
            spellCheck={false}
            autoComplete="off"
            className="min-w-0 flex-1 bg-transparent text-readout outline-none"
            style={{ caretColor: 'rgb(var(--c-signal))' }}
          />
          {busy && <span className="cursor-blink ml-1 h-[14px] w-[7px] shrink-0 bg-signal/70" />}
        </div>
      </div>
    </div>
  )
}

// ── bits ──────────────────────────────────────────────────────────────────

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="plate">{label}</span>
      {children}
    </label>
  )
}

function Toggle({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={[
        'flex-1 rounded border py-1.5 font-plate text-[10px] uppercase tracking-wider transition-colors duration-led',
        active ? 'border-signal bg-signal/10 text-signal' : 'border-rule text-plate hover:text-readout',
      ].join(' ')}
    >
      {label}
    </button>
  )
}

const inp =
  'w-full rounded border border-rule bg-panel px-2 py-1.5 font-mono text-sm text-readout tabular-nums outline-none focus:border-signal'
const barBtn =
  'rounded border border-rule px-2 py-1 font-plate text-[9px] uppercase tracking-wider text-plate transition-colors duration-led hover:text-readout'
