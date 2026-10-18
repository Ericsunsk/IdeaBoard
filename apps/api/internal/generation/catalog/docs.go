// Package catalog owns Service Model Pool persistence (ADR-0012,
// catalog/configuration ownership in section 2a). It implements the
// routing.PoolStore port the RoutingResolver reads, performs atomic
// replace-entire-pool Admin writes with optimistic concurrency, and
// persists/loads the immutable RoutingSnapshot task fact.
//
// Storage (fragments/modelpool.sql, queries/service_model_pool.sql):
//
//	service_model_pool_entry(id, service, model_id, enabled, ...)
//	  UNIQUE(service, model_id)
//	service_model_pool_resolution(pool_entry_id FK CASCADE, resolution)
//	  UNIQUE(entry, resolution); capability values are 1k/2k/4k only.
//	  `default` is never a pool entry capability (ADR-0012 rule 3).
//	service_model_pool_version(service PK, version)
//	  Per-Service monotonic uint64, opaque to routing. Starts at 1 on
//	  first effective pool write; 0 means "never written" and is only
//	  ever observed in memory, never stored (CHECK version >= 1).
//	tasks.routing_snapshot JSONB NULL
//	  Immutable submit-time snapshot. Nullable so pre-pool tasks stay
//	  readable; wave 2 submit always writes it for new tasks via
//	  SaveRoutingSnapshot inside the submit transaction.
//	task_execution_attempts.retry_layer / .gateway_retry_count
//	  Retry-layer fact columns (gateway|model_fallback|river_task,
//	  count >= 0, ''/0 until wave 2 writes them). Wave 2 owns the
//	  attempt write path; this package only documents the columns.
//
// Optimistic concurrency: ReplacePool takes the caller-observed
// expectedVersion, holds a per-service advisory transaction lock, and
// compares-and-swaps. A mismatch fails with ErrPoolVersionConflict and
// writes nothing. No-op saves (effective-equal pool) succeed without
// bumping the version, per ADR-0012 rule 9 ("increments only when an
// effective pool entry, enabled state, or capability set changes").
//
// Snapshot JSON shape is routing.RoutingSnapshot exactly (no envelope):
//
//	{"service","requested_resolution","eligible_models",
//	 "selected_model","retry_candidate","selection_seed","pool_version"}
//
// with selection_seed as 32-char lowercase hex (128-bit) and
// pool_version as uint64. eligible_models is sorted lexicographically.
// Save/Load validate via RoutingSnapshot.Verify, so a stored snapshot
// always replays without the live pool.
//
// Vision search stays outside this contract (ADR-0012 rule 14): service
// validation accepts only AI image services, so "vision" can never own
// a pool.
package catalog
