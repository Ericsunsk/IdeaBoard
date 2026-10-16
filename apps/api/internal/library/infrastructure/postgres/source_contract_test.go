package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"ideaboard/api/internal/library/domain"
)

// mustSeedSizedMediaAsset inserts a completed upload session + media asset
// with explicit dimensions (nil = unknown) and
// returns its asset_... public id. Tags must be unique per call.
func mustSeedSizedMediaAsset(t *testing.T, ctx context.Context, env *libraryTestEnv, ws, tag, kind string, width, height *int) string {
	t.Helper()
	wsInternalID := libraryWorkspaceInternalID(t, ctx, env, ws)
	mediaPublicID := fmt.Sprintf("asset_lib930_%s", tag)
	var uploadID int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO upload_sessions (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, declared_checksum, storage_key, status, completed_at)
		 VALUES ($1, $2, $3, 'image/png', 10, 'chk', $4, 'completed', now()) RETURNING internal_id`,
		fmt.Sprintf("upload_lib930_%s", tag), wsInternalID, env.userID, fmt.Sprintf("test930-storage-key-%s", tag),
	).Scan(&uploadID); err != nil {
		t.Fatalf("seed upload session: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO media_assets (public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, width, height, metadata)
		 VALUES ($1, $2, $3, $4, $5, 'image/png', 10, 'chk', $6, $7, $8, '{}')`,
		mediaPublicID, wsInternalID, uploadID, env.userID, fmt.Sprintf("test930-media-key-%s", tag), kind, width, height,
	); err != nil {
		t.Fatalf("seed media asset: %v", err)
	}
	return mediaPublicID
}

func mustSaveManaged(t *testing.T, ctx context.Context, env *libraryTestEnv, ws string, input domain.V1SaveManagedAssetInput) (*domain.V1Asset, bool) {
	t.Helper()
	publicID, err := domain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if input.OperatorUserID == 0 {
		input.OperatorUserID = env.userID
	}
	saved, created, err := env.svc.SaveManagedMediaAsset(ctx, ws, input, publicID)
	if err != nil {
		t.Fatalf("managed save: %v", err)
	}
	return saved, created
}

func intPtr(v int32) *int32 { return &v }

func checkDims(t *testing.T, what string, asset *domain.V1Asset, width, height *int32) {
	t.Helper()
	if (asset.Width == nil) != (width == nil) || (asset.Height == nil) != (height == nil) {
		t.Fatalf("%s dims = %v x %v, want %v x %v", what, asset.Width, asset.Height, width, height)
	}
	if width != nil && (*asset.Width != *width || *asset.Height != *height) {
		t.Fatalf("%s dims = %d x %d, want %d x %d", what, *asset.Width, *asset.Height, *width, *height)
	}
}

func TestLibraryExternalPartialDimensionsReadBack(t *testing.T) {
	for _, dimensions := range [][2]int32{{0, 80}, {100, 0}} {
		t.Run(fmt.Sprintf("%dx%d", dimensions[0], dimensions[1]), func(t *testing.T) {
			env := openLibraryTestEnv(t)
			ctx := context.Background()
			input := libraryTestAssetInput("partial-dimensions")
			input.Width, input.Height = dimensions[0], dimensions[1]
			publicID, err := domain.NewV1AssetPublicID()
			if err != nil {
				t.Fatal(err)
			}
			saved, created, err := env.svc.SaveAsset(ctx, env.wsA, domain.V1SaveAssetInput{Asset: input}, publicID)
			if err != nil || !created {
				t.Fatalf("save created=%t err=%v", created, err)
			}
			checkDims(t, "saved", saved, &dimensions[0], &dimensions[1])
			read, err := env.svc.GetAsset(ctx, env.wsA, saved.PublicID)
			if err != nil {
				t.Fatal(err)
			}
			checkDims(t, "read", read, &dimensions[0], &dimensions[1])
			listed, err := env.svc.ListAssets(ctx, env.wsA)
			if err != nil || len(listed) != 1 {
				t.Fatalf("list count=%d err=%v", len(listed), err)
			}
			checkDims(t, "listed", listed[0], &dimensions[0], &dimensions[1])
			resolved, err := env.svc.ResolveAssets(ctx, env.wsA, []domain.V1ResolveItem{
				{Provider: input.Provider, SourceID: input.SourceID},
				{Provider: input.Provider, SourceURL: input.SourceURL},
			})
			if err != nil || len(resolved) != 2 {
				t.Fatalf("resolve count=%d err=%v", len(resolved), err)
			}
			for _, result := range resolved {
				if result.Asset == nil {
					t.Fatalf("resolve returned no asset: %+v", result)
				}
				checkDims(t, "resolved", result.Asset, &dimensions[0], &dimensions[1])
			}
		})
	}
}

