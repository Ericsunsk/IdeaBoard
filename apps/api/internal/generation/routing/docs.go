// Package routing is the pure routing domain for Service Model Pools
// (ADR-0012, routing ownership in section 2a). It owns eligibility,
// deterministic selection, retry-candidate derivation, retry
// classification, and construction of the immutable RoutingSnapshot.
//
// This package is deliberately dependency-free: no database, no River, no
// HTTP, no provider SDKs. The catalog implements the PoolStore port; the
// submit transaction (later wave) calls RoutingResolver and persists the
// snapshot atomically with the credit hold, task insert, and River
// enqueue; the worker consumes only the persisted snapshot and never
// re-reads the live pool.
//
// A new package (rather than extending internal/workflow) fits because
// routing is a distinct bounded context with stable domain language
// (pool entry, snapshot, selection seed, retry taxonomy) behind one small
// module interface consumed by readiness, quote, submit, and worker
// layers. internal/workflow keeps owning feature compilation and
// parameter shaping; it never selects a model.
//
// Vision search stays outside this contract (ADR-0012 rule 14): vision
// failures must never be classified by this package and vision models
// must never enter a service pool.
package routing
