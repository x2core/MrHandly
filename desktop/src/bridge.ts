// The bridge is the single seam between the webview and the Rust core. In the
// Tauri app it invokes commands and listens to events; in a plain browser (dev
// / screenshots / CI build) it falls back to an in-memory mock that synthesises
// a small fleet so the UI can be developed without the desktop runtime.
//
// The webview NEVER opens a socket itself — either the Rust core does, or the
// mock fabricates data (CLAUDE.md §3).

import type {
  Container,
  Image,
  LogRecord,
  MetricsEvent,
  Metrics,
  Peer,
  Process,
  Service,
  StatusEvent,
} from './types'

export type ViewKind = 'processes' | 'services'
export type LogKind = 'service' | 'container'

export interface Bridge {
  listPeers(): Promise<Peer[]>
  addPeer(name: string, address: string, port: number): Promise<void>
  removePeer(address: string): Promise<void>
  renamePeer(address: string, name: string): Promise<void>
  onPeers(cb: (peers: Peer[]) => void): () => void
  onStatus(cb: (e: StatusEvent) => void): () => void
  onMetrics(cb: (e: MetricsEvent) => void): () => void

  // M5 views.
  fetchServices(address: string): Promise<Service[]>
  fetchContainers(address: string): Promise<Container[]>
  fetchImages(address: string): Promise<Image[]>
  serviceAction(address: string, unit: string, action: string): Promise<void>
  containerAction(address: string, id: string, action: string): Promise<void>
  subscribeProcesses(address: string, cb: (items: Process[]) => void): () => void
  subscribeServices(address: string, cb: (items: Service[]) => void): () => void
  subscribeLogs(address: string, kind: LogKind, target: string, cb: (line: LogRecord) => void): () => void

  // SSH terminal — a direct SSH session from this app, NOT an agent feature.
  sshConnect(address: string, req: SshConnect): Promise<SshResult>
  sshExec(address: string, line: string): Promise<{ output: string; exit: number }>
  sshDisconnect(address: string): Promise<void>
}

export interface SshConnect {
  username: string
  port: number
  auth: 'key' | 'password'
  /** Password or key passphrase — sent to the backend, never stored. */
  secret: string
  keyPath?: string
}

export interface SshResult {
  ok: boolean
  motd?: string
  error?: string
}

const inTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window

// ---------------------------------------------------------------------------
// Tauri bridge
// ---------------------------------------------------------------------------

