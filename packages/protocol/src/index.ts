// @oikos/protocol — the single source of truth for the Oikos wire API.
//
// Types declared here are mirrored into Go (CLAUDE.md §10); if the two sides
// drift, that is a bug, not a variation. Agent errors carry a stable `code`
// and the UI switches on `code`, never on message text.
//
// M0 ships only the version handshake and the shared error envelope. The real
// payloads — `Info`, `Metrics`, and the SSE frames — land in M1.

/**
 * Protocol version negotiated between the desktop app and each agent. Bumped
 * only on an incompatible change to the wire format.
 */
export const PROTOCOL_VERSION = 1 as const;

/**
 * Stable machine-readable error codes returned by the agent. The UI switches
 * on these; message text is for humans and may change freely.
 *
 * More codes are added as endpoints land (`docker_unavailable`,
 * `unit_not_allowed`, `peer_forbidden`, …). M0 defines only the baseline.
 */
export type ErrorCode = 'internal' | 'not_found' | 'bad_request';

/** The structured error envelope every agent error response conforms to. */
export interface ApiError {
  /** Stable, switchable identifier. */
  code: ErrorCode;
  /** Human-readable description. Never parse this. */
  message: string;
}
