# Oikos — Roadmap, Preview 1

**Scope of Preview 1:** the agent and the desktop app. MCP is Preview 2 and is out of scope
here (see `CLAUDE.md` §11).

**Working model:** Claude Code in the cloud (`claude.ai/code`) has no systemd, no Docker and
no root. Everything in M1–M4 is therefore built fixture-first and tested against `testdata/`.
Real-host verification happens on the local machine at the end of each milestone, and CI
builds the artifacts to download. If a task can't be tested from a sandbox, that's a design
smell — fix the seam, don't skip the test.

**Milestone rule:** each milestone ends with something runnable and a tagged artifact you can
actually download. No milestone is "internal refactoring."

---

## M0 — Skeleton and the release pipeline

Do the boring thing first: prove that a tag produces a downloadable, checksummed binary
*before* there is anything worth downloading. Everything after this is easier.

**Tasks**

- Monorepo layout per `CLAUDE.md` §5. `agent/`, `desktop/`, `packages/protocol/`, `docs/`.
- `agent/`: Go module, `cmd/oikos-agent/main.go` that prints version and exits. Makefile with
  `build` / `test` / `lint` / `bench`. Version injected via `-ldflags -X`.
- `.github/workflows/ci.yml` — on PR: `go vet`, `staticcheck`, `go test ./...`, `tsc`,
  `eslint`, `cargo clippy`.
- `.github/workflows/agent-release.yml` — on tag `agent-v*`: goreleaser, matrix
  `linux/amd64` + `linux/arm64`, `CGO_ENABLED=0`, publish to Releases with
  `checksums.txt`.
- Add `actions/attest-build-provenance` to the release job. Go builds with cgo off are
  effectively reproducible; provenance is what makes a root-equivalent binary auditable
  rather than "trust me."
- `.gitignore` that makes committing a binary hard. `docs/SECURITY.md` stub.

**Done when:** `git tag agent-v0.0.1 && git push --tags` yields a Release with two binaries
and a checksums file, and `sha256sum -c` passes on a downloaded artifact.

---

## M1 — Agent core: identity, metrics, streaming, and the bind guard

The security spine goes in first. It is much harder to retrofit a bind guard than to start
with one.

**Tasks**

- `internal/config`: TOML config — `interface` (e.g. `wg0`), `subnet`, `peers[]`,
  `unit_allowlist[]`, `read_allowlist[]`, `sample_interval`. Validation is strict; unknown
  keys are errors.
- **Bind guard** (`config` + `api`): resolve the configured interface to an address; refuse
  to start if it is unspecified, loopback, or outside `subnet`. Log the refusal loudly. This
  gets a dedicated test table with a fake `net.Interface` lookup — inject the resolver.
- **Peer middleware**: reject any request whose `RemoteAddr` is not in `peers[]`. `403`,
  structured error, audit-logged.
- `internal/procfs` with injectable root: `/proc/stat`, `/proc/meminfo`, `/proc/net/dev`,
  `/proc/loadavg`, `/proc/uptime`, `/proc/diskstats`. Byte-level parsing, zero allocation in
  the hot path.
- `internal/fingerprint`: hostname, kernel, distro, CPU count, total memory, and capability
  detection — `systemd` present? `docker.sock` present and dialable? Cached at start, docker
  re-probed lazily.
- `internal/sampler`: one goroutine per source, fixed tick, broadcast to N subscribers.
  Subscribers register/unregister; **a source with zero subscribers does not sample.**
- `internal/api`: `GET /v1/info`, `GET /v1/metrics`, `GET /v1/metrics/stream` (SSE).
- `internal/audit`: structured write-action log to stderr → journald.
- Benchmarks for `/proc/stat` parse and the sampler tick. Wire `make bench` into CI as a
  regression gate.
- `packages/protocol`: TS types for `Info`, `Metrics`. Mirror into Go.
- `oikos-agent.service` unit file with hardening directives (`NoNewPrivileges`,
  `ProtectHome`, `PrivateTmp`, `GOMEMLIMIT`).

**Done when:** the agent runs on a real Debian box, refuses to start when pointed at
`0.0.0.0`, and `curl` from an allowed peer over `wg0` streams metrics at 1 Hz while a
non-allowed peer gets a clean 403. RSS under 20 MB.

---

## M2 — Agent: systemd

**Tasks**

- `internal/systemd`: `godbus` + `coreos/go-systemd` against
  `org.freedesktop.systemd1`. Define a `Conn` **interface** — `ListUnits`, `GetUnit`,
  `StartUnit`, `StopUnit`, `RestartUnit`, `Subscribe`. Fake implementation for tests using
  recorded payloads in `testdata/`.
