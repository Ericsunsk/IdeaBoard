package application

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/ports"
	materializerassets "ideaboard/api/internal/media/assets"
)

// Library materialization read-through orchestration (issue #908).
//
// Ownership:
//   - Media (media/assets.LibraryMaterializer) owns remote download and
//     managed-media writes.
//   - This package owns the materialization lifecycle and replay
//     orchestration: resolve/claim/dedup/fetch/settle over the typed
//     store/materializer/executor seams injected by composition.
//   - HTTP only translates authenticated requests/responses and never
//     constructs the materializer, manages detached execution, or
//     advances job state.
//
// Contracts preserved: owner stamping, workspace isolation (store keys
// every lookup by workspace), provider policy (media rows
// never trigger; fetchable hosts stay code-owned inside the
// materializer), detached timeout + panic recovery (best-effort: reads
// never fail because the fetch did), atomic claim with
// pending/running dedup,
// completed replay, terminal immutability, and
// failed jobs that never masquerade as usable media. No River kinds,
// no schema or process-crash recovery policy change.

// DefaultReadThroughTimeout bounds one detached async fetch (the
// shared remote downloader already caps a single fetch at 30s;
// ingest is local byte writes).
const DefaultReadThroughTimeout = 90 * time.Second

// ReadThroughFailureCode and ReadThroughFailureMessage identify failed
// external ingestion attempts.
const (
	ReadThroughFailureCode    = "library_materialization_failed"
	ReadThroughFailureMessage = "素材转存失败，请稍后重试"
)

// DefaultReadThroughSettleTimeout bounds the post-fetch terminal
// writes (failed fold / completed link). The fetch may consume the
// whole detached timeout, so settling reuses that expired context
// would drop the fold and wedge the job in running: settlement runs
// on a fresh short-lived context detached from fetch cancellation.
const DefaultReadThroughSettleTimeout = 10 * time.Second

// ReadThroughStore is the typed persistence seam for the
// orchestration. Composition injects the postgres Service (which
// implements the listing + atomic claim); tests inject the in-memory
// store. Request-time type assertions against implied capabilities
// are gone: the compiler enforces the seam.
type ReadThroughStore interface {
	ports.WorkspaceLibraryStore
	ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error)
	ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error)
}

// Executor runs detached best-effort fetches off the request path.
// Composition injects the goroutine runner; tests inject inline or
// capturing fakes.
type Executor interface {
	Spawn(fn func())
}

// SpawnFunc adapts a plain func to Executor.
type SpawnFunc func(func())

// Spawn runs fn through f.
func (f SpawnFunc) Spawn(fn func()) {
	if f == nil || fn == nil {
		return
	}
	f(fn)
}

// GoroutineExecutor is the production Executor: fire-and-forget
// goroutines (panic recovery + timeout stay in the service).
type GoroutineExecutor struct{}

// Spawn runs fn on a new goroutine.
func (GoroutineExecutor) Spawn(fn func()) {
	if fn == nil {
		return
	}
	go fn()
}

// ReadThroughService orchestrates one on-demand materialization:
// sync resolve (completed job -> managed media link) plus the async
// fetch with job-record dedup.
type ReadThroughService struct {
	store        ReadThroughStore
	materializer materializerassets.WorkspaceAssetMaterializer
	executor     Executor
	timeout      time.Duration
}

// NewReadThroughService builds the orchestrator over typed deps. Nil
// store/materializer/executor fails closed (nil service: resolves
// miss, requests no-op) so unwired reads 404 without spawning.
func NewReadThroughService(store ReadThroughStore, materializer materializerassets.WorkspaceAssetMaterializer, executor Executor, timeout time.Duration) *ReadThroughService {
	if store == nil || materializer == nil || executor == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = DefaultReadThroughTimeout
	}
	return &ReadThroughService{store: store, materializer: materializer, executor: executor, timeout: timeout}
}

// Resolve selects the owned Media link: direct for managed references,
// completed materialization for external references.
func (s *ReadThroughService) Resolve(ctx context.Context, workspacePublicID, assetPublicID string) (stored *domain.V1Asset, mediaAssetID string, found bool) {
	if s == nil || s.store == nil {
		return nil, "", false
	}
	stored, mediaAssetID, err := (&SourceResolver{Store: s.store}).Resolve(ctx, workspacePublicID, assetPublicID)
	return stored, mediaAssetID, err == nil && mediaAssetID != ""
}

