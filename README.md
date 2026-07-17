# Oikos

A personal control panel for a small fleet of Linux machines, reachable over an
existing WireGuard mesh. A Windows Task Manager for a whole fleet — plus a
minimal Docker UI where Docker is present.

> **Name is a placeholder** (`oikos` = household estate management). It will be
> swapped before the first public tag.

A lightweight **agent** runs on each guest and exposes a read-mostly HTTP/SSE
API over WireGuard. A **Tauri desktop app** on the operator's machine talks to
every agent directly and renders the fleet.

```
Desktop app (Tauri: Rust core + React)   ── operator's machine, holds peers.toml
        │  HTTP + SSE over wg0
   ┌────┴────┬─────────┬─────────┐
 agent     agent     agent     agent      ── Debian guests: one static binary
   │
   ├─ procfs (/proc, /sys)            → metrics, processes
   ├─ D-Bus (systemd)                 → services
   ├─ journalctl -o json             → logs
   └─ /var/run/docker.sock            → containers
```

There is **no server node**. The desktop app is the only aggregation point.

## What this is NOT

- Not an orchestrator, not a PaaS, not Kubernetes/Nomad.
- Not coupled to Proxmox. Not multi-tenant. Not production infrastructure.

See [`CLAUDE.md`](./CLAUDE.md) for the full theses, invariants, and stack
decisions, and [`docs/ROADMAP.md`](./docs/ROADMAP.md) for the milestone plan.

## Repository layout

| Path | What |
|---|---|
| `agent/` | Go. The only thing that runs on guests. |
| `desktop/` | Tauri app. Operator machine only (scaffolded in M4). |
| `packages/protocol/` | Shared wire-API types. Source of truth for both sides. |
| `docs/` | Roadmap, security notes. |
| `.github/workflows/` | CI and release pipelines. |

## Status

**M3 — Docker (in progress).** A Docker Engine client over the raw unix socket
using plain `net/http` — no Docker SDK, API version pinned in the path. Serves
`GET /v1/docker/containers`, `GET /v1/docker/containers/:id`,
`POST /v1/docker/containers/:id/{start,stop,restart}`,
`GET /v1/docker/containers/:id/logs?follow=1` (SSE, with the multiplexed
stdout/stderr **frame demux**), and `GET /v1/docker/images`. Capability-gated:
where no socket is dialable every route returns `docker_unavailable` and the
capability is re-probed lazily. The socket is root-equivalent, so writes obey a
`docker_read_only` flag at the handler boundary (`403 docker_read_only`,
audited). Tested with an `httptest` engine over a temp unix socket replaying
recorded Engine API payloads — no Docker daemon required.

**M2 — systemd (done).** systemd units over D-Bus behind a fakeable `Conn`
interface (`godbus`, `CGO_ENABLED=0`): `GET /v1/services`,
`GET /v1/services/:unit`, `POST /v1/services/:unit/{start,stop,restart}`,
`GET /v1/services/:unit/logs?follow=1`, and live `GET /v1/services/stream`
(SSE, D-Bus signals not polling). Write actions gated by the **unit allowlist
at the handler boundary** (403 + audit); logs shell out to `journalctl -o json`
with a strict no-orphan process lifecycle. Capability-gated with
`systemd_unavailable`.

**M1 — agent core (done).** Strict TOML config, the **bind guard** (refuses to
start on an unspecified, loopback, or out-of-subnet address), and the **peer
allowlist** middleware. `GET /v1/info`, `GET /v1/metrics`, and
`GET /v1/metrics/stream` (SSE), fed by a subscription-driven sampler that reads
nothing when nobody is watching. Idle RSS ~6 MB (budget < 20 MB).

**M0 — skeleton and release pipeline (done).** Static `CGO_ENABLED=0` binary;
CI gates the agent (`go vet`, `staticcheck`, `go test -race`, benchmarks) and
the protocol package (`tsc`, `eslint`); a tag `agent-v*` produces a checksummed,
provenance-attested GitHub Release for `linux/amd64` and `linux/arm64`.

## Building the agent

```bash
cd agent
make build      # CGO_ENABLED=0 static binary → dist/oikos-agent
make test       # fixture-based, no root/systemd/docker needed
make lint       # go vet + staticcheck
./dist/oikos-agent   # M0: prints version and exits
```

Cross-compilation is the norm; the guest never builds anything:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 make build
```

## Cutting an agent release

```bash
git tag agent-v0.0.1
git push origin agent-v0.0.1
```

This runs the `agent-release` workflow, which builds both static binaries,
generates `checksums.txt`, attaches a build-provenance attestation, and
publishes a GitHub Release. Verify a download with:

```bash
sha256sum -c checksums.txt
```