- Use D-Bus **signals** for unit state changes, not polling. Push into the sampler's
  broadcast so the UI updates on change rather than on tick.
- **Unit allowlist enforcement** at the handler boundary, not in the D-Bus layer. Write
  actions on non-allowlisted units → `403`, audited. Read scope from `read_allowlist`.
- `internal/journal`: shell out to `journalctl -o json -f -u <unit>`, parse the JSON stream,
  expose as SSE. Bounded backlog, cancel on client disconnect (leaked `journalctl`
  processes are the obvious bug here — test for it).
- API: `GET /v1/services`, `GET /v1/services/:unit`,
  `POST /v1/services/:unit/{start,stop,restart}`, `GET /v1/services/:unit/logs?follow=1`.

**Note on journald:** `sd-journal` is C-only and the on-disk format is internal, not an ABI.
Shelling out to `journalctl` is the sanctioned path and the one concession to "no
subprocesses." Don't try to parse `/var/log/journal` directly.

**Done when:** services list, start/stop/restart obey the allowlist, logs stream, and killing
the client leaves no orphan `journalctl`.

---

## M3 — Agent: Docker

**Tasks**

- `internal/docker`: `http.Client` with a `DialContext` onto the unix socket. No Docker SDK.
  Pin the API version in the path.
- Capability-gated: if the socket is absent or undialable, every route returns a structured
  `docker_unavailable` error and `/v1/info` reports `docker: false`. Same binary everywhere —
  the UI hides the tab, the agent doesn't care.
- API: `GET /v1/docker/containers`, `GET /v1/docker/containers/:id`,
  `POST /v1/docker/containers/:id/{start,stop,restart}`,
  `GET /v1/docker/containers/:id/logs?follow=1`, `GET /v1/docker/images`.
- Container logs use Docker's multiplexed stream framing — demux stdout/stderr properly, it's
  a length-prefixed header format, not raw text.
- Tests: `httptest` server bound to a temp unix socket, replaying recorded Engine API
  payloads.

**Security note to write down in `SECURITY.md` while it's fresh:** the Docker socket *is*
root, with no nuance. An agent with docker access is root-equivalent regardless of the user
it runs as. Consider a `docker: read_only` config flag for hosts where container control
isn't wanted.

**Done when:** containers list and stream logs on a Docker host, and the agent behaves
correctly on a host with no Docker at all.

---

## M4 — Desktop shell: peers, connection, Fleet Strip

First pixels. Aim the effort at the signature element (`CLAUDE.md` §7) — if the Fleet Strip
is right, the rest of the app is filling in tables.

**Tasks**

- Tauri scaffold. Rust core owns: `peers.toml` load/save (in the platform config dir), the
  HTTP client, SSE subscriptions, and fan-out to the webview via Tauri events. The webview
  never opens a socket itself.
- Peer management UI: add/remove/rename, live reachability. Adding a peer is a name and a
  `wg0` address — nothing else.
- Design tokens first: palette, IBM Plex families, the 28–32px row grid, `tabular-nums` as a
  base. Get these into Tailwind config before building components.
- shadcn/ui init. Pull in only what's used.
- **Fleet Strip**: fixed-height row per host — name plate (Plex Condensed, uppercase,
  tracked), uPlot sparklines for CPU/RAM/net, LED cluster on the right. Persistent, never
  scrolls away.
- Focus view: click a host → detail pane below, with a real Performance view (larger uPlot
  charts, same data source).
- Disconnected state: a labelled offline plate, last-seen timestamp, retained layout. No
  spinner limbo, no blank panel.

**Done when:** the app is open next to a terminal for an hour and you reach for it first.

---

## M5 — Services, Processes and Docker views

**Tasks**

- TanStack Table v8 + TanStack Virtual. One table primitive, three configurations. Column
  sizing persisted per view.
- **Processes view**: the Task Manager panel. Virtualized, sorted by CPU or RSS. Subscribes
  on mount and **unsubscribes on unmount** — this is what makes the agent's
  subscription-driven sampling actually pay off. Verify agent CPU drops when the tab closes.
