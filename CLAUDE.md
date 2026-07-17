# CLAUDE.md

Project context for Claude Code. Read this before touching anything.

---

## 1. What this is

**Oikos** — a personal control panel for a small fleet of Linux machines, reachable over an
existing WireGuard mesh.

> *Name is a placeholder; `oikos` = household estate management. Swap freely before first
> public tag, it appears in binary names, unit names and config paths.*

A lightweight **agent** runs on each guest machine and exposes a read-mostly HTTP API over
the WireGuard interface. A **Tauri desktop app** on the operator's machine talks to all
agents directly and renders a fleet view: a Windows Task Manager for a whole fleet, plus a
minimal Docker UI where Docker is present.

The problem it solves: not having to open the Proxmox noVNC console and write bash to see
what a machine is doing.

### What this is NOT

Say no to these. They have been considered and explicitly rejected:

- **Not an orchestrator.** No scheduling, no placement, no bin-packing, no "give me a
  Postgres somewhere." The operator already knows which machine they mean.
- **Not a PaaS / not Nomad / not Kubernetes.** Those were evaluated and cut.
- **Not coupled to Proxmox.** Proxmox does not appear in a single line of code. Whether a
  host is bare metal or a VM is invisible and irrelevant.
- **Not multi-tenant.** One operator. No user accounts, no RBAC, no billing.
- **Not production infrastructure.** Personal tooling and internal use.

---

## 2. Core theses

These are load-bearing. If a change violates one, the change is wrong.

**Projection, not ownership.** The agent owns no state. It is a live projection of what
already exists on the host (systemd, Docker, procfs). Ask and it answers. No database, no
reconciliation loop, no desired-state, no drift. The moment the agent starts *owning*
something, this becomes the product we rejected.

**Mechanism, not policy.** The agent exposes capabilities. It does not decide anything.

**Consume the transport, don't own it.** WireGuard is assumed to exist. The agent does not
install, configure or manage `wg0`. It binds to it.

**The guest stays clean.** No toolchain, no runtime, no package manager churn on guests. One
static binary and one unit file. That is the entire footprint.

---

## 3. Architecture

```
┌──────────────────────────────┐
│  Desktop app (Tauri)         │   operator's machine
│  Rust core + React UI        │
│  peers.toml  ·  fan-in       │
└──────────────┬───────────────┘
               │  HTTP + SSE over wg0
   ┌───────────┼───────────┬───────────┐
   ▼           ▼           ▼           ▼
┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐
│agent │   │agent │   │agent │   │agent │   Debian guests
└──┬───┘   └──────┘   └──────┘   └──────┘
   │
   ├─ procfs  (/proc, /sys)          → metrics, processes
   ├─ D-Bus   (org.freedesktop.systemd1) → services
   ├─ journalctl -o json             → logs
   └─ /var/run/docker.sock (HTTP)    → containers
```

There is **no server node**. The desktop app is the only aggregation point, and it is only a
multiplexer — it holds the peer list and fans requests out. It has no business logic.

**Known consequence, accepted:** no always-on component means no metric history, no scheduled
tasks, no alerting. Close the laptop and nobody is watching. This is fine for a personal tool
and is the one door deliberately left shut.

---

## 4. Hard invariants

Violating any of these is a bug, not a tradeoff. The agent is a **root-equivalent daemon that
parses network input** — treat it accordingly.

1. **The agent binds only to the WireGuard interface.** Never `0.0.0.0`. On startup it
   resolves the configured interface, and **refuses to start** if the resolved address is
   unspecified, loopback-only, or outside the configured WG subnet. This is the single most
   important line of code in the project.
2. **Peer allowlist.** Only source IPs in the configured allowlist are served. WireGuard's
   cryptokey routing makes the source IP inside the tunnel a real credential — a peer cannot
   forge an in-tunnel IP without the private key. Because of this, **no mTLS on top**: it
   would be encryption over encryption.
3. **Unit allowlist.** Write actions (`start`/`stop`/`restart`) only apply to units matching
   configured globs. Read scope may be wider than write scope. Blast radius is bounded by
   config, not by hope.
4. **No `exec` endpoint.** Not in v1. A generic exec hole is not a feature, it is the end of
   the security model.
5. **`CGO_ENABLED=0`, always.** Pure-Go D-Bus (`godbus`), never `libsystemd` via cgo. cgo
   couples the binary to the host's glibc/libsystemd version and destroys static linking,
   which destroys the install story.
6. **No compiled binaries in git.** Ever. CI builds, Releases publishes, checksums verify.
7. **Every write action is audit-logged** to the agent's own journal: actor peer IP, action,
   target, result.

