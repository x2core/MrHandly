# desktop

The Oikos desktop app — a Tauri shell (Rust core + React/TS UI) that runs on the
operator's machine only. It holds `peers.toml`, owns the HTTP/SSE client, fans
requests out to every agent, and renders the fleet (see `CLAUDE.md` §3, §7).

**Not scaffolded yet.** This app is built in **M4** (`docs/ROADMAP.md`). The
directory exists now so the monorepo layout in `CLAUDE.md` §5 is complete and so
CI can wire the desktop lint gate (`cargo clippy`, `tsc`, `eslint`) the moment
`desktop/src-tauri/Cargo.toml` lands.