// Mixed external + known-size + unknown-size managed rows read back in
// one list with their per-source shapes (no empty-URL rows, no 503).
func TestLibraryManagedSaveAndMixedListReadBack(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	external := mustSaveAsset(t, ctx, env.svc, env.wsA, "mix-ext", "")
	knownMedia := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "mix-known", "image", ptr(1600), ptr(900))
	unknownMedia := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "mix-unknown", "image", nil, nil)
	known, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{
		MediaAssetID: knownMedia, Title: "hero", Author: "artist",
		Lineage: &domain.TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: 2},
	})
	if !created {
		t.Fatal("known managed save: created = false, want true")
	}
	unknown, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{MediaAssetID: unknownMedia})
	if !created {
		t.Fatal("unknown managed save: created = false, want true")
	}
	all, err := env.svc.ListAssets(ctx, env.wsA)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("list = %d rows, want 3", len(all))
	}
	byID := map[string]*domain.V1Asset{external.PublicID: external, known.PublicID: known, unknown.PublicID: unknown}
	for _, row := range all {
		want, ok := byID[row.PublicID]
		if !ok {
			t.Fatalf("unexpected row %q", row.PublicID)
		}
		if !reflect.DeepEqual(row, want) {
			t.Fatalf("list projection differs from save: %+v / %+v", row, want)
		}
		read, err := env.svc.GetAsset(ctx, env.wsA, row.PublicID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(read, want) {
			t.Fatalf("get projection differs from save: %+v / %+v", read, want)
		}
	}
	if known.Source != domain.AssetSourceMedia || known.Provider != "" || known.SourceID != "" ||
		known.SourceURL != "" || known.PreviewURL != "" || known.OriginalURL != "" {
		t.Fatalf("known = %+v; want media source with no provider/URL keys", known)
	}
	checkDims(t, "known", known, intPtr(1600), intPtr(900))
	checkDims(t, "unknown", unknown, nil, nil)
	if external.Source != domain.AssetSourceExternal || external.Provider != domain.ProviderUnsplash {
		t.Fatalf("external = %+v; want external source", external)
	}
	checkDims(t, "external", external, intPtr(100), intPtr(80))
}

func TestLibraryManagedMoveRetainsMediaProjection(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	for _, known := range []bool{true, false} {
		t.Run(fmt.Sprintf("known=%t", known), func(t *testing.T) {
			var width, height *int
			if known {
				width, height = ptr(1600), ptr(900)
			}
			mediaID := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, fmt.Sprintf("move-%t", known), "image", width, height)
			saved, _ := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{
				MediaAssetID: mediaID, Title: "hero",
				Lineage: &domain.TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: 2},
			})
			folder := mustFolderID(t, ctx, env.svc, env.wsA, fmt.Sprintf("Move %t", known))
			for _, target := range []string{folder, ""} {
				moved, err := env.svc.MoveAsset(ctx, env.wsA, saved.PublicID, target)
				if err != nil {
					t.Fatal(err)
				}
				if moved.Source != domain.AssetSourceMedia || moved.MediaAssetID != mediaID || moved.Title != saved.Title ||
					moved.Lineage == nil || *moved.Lineage != *saved.Lineage {
					t.Fatalf("move lost Media projection: %+v", moved)
				}
				if target != "" && moved.FolderPublicID != target {
					t.Fatalf("move folder = %q, want %q", moved.FolderPublicID, target)
				}
				checkDims(t, "moved", moved, saved.Width, saved.Height)
				read, err := env.svc.GetAsset(ctx, env.wsA, saved.PublicID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(read, moved) {
					t.Fatalf("move response differs from stored asset: %+v / %+v", moved, read)
				}
			}
		})
	}
}

// Unknown-size managed saves persist and read back null (the 23502/503
// regression: library width/height stay NULL, never zero-filled).
func TestLibraryManagedSaveUnknownDimensions(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	mediaID := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "unk-dims", "image", nil, nil)
	saved, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{MediaAssetID: mediaID})
	if !created {
		t.Fatal("unknown save: created = false, want true")
	}
	checkDims(t, "saved", saved, nil, nil)
	got, err := env.svc.GetAsset(ctx, env.wsA, saved.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	checkDims(t, "read", got, nil, nil)
}

