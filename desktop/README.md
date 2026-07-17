# desktop

The Oikos desktop app — a Tauri shell (Rust core + React/TS UI) that runs on the
operator's machine only. It holds `peers.toml`, owns the HTTP/SSE client, fans
requests out to every agent, and renders the fleet (CLAUDE.md §3, §7).

## Layout

| Path | What | Builds without a GUI toolkit? |
|---|---|---|
| `core/` | `oikos-core` — peers, the agent HTTP/SSE client, wire types. **No Tauri/webkit dep.** | ✅ `cargo test` |
| `src-tauri/` | The Tauri shell: commands + event fan-out. Thin glue, no logic. | ❌ needs system webkit |
| `src/` | React + TypeScript UI (Vite). | ✅ `pnpm build` |

The split is deliberate: everything worth testing lives in `core`, which builds
and tests in any CI or sandbox. `src-tauri` is the thin part verified locally.

## Develop

```bash
cd desktop
pnpm install

# UI in a plain browser (no Tauri): the bridge falls back to a mock that
# synthesises a live fleet, so the Fleet Strip works standalone.
pnpm dev            # http://localhost:5173

# The full desktop app (needs the Tauri system deps for your OS):
pnpm tauri dev
```

On Debian/Ubuntu the Tauri build needs:

```bash
sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev librsvg2-dev \
  libayatana-appindicator3-dev libxdo-dev libssl-dev
```

## Test

```bash
cd desktop/core && cargo test         # peers + client (over a stub socket)
cd desktop && pnpm typecheck && pnpm lint && pnpm build
```

## Design

The register is **rack-mounted instrument** (CLAUDE.md §7): warm graphite,
silkscreen label plates, meters and status LEDs on a grid. Tokens live in
`tailwind.config.js`; colour carries state and nothing else. The **Fleet Strip**
is the signature element — one fixed-height row per host, always visible: name
plate, uPlot sparklines (CPU/RAM/net), LED cluster. Click a row to focus it in
the Performance view below. An offline peer degrades to a labelled disconnected
plate with a last-seen timestamp — never a blank panel or an endless spinner.