---

## 5. Repo layout

```
/agent                     Go. The only thing that runs on guests.
  cmd/oikos-agent/
  internal/
    config/                config load + validation (incl. bind guard)
    fingerprint/           host identity + capability detection
    procfs/                /proc + /sys readers   (root path injectable)
    systemd/               D-Bus client            (interface, fakeable)
    docker/                unix-socket HTTP client (socket path injectable)
    journal/               journalctl -o json streaming
    sampler/               single-source metric sampling + fan-out
    api/                   HTTP handlers, SSE
    audit/
  testdata/                fixture /proc trees, recorded Docker + D-Bus payloads
/desktop                   Tauri app. Operator machine only.
  src-tauri/               Rust core: peers.toml, HTTP/SSE client, fan-out to webview
  src/                     React + TS
/packages/protocol         Shared API types. Source of truth for both sides.
/.github/workflows/
/docs/
```

### Testability requirement

The agent must be fully testable **without** systemd, Docker, or root:

- `procfs.New(root string)` → tests point at `testdata/proc/...`
- `systemd.Conn` is an interface → fake in tests
- Docker socket path is config → tests use an `httptest` server on a temp unix socket

This is not optional polish. Claude Code sessions run in a cloud sandbox with no systemd and
no Docker; if the agent can only be tested on a real Debian box, most of this project cannot
be developed there. **Design for fixtures first.**

---

## 6. Stack decisions (settled — do not relitigate)

| Component | Choice | Why |
|---|---|---|
| Agent language | **Go** | Static binary, stdlib HTTP+JSON, goroutines for N live streams, memory-safe. |
| Agent deps | `godbus/dbus`, `coreos/go-systemd` | Pure Go. Nothing else without a strong reason. |
| Docker access | **Raw unix socket + `net/http`** | `docker.sock` speaks plain HTTP/1.1. The Docker SDK drags in half of Moby for nothing. Pin the API version in the path (`/v1.43/...`). |
| Desktop | **Tauri** (Rust core + React) | Small, no bundled Chromium, Rust core can hold the HTTP client and peer list. |
| UI kit | **shadcn/ui** | Copy-in components: we own the code, no runtime dep on a component vendor. Radix underneath = real a11y and keyboard behaviour. |
| Tables | **TanStack Table v8** + TanStack Virtual | The "fancy stable tables" requirement. Sorting/filtering/virtualization for long process lists. |
| Charts | **uPlot** | ~40KB, canvas, built for streaming time series. Recharts re-renders SVG per tick and dies at 1 Hz × N hosts. |
| Command palette | **cmdk** | See §7. |
| Builds | **GitHub Actions** | Agent via goreleaser; desktop via tauri-action. Download artifacts from Releases. |

