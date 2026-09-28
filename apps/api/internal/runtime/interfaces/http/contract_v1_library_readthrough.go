package httpapi

// Library materialization read-through (owner-approved cut, internal-only).
//
// The public library:materialize job API is deleted (no POST/GET
// lmat_ routes, no billing, no events, no frontend polling), and the
// internal fetcher (media/assets.LibraryMaterializer) has no
// production caller. Without a trigger, a freshly saved remote library
// reference (lasset_) never becomes managed media (asset_), so the
// Canvas import path (frontend materializeLibraryAssetForCanvas: one
// poll-free GET media-assets/:asset_id with the lasset_ id, retrying
// through the Canvas pending-fact loop) would 404 forever.
//
// This file wires the fetcher back into the end-to-end flow with a
// read-through on that single metadata GET (no new routes, no new
// workers, no River kinds, no billing, no events):
//   - lasset_ id on GET media-assets/:asset_id resolves the owned
//     library asset, then scans the durable library_materializations
//     job records for a completed link. A hit serves the linked managed
//     asset (200) with its asset_-based content/derivative URLs.
//   - A miss fires ONE best-effort async fetch (fire-and-forget
//     goroutine with panic recovery, mirroring the tasking execution
//     watcher discipline) and answers 404 media_not_found now; the
//     Canvas retry loop re-reads until the fetch lands (200).
//   - Dedup rides an atomic job claim (Claim -> Running -> Complete/Failed):
//     completed or in-flight (pending/running) jobs suppress a new
//     trigger; only a failed (or absent) history starts a new attempt.
//     The concrete store's atomic claimer closes the concurrent trigger
//     race; stores without that capability fail closed with no trigger
//     (404) instead of risking duplicate jobs.
//   - The async fetch stamps the reading member as owner
//     (WithLibraryMaterializationOwner) for the session-flow ingest and
//     runs detached (WithoutCancel + timeout) so the response never
//     blocks on provider bytes. The closed-world egress gate stays
//     fail-closed inside the fetcher: without operator allowlisting of
//     the provider CDN hosts the job records failed and reads keep
//     404ing (see residuals in the file footer).
//   - Task-output library rows (provider workspace_media) carry no
//     exposed media link on V1Asset, so they 404 without a trigger;
//     their media already exists as managed assets and the task worker
//     links them directly (no fetch needed there).
//
// Failure posture: trigger errors NEVER fail the read (404 either
// way) and fold into a failed job record with the fixed
// library_materialization_failed message (same shape as the task
// worker; no error_code is persisted beyond the existing seam param).
// Terminal job records are immutable, so concurrent settlers converge.

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/library"
	libraryports "ideaboard/api/internal/library/ports"
	materializerassets "ideaboard/api/internal/media/assets"
	derivativeports "ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// libraryMaterializationFailureCode/Message mirror the task worker's
// auto-materializer failure record (same shape as the former manual
// surface; the code word is seam-stable, not durably persisted).
const (
	libraryMaterializationFailureCode    = "library_materialization_failed"
	libraryMaterializationFailureMessage = msgPackage
)

// libraryReadThroughTimeout bounds one detached async fetch (the
// shared remote downloader already caps a single fetch at 30s;
// ingest is local byte writes).
const libraryReadThroughTimeout = 90 * time.Second

// libraryMaterializationLister is the optional job-listing capability
// the read-through needs for its completed-job scan and trigger
// dedup. The postgres library Service implements it; the frozen
// WorkspaceLibraryStore seam does not carry it, so callers
// type-assert and fail closed (no trigger) when it is absent.
type libraryMaterializationLister interface {
	ListMaterializations(ctx context.Context, workspacePublicID string) ([]*library.V1Materialization, error)
}

// libraryReadThroughTrigger runs one on-demand materialization: sync
// resolve (completed job -> managed asset) plus the async fetch with
// job-record dedup. Fetcher/Renderer/Spawn ride the v1 handler deps
// (production defaults in ContractV1Deps.normalize); tests inject
// fakes.
type libraryReadThroughTrigger struct {
	Library  library.WorkspaceLibraryStore
	Media    WorkspaceMediaStore
	Objects  WorkspaceMediaObjects
	Fetcher  materializerassets.LibrarySourceFetcher
	Renderer derivativeports.Renderer
	Spawn    func(func())
}

// newLibraryReadThroughTrigger builds the trigger from the handler
// deps. Nil library/media/objects/fetcher fail closed (nil trigger:
// reads 404 without spawning).
func newLibraryReadThroughTrigger(deps ContractV1Deps) *libraryReadThroughTrigger {
	if deps.Library.Store == nil || deps.Media == nil || deps.MediaObjects == nil || deps.libraryMaterializer == nil {
		return nil
	}
	var renderer derivativeports.Renderer
	if deps.MediaRenderer != nil {
		renderer = deps.MediaRenderer
	}
	spawn := deps.libraryMaterializeSpawn
	if spawn == nil {
		spawn = func(fn func()) { go fn() }
	}
	return &libraryReadThroughTrigger{
		Library: deps.Library.Store, Media: deps.Media, Objects: deps.MediaObjects,
		Fetcher: deps.libraryMaterializer, Renderer: renderer, Spawn: spawn,
	}
}

