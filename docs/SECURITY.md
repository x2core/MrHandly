# Security

> **Status: stub.** This is a placeholder created in M0. The full threat model
> is written in M8 (see `docs/ROADMAP.md`), once the agent's attack surface —
> systemd, Docker, journald, the SSE parsers — actually exists. What is written
> here now is the shape of the argument, not the finished document.

## The one sentence that matters

The Oikos agent is a **root-equivalent daemon that parses network input**. Treat
every design decision through that lens.

## What the agent is

- A read-mostly HTTP/SSE API running on each guest, bound **only** to the
  WireGuard interface — never `0.0.0.0`.
- A live projection of the host (systemd, Docker, procfs). It owns no state and
  runs no reconciliation loop.
- Able to take a bounded set of write actions (service start/stop/restart,
  container start/stop/restart) — and nothing else. **There is no `exec`
  endpoint, and there will not be one in v1.**

## What protects it

Defence in depth, each layer independent (see `CLAUDE.md` §4):

1. **Bind guard.** The agent resolves its configured interface at startup and
   refuses to start if the address is unspecified, loopback, or outside the
   configured WireGuard subnet.
2. **WireGuard cryptokey routing.** A source IP inside the tunnel is a real
   credential — it cannot be forged without the peer's private key. This is why
   there is no mTLS on top: it would be encryption over encryption.
3. **Peer allowlist.** Only configured source IPs are served; everything else
   gets a `403`.
4. **Unit allowlist.** Write actions apply only to units matching configured
   globs. Blast radius is bounded by config, not by hope.
5. **Audit log.** Every write action is logged to the agent's own journal:
   actor peer IP, action, target, result.

## What it does NOT protect against

To be completed in M8, but stated plainly even now:

- A **compromised operator machine owns the fleet.** The desktop app holds the
  peer list and speaks to every agent.
- **Docker socket access is root**, with no nuance. An agent with Docker access
  is root-equivalent regardless of the user it runs as.
- Nested / privileged container escapes are out of the agent's control.

## Reporting

No formal process yet — this is a personal project pre-first-public-tag. A
`SECURITY.md` with a real disclosure contact lands with the M8 hardening pass,
before the repository is made public.