function tauriBridge(): Bridge {
  // Imported lazily so the mock path never pulls the Tauri API in a browser.
  const core = () => import('@tauri-apps/api/core')
  const event = () => import('@tauri-apps/api/event')

  const listen = <T>(name: string, cb: (p: T) => void): (() => void) => {
    let un: (() => void) | undefined
    void event().then(({ listen }) => listen<T>(name, (e) => cb(e.payload)).then((u) => (un = u)))
    return () => un?.()
  }

  return {
    async listPeers() {
      return (await core()).invoke<Peer[]>('list_peers')
    },
    async addPeer(name, address, port) {
      await (await core()).invoke('add_peer', { name, address, port })
    },
    async removePeer(address) {
      await (await core()).invoke('remove_peer', { address })
    },
    async renamePeer(address, name) {
      await (await core()).invoke('rename_peer', { address, name })
    },
    onPeers: (cb) => listen<Peer[]>('peers:changed', cb),
    onStatus: (cb) => listen<StatusEvent>('peer:status', cb),
    onMetrics: (cb) => listen<MetricsEvent>('peer:metrics', cb),

    // Grab a single services snapshot via the stream, then unsubscribe.
    fetchServices(address) {
      return new Promise<Service[]>((resolve) => {
        let off = () => {}
        let done = false
        off = this.subscribeServices(address, (items) => {
          if (done) return
          done = true
          resolve(items)
          off()
        })
        setTimeout(() => {
          if (!done) {
            done = true
            resolve([])
            off()
          }
        }, 4000)
      })
    },
    async fetchContainers(address) {
      return (await core()).invoke<Container[]>('fetch_containers', { address })
    },
    async fetchImages(address) {
      return (await core()).invoke<Image[]>('fetch_images', { address })
    },
    async serviceAction(address, unit, action) {
      await (await core()).invoke('service_action', { address, unit, action })
    },
    async containerAction(address, id, action) {
      await (await core()).invoke('container_action', { address, id, action })
    },
    subscribeProcesses(address, cb) {
      void core().then(({ invoke }) => invoke('subscribe_view', { address, kind: 'processes' }))
      const off = listen<{ address: string; items: Process[] }>('view:processes', (e) => {
        if (e.address === address) cb(e.items)
      })
      return () => {
        off()
        void core().then(({ invoke }) => invoke('unsubscribe_view', { address, kind: 'processes' }))
      }
    },
    subscribeServices(address, cb) {
      void core().then(({ invoke }) => invoke('subscribe_view', { address, kind: 'services' }))
      const off = listen<{ address: string; items: Service[] }>('view:services', (e) => {
        if (e.address === address) cb(e.items)
      })
      return () => {
        off()
        void core().then(({ invoke }) => invoke('unsubscribe_view', { address, kind: 'services' }))
      }
    },
    subscribeLogs(address, kind, target, cb) {
      void core().then(({ invoke }) => invoke('subscribe_logs', { address, kind, target }))
      const off = listen<{ address: string; line: Record<string, unknown> }>('peer:log', (e) => {
        if (e.address === address) cb(mapLog(e.line))
      })
      return () => {
        off()
        void core().then(({ invoke }) => invoke('unsubscribe_logs', { address }))
      }
    },
    async sshConnect(address, req) {
      return (await core()).invoke<SshResult>('ssh_connect', { address, ...req })
    },
    async sshExec(address, line) {
      return (await core()).invoke<{ output: string; exit: number }>('ssh_exec', { address, line })
    },
    async sshDisconnect(address) {
      await (await core()).invoke('ssh_disconnect', { address })
    },
  }
}

// mapLog normalises a service LogLine or container ContainerLog into LogRecord.
function mapLog(raw: Record<string, unknown>): LogRecord {
  if ('stream' in raw) {
    return {
      timestamp: Number(raw.timestamp ?? 0),
      message: String(raw.message ?? ''),
      priority: -1,
      source: String(raw.stream ?? 'stdout'),
    }
  }
  return {
    timestamp: Number(raw.timestamp ?? 0),
    message: String(raw.message ?? ''),
    priority: Number(raw.priority ?? 6),
    source: String(raw.unit ?? ''),
  }
}

// ---------------------------------------------------------------------------
// Mock bridge (browser dev / build)
// ---------------------------------------------------------------------------