// Request fires the best-effort async fetch for a known remote
// library asset. The detached context (owner-stamped, WithoutCancel
// + timeout) outlives the request; panics are absorbed with a log
// so the trigger can never take down the server. Unknown assets,
// media rows, and missing owners are no-ops.
func (s *ReadThroughService) Request(baseCtx context.Context, workspacePublicID string, stored *domain.V1Asset, ownerUserID int64) {
	if s == nil || s.store == nil || s.materializer == nil || s.executor == nil || stored == nil || ownerUserID <= 0 {
		return
	}
	if stored.Source == domain.AssetSourceMedia {
		return
	}
	workspaceID := strings.TrimSpace(workspacePublicID)
	if workspaceID == "" {
		return
	}
	assetCopy := *stored
	s.executor.Spawn(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("library read-through fetch panicked",
					"workspace_id", workspaceID, "library_asset", assetCopy.PublicID, "panic", recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(baseCtx), s.timeout)
		defer cancel()
		s.run(ctx, workspaceID, &assetCopy, ownerUserID)
	})
}

// run executes one materialization attempt: atomic job claim
// (pending -> running), fetch, terminal link. Created jobs proceed to
// fetch, uncreated live jobs converge, and completed jobs replay.
// Every failure folds into a failed job record; nothing propagates to
// the read path.
func (s *ReadThroughService) run(ctx context.Context, workspacePublicID string, stored *domain.V1Asset, ownerUserID int64) {
	matID, err := domain.NewV1MaterializationPublicID()
	if err != nil {
		slog.Warn("library read-through: job mint failed", "workspace_id", workspacePublicID, "err", err)
		return
	}
	job, created, err := s.store.ClaimMaterialization(ctx, workspacePublicID, stored.PublicID, matID)
	if err != nil || job == nil {
		slog.Warn("library read-through: job create failed",
			"workspace_id", workspacePublicID, "materialization", matID, "err", err)
		return
	}
	matID = job.PublicID
	if !created && job.Status == domain.V1MaterializationCompleted {
		slog.Info("library read-through converges: completed job already links media; skipping duplicate fetch",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID, "materialization", matID)
		return
	}
	if !created && (job.Status == domain.V1MaterializationPending || job.Status == domain.V1MaterializationRunning) {
		slog.Info("library read-through converges: materialization already in flight",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID, "materialization", matID)
		return
	}
	if _, err := s.store.SetMaterializationResult(ctx, workspacePublicID, matID, domain.V1MaterializationRunning, "", "", ""); err != nil {
		_, _ = s.store.SetMaterializationResult(ctx, workspacePublicID, matID,
			domain.V1MaterializationFailed, "", ReadThroughFailureCode, ReadThroughFailureMessage)
		slog.Warn("library read-through: job start failed",
			"workspace_id", workspacePublicID, "materialization", matID, "err", err)
		return
	}
	fetchCtx := materializerassets.WithLibraryMaterializationOwner(ctx, ownerUserID)
	mediaID, err := s.materializer.MaterializeWorkspaceAsset(fetchCtx, workspacePublicID, stored)
	settleCtx, settleCancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultReadThroughSettleTimeout)
	defer settleCancel()
	if err != nil {
		_, _ = s.store.SetMaterializationResult(settleCtx, workspacePublicID, matID,
			domain.V1MaterializationFailed, "", ReadThroughFailureCode, ReadThroughFailureMessage)
		slog.Warn("library read-through: fetch failed",
			"workspace_id", workspacePublicID, "materialization", matID,
			"library_asset", stored.PublicID, "err", err)
		return
	}
	if _, err := s.store.SetMaterializationResult(settleCtx, workspacePublicID, matID, domain.V1MaterializationCompleted, mediaID, "", ""); err != nil {
		slog.Error("library read-through: media link failed; repair required",
			"workspace_id", workspacePublicID, "materialization", matID, "media_asset", mediaID, "err", err)
		return
	}
	slog.Info("library read-through materialized remote reference as managed media",
		"workspace_id", workspacePublicID, "materialization", matID,
		"library_asset", stored.PublicID, "media_asset", mediaID)
}
