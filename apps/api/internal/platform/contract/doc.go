// Package contract is the shared backend contract kernel for the Public
// Data Plane (/api/v1) and Control Plane (/api/control/v1) redesign.
//
// Baseline: IdeaBoard-API-Redesign-Baseline sections 2 (surfaces), 5
// (contract kernel), 11 (verification gates). First batch: issue #800;
// issues #801-809 build on top.
//
// Rules for this package:
//   - Additive only. The legacy numeric envelope in platform/response
//     stays untouched as the compat shim for the existing /api/user and
//     /api/admin routes (removal is #809).
//   - Thin wrappers freeze the new wire bounds (page_size default 20 max
//     100; header-only Idempotency-Key) without changing the underlying
//     platform packages.
//   - platform must not import business contexts; this package imports
//     only platform siblings and gin.
//   - River + PostgreSQL stay the task queue and state fact source; NATS
//     stays the event layer; the Better Auth transport is untouched.
package contract