function mockBridge(): Bridge {
  let peers: Peer[] = [
    { name: 'lab-01', address: '10.44.0.2', port: 8443 },
    { name: 'lab-02', address: '10.44.0.3', port: 8443 },
    { name: 'edge-gw', address: '10.44.0.4', port: 8443 },
    { name: 'nas', address: '10.44.0.5', port: 8443 },
  ]
  const peersCbs = new Set<(p: Peer[]) => void>()
  const statusCbs = new Set<(e: StatusEvent) => void>()
  const metricsCbs = new Set<(e: MetricsEvent) => void>()

  const emitPeers = () => peersCbs.forEach((cb) => cb([...peers]))

  // Synthetic per-host state so the meters move like a real fleet.
  const sim: Record<string, { rx: number; tx: number; base: number; online: boolean; docker: boolean }> = {}
  const seed = (p: Peer, i: number) => {
    sim[p.address] = { rx: 1e9, tx: 5e8, base: 0.12 + i * 0.08, online: p.address !== '10.44.0.5', docker: i % 2 === 0 }
  }
  peers.forEach(seed)

  let t = 0
  setInterval(() => {
    t += 1
    for (const p of peers) {
      const s = sim[p.address]
      if (!s) continue
      if (!s.online) {
        statusCbs.forEach((cb) => cb({ address: p.address, online: false, info: null }))
        continue
      }
      statusCbs.forEach((cb) =>
        cb({
          address: p.address,
          online: true,
          info: {
            protocol: 1,
            agent: { version: 'v0.1.0', commit: 'devbuild' },
            host: {
              hostname: p.name,
              kernel: '6.1.0-18-amd64',
              distro: 'Debian GNU/Linux 12',
              arch: 'amd64',
              cpus: 4,
              total_memory: 16 * 1024 ** 3,
              boot_time: 1700000000,
            },
            capabilities: { systemd: true, docker: s.docker },
          },
        }),
      )
      const usage = clamp01(s.base + 0.1 * Math.sin(t / 7 + s.base * 10) + 0.05 * Math.random())
      const memUsed = (0.4 + 0.2 * Math.sin(t / 13 + s.base)) * 16 * 1024 ** 3
      s.rx += 4e5 + 3e5 * Math.random()
      s.tx += 2e5 + 2e5 * Math.random()
      const m: Metrics = {
        timestamp: Date.now(),
        cpu: { usage, per_core: [0, 1, 2, 3].map(() => clamp01(usage + (Math.random() - 0.5) * 0.2)) },
        memory: {
          total: 16 * 1024 ** 3,
          used: memUsed,
          free: 16 * 1024 ** 3 - memUsed,
          available: 16 * 1024 ** 3 - memUsed,
          buffers: 2e8,
          cached: 4e9,
          swap_total: 0,
          swap_used: 0,
        },
        load: { one: usage * 4, five: usage * 3.5, fifteen: usage * 3 },
        uptime_seconds: 1_500_000 + t,
        network: { eth0: { rx_bytes: s.rx, tx_bytes: s.tx, rx_packets: t * 100, tx_packets: t * 80 } },
        disk: { nvme0n1: { reads: t * 10, writes: t * 6, read_sectors: t * 80, write_sectors: t * 40 } },
      }
      metricsCbs.forEach((cb) => cb({ address: p.address, metrics: m }))
    }
  }, 1000)

  return {
    async listPeers() {
      return [...peers]
    },
    async addPeer(name, address, port) {
      if (!peers.some((p) => p.address === address)) {
        const p = { name, address, port }
        peers = [...peers, p]
        seed(p, peers.length)
        emitPeers()
      }
    },
    async removePeer(address) {
      peers = peers.filter((p) => p.address !== address)
      delete sim[address]
      emitPeers()
    },
    async renamePeer(address, name) {
      peers = peers.map((p) => (p.address === address ? { ...p, name } : p))
      emitPeers()
    },
    onPeers: (cb) => {
      peersCbs.add(cb)
      return () => peersCbs.delete(cb)
    },
    onStatus: (cb) => {
      statusCbs.add(cb)
      return () => statusCbs.delete(cb)
    },
    onMetrics: (cb) => {
      metricsCbs.add(cb)
      return () => metricsCbs.delete(cb)
    },

    async fetchServices() {
      return mockServices()
    },
    async fetchContainers() {
      return mockContainers()
    },
    async fetchImages() {
      return mockImages()
    },
    async serviceAction() {
      /* no-op in demo */
    },
    async containerAction() {
      /* no-op in demo */
    },
    subscribeProcesses(_address, cb) {
      const tick = () => cb(mockProcesses())
      tick()
      const h = setInterval(tick, 2000)
      return () => clearInterval(h)
    },
    subscribeServices(_address, cb) {
      cb(mockServices())
      const h = setInterval(() => cb(mockServices()), 3000)
      return () => clearInterval(h)
    },
    subscribeLogs(_address, kind, target, cb) {
      let n = 0
      const h = setInterval(() => {
        n++
        cb({
          timestamp: Date.now(),
          message:
            kind === 'container'
              ? `${target.slice(0, 8)} handled request ${n} in ${(2 + Math.random() * 40).toFixed(0)}ms`
              : `${target}: worker ${n % 4} processed job ${n}`,
          priority: kind === 'container' ? -1 : n % 11 === 0 ? 3 : 6,
          source: kind === 'container' ? (n % 5 === 0 ? 'stderr' : 'stdout') : target,
        })
      }, 700)
      return () => clearInterval(h)
    },
    async sshConnect(_address, req) {
      await new Promise((r) => setTimeout(r, 500)) // simulate handshake
      if (!req.username.trim()) return { ok: false, error: 'username required' }
      if (req.auth === 'password' && !req.secret) return { ok: false, error: 'password required' }
      const motd = [
        'Linux lab-01 6.1.0-18-amd64 #1 SMP Debian 6.1.76-1 x86_64 GNU/Linux',
        '',
        'The programs included with the Debian GNU/Linux system are free software.',
        `Last login: ${new Date().toUTCString()} from 10.44.0.1`,
      ].join('\n')
      return { ok: true, motd }
    },
    async sshExec(_address, line) {
      await new Promise((r) => setTimeout(r, 40 + Math.random() * 120))
      return mockShell(line)
    },
    async sshDisconnect() {
      /* no-op in demo */
    },
  }
}