// findCompletedLibraryMedia scans one workspace job listing for the
// newest completed link for the library asset. Malformed media links
// read as a miss (never handed to the media store or the fetcher, both
// of which require asset_ shape).
func findCompletedLibraryMedia(jobs []*library.V1Materialization, assetPublicID string) (string, bool) {
	trimmed := strings.TrimSpace(assetPublicID)
	for index := len(jobs) - 1; index >= 0; index-- {
		job := jobs[index]
		if job == nil || job.AssetPublicID != trimmed {
			continue
		}
		if job.Status != library.V1MaterializationCompleted {
			continue
		}
		if !contract.IsAssetID(strings.TrimSpace(job.MediaAssetID)) {
			continue
		}
		return strings.TrimSpace(job.MediaAssetID), true
	}
	return "", false
}

// libraryMaterializationReplay backs the fetcher's replay seam by
// scanning the concrete store's job listing for a completed link, so
// repeat fetches converge on the same asset_ id without refetching
// (no new SQL, no migration).
type libraryMaterializationReplay struct {
	lister libraryMaterializationLister
}

var _ materializerassets.MaterializationReplayStore = (*libraryMaterializationReplay)(nil)

func (r *libraryMaterializationReplay) FindCompletedMaterializationMedia(ctx context.Context, workspacePublicID, assetPublicID string) (string, bool, error) {
	if r == nil || r.lister == nil {
		return "", false, nil
	}
	jobs, err := r.lister.ListMaterializations(ctx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return "", false, err
	}
	mediaID, found := findCompletedLibraryMedia(jobs, assetPublicID)
	return mediaID, found, nil
}

// resolve serves the read-through synchronously: owned library asset
// -> completed job link -> managed media asset. ok=false is a miss
// (unknown asset, no completed link, or unreadable media): the caller
// triggers and answers 404.
func (t *libraryReadThroughTrigger) resolve(ctx context.Context, workspacePublicID, assetPublicID string) (*workspacemedia.Asset, bool) {
	if t == nil || t.Library == nil || t.Media == nil {
		return nil, false
	}
	stored, err := t.Library.GetAsset(ctx, workspacePublicID, assetPublicID)
	if err != nil || stored == nil {
		return nil, false
	}
	if strings.TrimSpace(stored.Provider) == library.ProviderWorkspaceMedia {
		return nil, false
	}
	lister, ok := t.Library.(libraryMaterializationLister)
	if !ok {
		return nil, false
	}
	jobs, err := lister.ListMaterializations(ctx, workspacePublicID)
	if err != nil {
		return nil, false
	}
	mediaID, found := findCompletedLibraryMedia(jobs, assetPublicID)
	if !found {
		return nil, false
	}
	asset, err := t.Media.GetAsset(ctx, workspacePublicID, mediaID)
	if err != nil || asset == nil {
		return nil, false
	}
	return asset, true
}

// spawnFetch fires the best-effort async fetch for a known remote
// library asset. The detached context (owner-stamped, egress policy
// via the global provider) outlives the request; panics are absorbed
// with a log so the trigger can never take down the server.
func (t *libraryReadThroughTrigger) spawnFetch(baseCtx context.Context, workspacePublicID string, stored *library.V1Asset, ownerUserID int64) {
	if t == nil || t.Spawn == nil || stored == nil || ownerUserID <= 0 {
		return
	}
	workspaceID := strings.TrimSpace(workspacePublicID)
	assetCopy := *stored
	t.Spawn(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("library read-through fetch panicked",
					"workspace_id", workspaceID, "library_asset", assetCopy.PublicID, "panic", recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(baseCtx), libraryReadThroughTimeout)
		defer cancel()
		t.run(ctx, workspaceID, &assetCopy, ownerUserID)
	})
}