// Task-auto and manual entries for one media asset converge on one row:
// the repeat returns the existing record without touching folder,
// lineage, or metadata.
func TestLibraryManagedSaveAutoManualConverge(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	mediaID := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "converge", "image", ptr(100), ptr(80))
	auto, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{
		MediaAssetID: mediaID, Title: "auto",
		Lineage: &domain.TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: 2},
	})
	if !created {
		t.Fatal("auto save: created = false, want true")
	}
	folder := mustFolderID(t, ctx, env.svc, env.wsA, "Converge Folder")
	manual, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{
		FolderPublicID: folder, MediaAssetID: mediaID, Title: "manual-override",
	})
	if created {
		t.Fatal("manual re-save: created = true, want existing row")
	}
	if manual.PublicID != auto.PublicID {
		t.Fatalf("manual = %q, auto = %q; want one row", manual.PublicID, auto.PublicID)
	}
	if manual.FolderPublicID == folder {
		t.Fatalf("repeat moved folder to %q; want original %q", manual.FolderPublicID, auto.FolderPublicID)
	}
	if manual.Title != "auto" {
		t.Fatalf("repeat title = %q; want original metadata kept", manual.Title)
	}
	if manual.Lineage == nil || manual.Lineage.TaskID != "task_aaaaaaaaaaaaaaaaaaaaaaaa" || manual.Lineage.ResultIndex != 2 {
		t.Fatalf("repeat lineage = %+v; want original provenance kept", manual.Lineage)
	}
	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM library_assets WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1)`, env.wsA).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
}

// Concurrent saves of one media asset converge on one row via the
// partial unique backstop (exactly one created=true).
func TestLibraryManagedSaveConcurrentDedup(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	mediaID := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "conc-dedup", "image", ptr(100), ptr(80))
	const callers = 16
	ids := make([]string, callers)
	created := make([]bool, callers)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			publicID, err := domain.NewV1AssetPublicID()
			if err != nil {
				t.Errorf("mint: %v", err)
				return
			}
			saved, wasCreated, err := env.svc.SaveManagedMediaAsset(ctx, env.wsA, domain.V1SaveManagedAssetInput{
				MediaAssetID: mediaID, OperatorUserID: env.userID,
			}, publicID)
			if err != nil {
				t.Errorf("concurrent save: %v", err)
				return
			}
			ids[i], created[i] = saved.PublicID, wasCreated
		}(i)
	}
	wg.Wait()
	first := ids[0]
	winners := 0
	for i := range ids {
		if ids[i] == "" {
			t.Fatal("concurrent save returned no id")
		}
		if ids[i] != first {
			t.Fatalf("ids diverged: %q vs %q", ids[i], first)
		}
		if created[i] {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("created winners = %d, want 1", winners)
	}
}

// Explicit folder binds on first save; repeats never move the row (moves
// stay the explicit move operation's job).
func TestLibraryManagedSaveFolderPreserved(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	mediaID := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "folder-keep", "image", ptr(100), ptr(80))
	folderA := mustFolderID(t, ctx, env.svc, env.wsA, "Folder A")
	folderB := mustFolderID(t, ctx, env.svc, env.wsA, "Folder B")
	first, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{FolderPublicID: folderA, MediaAssetID: mediaID})
	if !created || first.FolderPublicID != folderA {
		t.Fatalf("first = %+v; want created in folder A", first)
	}
	second, created := mustSaveManaged(t, ctx, env, env.wsA, domain.V1SaveManagedAssetInput{FolderPublicID: folderB, MediaAssetID: mediaID})
	if created || second.PublicID != first.PublicID || second.FolderPublicID != folderA {
		t.Fatalf("second = %+v; want existing row kept in folder A", second)
	}
}

// Cross-workspace, archived, and missing media fail closed with no row.
func TestLibraryManagedSaveForbiddenMedia(t *testing.T) {
	env := openLibraryTestEnv(t)
	ctx := context.Background()
	foreignMedia := mustSeedSizedMediaAsset(t, ctx, env, env.wsB, "foreign", "image", ptr(100), ptr(80))
	archivedMedia := mustSeedSizedMediaAsset(t, ctx, env, env.wsA, "archived", "image", ptr(100), ptr(80))
	if _, err := env.pool.Exec(ctx, `UPDATE media_assets SET archived_at = now() WHERE public_id = $1`, archivedMedia); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		ws    string
		media string
	}{
		{name: "cross-workspace", ws: env.wsA, media: foreignMedia},
		{name: "archived", ws: env.wsA, media: archivedMedia},
		{name: "missing", ws: env.wsA, media: "asset_missing000000000001"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			publicID, err := domain.NewV1AssetPublicID()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := env.svc.SaveManagedMediaAsset(ctx, tt.ws, domain.V1SaveManagedAssetInput{
				MediaAssetID: tt.media, OperatorUserID: env.userID,
			}, publicID); !errors.Is(err, domain.ErrV1AssetNotFound) {
				t.Fatalf("save err = %v, want %v", err, domain.ErrV1AssetNotFound)
			}
		})
	}
	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM library_assets`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rows = %d after forbidden saves, want 0", count)
	}
}

func ptr(v int) *int { return &v }