// A small canned shell for the demo terminal. In the real app these round-trips
// are a live SSH PTY; here they just make the UI feel real.
function mockShell(line: string): { output: string; exit: number } {
  const cmd = line.trim()
  const [head, ...rest] = cmd.split(/\s+/)
  const arg = rest.join(' ')
  switch (head) {
    case '':
      return { output: '', exit: 0 }
    case 'help':
      return { output: 'available: ls, pwd, whoami, id, uname, uptime, df, free, ps, cat, echo, date, clear, exit', exit: 0 }
    case 'whoami':
      return { output: 'oikos', exit: 0 }
    case 'id':
      return { output: 'uid=1000(oikos) gid=1000(oikos) groups=1000(oikos),27(sudo),999(docker)', exit: 0 }
    case 'pwd':
      return { output: '/home/oikos', exit: 0 }
    case 'ls':
      return {
        output: rest.includes('-la')
          ? 'total 28\ndrwxr-xr-x 4 oikos oikos 4096 Jul 17 09:14 .\ndrwxr-xr-x 3 root  root  4096 Jul 10 11:02 ..\n-rw-r--r-- 1 oikos oikos  220 Jul 10 11:02 .bash_logout\n-rw-r--r-- 1 oikos oikos 3771 Jul 10 11:02 .bashrc\ndrwxr-xr-x 2 oikos oikos 4096 Jul 17 09:14 deploy\n-rwxr-xr-x 1 oikos oikos  512 Jul 16 21:40 run.sh'
          : 'deploy  run.sh',
        exit: 0,
      }
    case 'uname':
      return { output: rest.includes('-a') ? 'Linux lab-01 6.1.0-18-amd64 #1 SMP Debian x86_64 GNU/Linux' : 'Linux', exit: 0 }
    case 'uptime':
      return { output: ' 09:14:22 up 17 days,  8:03,  1 user,  load average: 0.98, 0.58, 0.48', exit: 0 }
    case 'df':
      return {
        output:
          'Filesystem      Size  Used Avail Use% Mounted on\n/dev/nvme0n1p2  228G   74G  143G  35% /\ntmpfs           7.9G     0  7.9G   0% /dev/shm',
        exit: 0,
      }
    case 'free':
      return {
        output:
          '               total        used        free      shared  buff/cache   available\nMem:            15Gi       3.1Gi       9.8Gi       210Mi       2.6Gi        12Gi\nSwap:          2.0Gi          0B       2.0Gi',
        exit: 0,
      }
    case 'ps':
      return { output: 'PID   USER   %CPU  COMMAND\n198   root    2.1  /usr/sbin/sshd -D\n275   oikos   1.4  node /srv/app/index.js\n289   redis   0.9  redis-server *:6379', exit: 0 }
    case 'cat':
      if (arg.includes('os-release'))
        return { output: 'PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"\nNAME="Debian GNU/Linux"\nVERSION_ID="12"\nID=debian', exit: 0 }
      return { output: `cat: ${arg}: No such file or directory`, exit: 1 }
    case 'echo':
      return { output: arg, exit: 0 }
    case 'date':
      return { output: new Date().toUTCString(), exit: 0 }
    case 'sudo':
      return { output: `[sudo] password for oikos: \noikos is not in the sudoers file. This incident will be reported.`, exit: 1 }
    default:
      return { output: `bash: ${head}: command not found`, exit: 127 }
  }
}