**Rejected, with reasons** (so they don't come back):

- **C for the agent** — the agent is 80% network server, 20% syscalls. libc gives you no
  HTTP, no JSON, no TLS, no concurrency primitives; you'd add libmicrohttpd + jansson +
  libcurl and end up with *more* dependencies and a dynamically-linked binary. Meanwhile a
  root daemon parsing network strings in C is the worst square on the board.
- **Rust for the agent** — legitimate (musl static, no GC), but async Rust costs development
  time this tool doesn't have to spend. Revisit only if the Go binary's footprint becomes a
  real problem, not an aesthetic one.
- **A server node / Node.js orchestrator** — collapsed into the desktop app. The operator's
  machine is already on the mesh; a third node earned nothing.
- **Postgres / any DB** — the only real state is the peer list. It's a TOML file.

---

## 7. UI design direction

The register is **rack-mounted instrument**, not admin dashboard. Reference world: studio
console, patch bay, rack unit — fixed-height horizontal strips, a silkscreened label plate on
the left, meters in the middle, status LEDs on the right, everything on a grid.

Explicitly **not**: near-black with an acid accent, cards with generous padding, decorative
gradients, hero numbers with small labels. This is a dense tool for one power user.

**Palette** — warm graphite, the colour of anodized rack gear. Never pure black.

| Token | Hex | Use |
|---|---|---|
| `panel` | `#1B1A18` | app background |
| `rack` | `#232220` | strip / row surface |
| `rule` | `#35332F` | 1px hairlines, the only separator device |
| `plate` | `#8A8578` | silkscreen label text |
| `readout` | `#EDEAE3` | primary text and numbers |
| `signal` | `#E8A33D` | **meter fill and running LED only** — never text, never decoration |
| `fault` | `#D2453C` | failed state only |

Colour carries state and nothing else. If a colour is decorative, delete it.

**Type**

- Label plates: **IBM Plex Sans Condensed**, uppercase, ~10px, wide tracking. Condensed is
  the point — it's what a real silkscreen label looks like.
- Body/UI: IBM Plex Sans.
- All numbers, PIDs, unit names, container IDs: **IBM Plex Mono** with
  `font-variant-numeric: tabular-nums`. Non-negotiable — at 1 Hz, proportional figures
  jitter and the whole panel reads as broken.

**Signature element — the Fleet Strip.** A persistent bank at the top: one fixed-height row
per host, always visible, never scrolled away. Left: name plate. Middle: live uPlot
sparklines (CPU / RAM / net). Right: an LED cluster (agent up, docker present, failed units).
It reads as a mixing desk of machines. Click a row to focus it below. This is the thing
Proxmox never gives you: the whole fleet, at a glance, always.

**Motion:** essentially none. Meters move because data moves. LED state changes get a 120ms
crossfade. No page transitions, no entrance animations. A panel that animates at 1 Hz is
unusable.

**Density:** 28–32px rows, 2px max radius, 1px hairlines. Respect `prefers-reduced-motion`,
keep focus rings visible.

**Command palette (⌘K)** is a first-class surface, not a shortcut. `restart nginx on lab-02`
without leaving the keyboard. This is the original complaint that started the project — the
default Proxmox UI makes every task a form to fill instead of a workflow to move through.
The palette is the answer to that, so it gets designed, not bolted on.

**Copy:** name things by what the operator controls. Active voice. A button that says
"Restart" produces a toast that says "Restarted". An offline peer degrades to a labelled
disconnected state — never a blank panel, never a spinner forever.

---

## 8. Agent performance budget

Optimization is a stated goal for the agent. Targets on an idle host:

- **RSS < 20 MB**, CPU < 1% with one subscriber
- Cold start < 50ms

Rules that get you there:

- **Sample once, fan out.** One sampler goroutine per source at a fixed tick; N SSE
  subscribers read from a broadcast. Never parse `/proc` per subscriber.
- **Subscription-driven sampling.** Nothing is sampled if nobody is watching it. The process
  table (`/proc/<pid>/stat` × ~300) is the expensive read — it runs only while a client has
  the Processes view open, and at 2s, not 1s. CPU/mem aggregate runs at 1 Hz.
- **Parse bytes, not strings.** Reuse buffers, avoid `strings.Split` in hot paths, avoid
  per-tick allocation. Benchmark the `/proc/stat` and process-table paths and keep the
  benchmarks in CI.
- Set `GOMEMLIMIT` and a conservative `GOGC` in the unit file.
- SSE payloads: emit changed fields only.

Do not micro-optimize anything outside the sampler. The API handlers are cold.

---

## 9. Commands

```bash
# Agent
cd agent
make build            # CGO_ENABLED=0 go build
make test             # fixture-based, no root/systemd/docker needed
make bench            # sampler hot paths
make lint             # go vet + staticcheck

# Desktop
cd desktop
pnpm install
pnpm tauri dev
pnpm tauri build
```

Cross-compilation is the norm, not an exception: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`.
The guest never builds anything.

---

## 10. Conventions

- English for all code, comments, docs and commit messages.
- Conventional Commits. Tags: `agent-v*` and `desktop-v*` trigger separate release workflows.
- API types are defined once in `/packages/protocol` and generated/mirrored into Go. If the
  two drift, that's a bug.
- Errors from the agent are structured JSON with a stable `code`. The UI switches on `code`,
  never on message text.
- No new agent dependency without justifying it against §6. The dependency budget is the
  product.

---

## 11. Deferred to Preview 2

Do not build these now. Noted so they aren't designed out of reach:

- **MCP server** — a separate binary, not embedded in the desktop app. Both are clients of
  the same peer list; bundling is a distribution convenience, not coupling. Keep that seam.
- **Plan/diff/apply** — when Claude authors a unit file, the app shows a diff against what's
  on the host and the operator approves. Authorship reintroduces the one piece of state
  worth having: provenance. The cheap answer is a local git repo of generated artifacts —
  git holds the state, the host stays a projection.
- **Tool discipline** — narrow typed tools (`list_services`, `service_logs`, `docker_ps`),
  read tools free, write tools confirmed. Never a generic exec tool.
- **Push install from the app** — scp the binary + unit file over SSH, host appears in the
  UI. Until then: signed release + checksum-verifying `install.sh`.
- **Hardening pass** — polkit rules for a non-root agent user, systemd sandboxing directives
  on the agent's own unit.
