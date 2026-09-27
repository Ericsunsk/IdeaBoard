// Cross-cutting transport waits (frontend).
//
// Single source for waits shared across transports: workspace task polling,
// workspace event-stream reconnect, media metadata TTL, admin session
// freshness. Per-surface polling cadences (monitor 4s, anomaly radar
// 15s/60s, run-detail 3s, health/finding 5s/30s) stay local to their
// resources — they are freshness policy, not transport waits. The canvas
// histogram throttle (250ms) likewise stays local.
export const TASK_POLL_INITIAL_DELAY_MS = 1_200
export const TASK_POLL_MAX_DELAY_MS = 8_000
export const TASK_POLL_BACKOFF_FACTOR = 1.6

export const EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS = 1_000
export const EVENT_STREAM_RECONNECT_MAX_DELAY_MS = 30_000
export const EVENT_STREAM_RECONNECT_BACKOFF_FACTOR = 1.6

export const MEDIA_METADATA_TTL_MS = 5 * 60 * 1_000

export const SESSION_FRESH_THRESHOLD_MS = 300_000