// --- mock M5 data ---------------------------------------------------------

const PROC_NAMES = [
  '/usr/sbin/nginx: worker',
  'postgres: checkpointer',
  '/usr/bin/dockerd',
  'systemd --user',
  '/usr/sbin/sshd -D',
  'node /srv/app/index.js',
  '/usr/bin/containerd',
  'redis-server *:6379',
  '/lib/systemd/systemd-journald',
  'python3 /opt/worker.py',
]

function mockProcesses(): Process[] {
  const procs: Process[] = []
  for (let i = 0; i < 42; i++) {
    procs.push({
      pid: 100 + i * 7,
      ppid: i < 3 ? 1 : 100 + Math.floor(Math.random() * i) * 7,
      name: PROC_NAMES[i % PROC_NAMES.length]!,
      state: Math.random() > 0.7 ? 'S' : Math.random() > 0.5 ? 'R' : 'S',
      cpu: Math.max(0, Math.random() ** 3),
      rss: Math.floor((5 + Math.random() * 500) * 1024 * 1024),
      threads: 1 + Math.floor(Math.random() * 12),
    })
  }
  return procs.sort((a, b) => b.cpu - a.cpu)
}

function mockServices(): Service[] {
  const base = [
    ['nginx.service', 'A high performance web server', 'active', 'running', true],
    ['ssh.service', 'OpenBSD Secure Shell server', 'active', 'running', false],
    ['docker.service', 'Docker Application Container Engine', 'active', 'running', true],
    ['postgresql.service', 'PostgreSQL RDBMS', 'active', 'running', false],
    ['cron.service', 'Regular background program processing', 'active', 'running', false],
    ['fail2ban.service', 'Fail2Ban Service', Math.random() > 0.85 ? 'failed' : 'active', 'running', false],
  ] as const
  return base.map(([name, description, active_state, sub_state, writable]) => ({
    name,
    description,
    load_state: 'loaded',
    active_state,
    sub_state: active_state === 'failed' ? 'failed' : sub_state,
    writable,
  }))
}

function mockContainers(): Container[] {
  return [
    { id: 'abc123def456', name: 'web', image: 'nginx:latest', state: 'running', status: 'Up 3 hours', created: 1700000000, writable: true },
    { id: '789ghi012jkl', name: 'db', image: 'postgres:16', state: 'running', status: 'Up 3 hours', created: 1699990000, writable: true },
    { id: 'mno345pqr678', name: 'cache', image: 'redis:7', state: 'exited', status: 'Exited (0) 10 minutes ago', created: 1699980000, writable: true },
  ]
}

function mockImages(): Image[] {
  return [
    { id: 'sha256:aaaa', tags: ['nginx:latest'], size: 142_000_000, created: 1699000000 },
    { id: 'sha256:bbbb', tags: ['postgres:16'], size: 379_000_000, created: 1698000000 },
    { id: 'sha256:cccc', tags: ['redis:7'], size: 41_000_000, created: 1697000000 },
    { id: 'sha256:dddd', tags: [], size: 8_000_000, created: 1696000000 },
  ]
}

function clamp01(v: number): number {
  return Math.max(0, Math.min(1, v))
}

export const bridge: Bridge = inTauri ? tauriBridge() : mockBridge()
export const isMock = !inTauri