- **Services view**: unit list with state LEDs, allowlisted actions enabled and the rest
  visibly disabled with a reason (don't hide them — the operator should see the boundary).
- **Docker view**: containers + images, rendered only where `info.docker === true`.
- **Log viewer**: shared component for journald and Docker logs. Virtualized, follow-tail
  with a break-on-scroll, level filter, plain-text copy.

**Done when:** every read path in the agent has a surface in the UI, and long process lists
scroll at 60fps.

---

## M6 — Command palette and workflow polish

The original grievance was that the default UI makes everything a form. This is where that
gets answered.

**Tasks**

- `cmdk` palette on ⌘K, scoped by context: `restart nginx on lab-02` resolves host + unit +
  action in one line.
- Fuzzy resolution across the fleet: hosts, units, containers in one index.
- Confirm-on-destructive: `stop` and `restart` prompt inline in the palette, not a modal.
- Keyboard navigation across the whole app — the palette is the entry point, not the only one.
- Toasts that use the same verb as the action (`Restart` → `Restarted`).

**Done when:** the common tasks are reachable without touching the mouse.

---

## M7 — Install flow

**Tasks**

- `install.sh`: download the release for the detected arch, **verify the checksum** (a
  `curl | sh` without verification, for a root daemon, is exactly the pattern this project
  claims not to be), install the binary and unit, `systemctl enable --now`.
- Push-install from the desktop app: scp binary + rendered unit over SSH, start it, add the
  peer. Guest needs no clone, no toolchain, no curl.
- Agent self-update check: report latest release in the UI. **Report only — never
  self-update.** A root daemon that updates itself is a supply-chain hole with a nice UI.
- `docs/INSTALL.md` including the manual path, since the manual path is the fallback that
  always works.

**Done when:** a fresh Debian VM goes from nothing to visible in the Fleet Strip in under a
minute, without opening a terminal on it.

---

## M8 — Hardening pass

Do this before making the repo public. This is the milestone that decides whether the project
reads as a toy or as serious work.

**Tasks**

- Non-root agent: dedicated `oikos` system user, polkit rules for the specific systemd
  actions in the allowlist, `docker` group only where container control is enabled. Measure
  what actually breaks — this may not fully land, and a documented honest partial is better
  than a false claim.
- Full systemd sandboxing on the agent unit: `ProtectSystem=strict`, `ProtectKernelTunables`,
  `RestrictAddressFamilies`, `SystemCallFilter`, `CapabilityBoundingSet` trimmed to what
  remains necessary.
- Fuzz the JSON and SSE parsing surfaces. `go-fuzz`/native fuzzing on the config loader and
  the journal parser.
- `SECURITY.md`: the real threat model. State plainly what the agent is (root-equivalent),
  what protects it (WG cryptokey routing + peer allowlist + unit allowlist + no exec), and
  what it does not protect against (a compromised operator machine owns the fleet; Docker
  socket access is root; nested D-in-D containers escape).
- README with an architecture diagram, the explicit non-goals, and a short honest section on
  prior art — **Cockpit** does much of this and ships in Debian. Say so, and say what's
  different (single static binary, fleet-first view, one desktop app, WG-native). A portfolio
  project that names its prior art reads as confident; one that ignores it reads as unaware.

**Done when:** a stranger can read `SECURITY.md` and correctly decide whether to run this.

---

## Preview 2 (not now)

Sketched only so M1–M8 don't design it out of reach:

1. MCP server as a **separate binary** sharing the peer list — usable from Claude Code
   without the GUI open.
2. Narrow typed tools; read free, write confirmed; **no generic exec tool, ever**.
3. Plan/diff/apply: Claude authors a unit file → the app diffs it against the host → operator
   approves. Generated artifacts live in a local git repo. Git holds provenance; the host
   stays a projection.
4. Optional always-on collector, if metric history or alerting ever justifies a server node
   again.

---

## Risk register

| Risk | Where it bites | Mitigation |
|---|---|---|
| **Root-equivalence** | The whole premise | Bind guard, peer allowlist, unit allowlist, no exec, audit log, M8 hardening pass. |
| **Sandbox can't test systemd/Docker** | M2, M3 velocity | Fixture-first: injectable roots, interfaces, recorded payloads. Non-negotiable. |
| **Scope creep back toward an orchestrator** | Everywhere | `CLAUDE.md` §1 "What this is NOT". Re-read it when a feature feels clever. |
| **Orphaned `journalctl` processes** | M2 | Explicit lifecycle test on client disconnect. |
| **uPlot × N hosts × 1 Hz** | M4 | One data source, subscription-driven; profile the strip with 10 fake peers early. |
| **Cockpit already exists** | Motivation | Evaluate it honestly for 20 minutes before M4. Build anyway if the reasons hold — but know them. |