// run executes one materialization attempt: atomic job claim
// (pending -> running), fetch, terminal link. Every failure folds
// into a failed job record; nothing propagates to the read path.
func (t *libraryReadThroughTrigger) run(ctx context.Context, workspacePublicID string, stored *library.V1Asset, ownerUserID int64) {
	lister, ok := t.Library.(libraryMaterializationLister)
	if !ok {
		slog.Warn("library read-through skipped: library store cannot list materialization jobs",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID)
		return
	}
	claimer, ok := t.Library.(libraryports.MaterializationClaimer)
	if !ok {
		slog.Warn("library read-through skipped: library store cannot claim materialization jobs",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID)
		return
	}
	matID, err := library.NewV1MaterializationPublicID()
	if err != nil {
		slog.Warn("library read-through: job mint failed", "workspace_id", workspacePublicID, "err", err)
		return
	}
	job, created, err := claimer.ClaimMaterialization(ctx, workspacePublicID, stored.PublicID, matID)
	if err != nil || job == nil {
		slog.Warn("library read-through: job create failed",
			"workspace_id", workspacePublicID, "materialization", matID, "err", err)
		return
	}
	matID = job.PublicID
	if !created && job.Status == library.V1MaterializationCompleted {
		slog.Info("library read-through converges: completed job already links media; skipping duplicate fetch",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID, "materialization", matID)
		return
	}
	if !created && (job.Status == library.V1MaterializationPending || job.Status == library.V1MaterializationRunning) {
		slog.Info("library read-through converges: materialization already in flight",
			"workspace_id", workspacePublicID, "library_asset", stored.PublicID, "materialization", matID)
		return
	}
	if _, err := t.Library.SetMaterializationResult(ctx, workspacePublicID, matID, library.V1MaterializationRunning, "", "", ""); err != nil {
		_, _ = t.Library.SetMaterializationResult(ctx, workspacePublicID, matID,
			library.V1MaterializationFailed, "", libraryMaterializationFailureCode, libraryMaterializationFailureMessage)
		slog.Warn("library read-through: job start failed",
			"workspace_id", workspacePublicID, "materialization", matID, "err", err)
		return
	}
	materializer, err := materializerassets.NewLibraryMaterializer(materializerassets.LibraryMaterializerDeps{
		Media: t.Media, Objects: t.Objects, Fetcher: t.Fetcher,
		Renderer: t.Renderer, Replay: &libraryMaterializationReplay{lister: lister},
	})
	if err != nil {
		_, _ = t.Library.SetMaterializationResult(ctx, workspacePublicID, matID,
			library.V1MaterializationFailed, "", libraryMaterializationFailureCode, libraryMaterializationFailureMessage)
		slog.Warn("library read-through: fetcher unavailable",
			"workspace_id", workspacePublicID, "materialization", matID, "err", err)
		return
	}
	fetchCtx := materializerassets.WithLibraryMaterializationOwner(ctx, ownerUserID)
	mediaID, err := materializer.MaterializeWorkspaceAsset(fetchCtx, workspacePublicID, stored)
	if err != nil {
		_, _ = t.Library.SetMaterializationResult(ctx, workspacePublicID, matID,
			library.V1MaterializationFailed, "", libraryMaterializationFailureCode, libraryMaterializationFailureMessage)
		slog.Warn("library read-through: fetch failed",
			"workspace_id", workspacePublicID, "materialization", matID,
			"library_asset", stored.PublicID, "err", err)
		return
	}
	if _, err := t.Library.SetMaterializationResult(ctx, workspacePublicID, matID, library.V1MaterializationCompleted, mediaID, "", ""); err != nil {
		slog.Error("library read-through: media link failed; repair required",
			"workspace_id", workspacePublicID, "materialization", matID, "media_asset", mediaID, "err", err)
		return
	}
	slog.Info("library read-through materialized remote reference as managed media",
		"workspace_id", workspacePublicID, "materialization", matID,
		"library_asset", stored.PublicID, "media_asset", mediaID)
}

// getLibraryMaterializedMediaAsset serves GET media-assets/:asset_id
// for lasset_ ids (the Canvas import read path). A completed link
// renders the managed asset DTO (200); anything else spawns one
// best-effort fetch and renders 404 media_not_found so the Canvas
// pending-fact retry loop owns the retry. Unknown assets, unmanaged
// providers, and unwired stores 404 without spawning.
func (h *contractV1Handler) getLibraryMaterializedMediaAsset(c *gin.Context, workspacePublicID, assetPublicID string) {
	trigger := newLibraryReadThroughTrigger(h.deps)
	if trigger == nil {
		renderMediaStoreError(c, workspacemedia.ErrNotFound)
		return
	}
	ctx := c.Request.Context()
	stored, err := trigger.Library.GetAsset(ctx, workspacePublicID, assetPublicID)
	if err != nil || stored == nil {
		renderMediaStoreError(c, workspacemedia.ErrNotFound)
		return
	}
	if strings.TrimSpace(stored.Provider) == library.ProviderWorkspaceMedia {
		renderMediaStoreError(c, workspacemedia.ErrNotFound)
		return
	}
	if asset, ok := trigger.resolve(ctx, workspacePublicID, stored.PublicID); ok {
		contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
		return
	}
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	trigger.spawnFetch(ctx, workspacePublicID, stored, identityPrincipal.UserID())
	renderMediaStoreError(c, workspacemedia.ErrNotFound)
}
