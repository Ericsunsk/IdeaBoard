package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	mediaassets "ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"

	"github.com/google/uuid"
)

func newBoardRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestBoardRepositoryOwnerAndAccessibleLookup(t *testing.T) {
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	userRepo := users.NewUserRepository(db)

	owner := &users.User{DisplayName: "owner", Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), owner); err != nil {
		t.Fatalf("seed owner failed: %v", err)
	}
	member := &users.User{DisplayName: "member", Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), member); err != nil {
		t.Fatalf("seed member failed: %v", err)
	}

	t.Run("missing owner board returns record not found", func(t *testing.T) {
		board, err := repo.GetByOwnerAndBoardID(context.Background(), owner.ID, "missing-board")
		if board != nil {
			t.Fatalf("board = %#v, want nil", board)
		}
		if !database.IsNotFound(err) {
			t.Fatalf("err = %v, want not found", err)
		}
	})

	t.Run("existing board returns owner board", func(t *testing.T) {
		const boardID = "board-1"
		seed := &UserBoard{
			BoardID:     boardID,
			OwnerUserID: owner.ID,
			CreatedBy:   owner.ID,
			Name:        "Board 1",
			State:       "{}",
		}
		seed.CreatedAt = time.Now()
		seed.UpdatedAt = seed.CreatedAt
		if err := repo.Upsert(context.Background(), seed); err != nil {
			t.Fatalf("seed board failed: %v", err)
		}

		board, err := repo.GetByOwnerAndBoardID(context.Background(), owner.ID, boardID)
		if err != nil {
			t.Fatalf("GetByOwnerAndBoardID returned error: %v", err)
		}
		if board == nil {
			t.Fatalf("board is nil")
			return
		}
		if board.BoardID != boardID {
			t.Fatalf("board.BoardID = %q, want %q", board.BoardID, boardID)
		}

		if err := repo.UpsertMember(context.Background(), owner.ID, boardID, member.ID, BoardAccessRoleViewer); err != nil {
			t.Fatalf("seed board member failed: %v", err)
		}
		accessible, err := repo.GetAccessibleByBoardID(context.Background(), member.ID, boardID)
		if err != nil {
			t.Fatalf("GetAccessibleByBoardID returned error: %v", err)
		}
		if accessible == nil {
			t.Fatal("accessible board is nil")
			return
		}
		if accessible.Board.OwnerUserID != owner.ID {
			t.Fatalf("accessible owner id = %d, want %d", accessible.Board.OwnerUserID, owner.ID)
		}
		if accessible.Role != BoardAccessRoleViewer {
			t.Fatalf("accessible role = %s, want viewer", accessible.Role)
		}
	})
}

func TestBoardRepositoryUpsertReconcilesBoardMediaRefs(t *testing.T) {
	ctx := context.Background()
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	userRepo := users.NewUserRepository(db)
	q := dbsqlc.New(db)

	owner := &users.User{DisplayName: "media-owner", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("seed owner failed: %v", err)
	}
	assetAID := seedBoardMediaAsset(t, q, owner.ID, "board-media-a", "uploads/tasks/42/task-a/output.png")
	assetBID := seedBoardMediaAsset(t, q, owner.ID, "board-media-b", "uploads/tasks/42/task-b/output.png")
	if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{
		MediaAssetID: assetAID,
		OwnerUserID:  owner.ID,
		RefType:      "task_output",
		RefID:        "task-output-a",
		BoardID:      "",
		TaskID:       "task-a",
		Role:         "image",
		Metadata:     json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("seed task output media ref failed: %v", err)
	}

	now := time.Now()
	board := &UserBoard{
		BoardID:            "media-board",
		OwnerUserID:        owner.ID,
		CreatedBy:          owner.ID,
		Name:               "Media Board",
		State:              boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetAID, RefType: "board_node", RefID: "image-1", Role: "source"}}),
		StateSchemaVersion: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Upsert(ctx, board); err != nil {
		t.Fatalf("initial Upsert returned error: %v", err)
	}

	refs, err := q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{OwnerUserID: owner.ID, BoardID: board.BoardID})
	if err != nil {
		t.Fatalf("ListMediaRefsByBoardID returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].MediaAssetID != assetAID || refs[0].RefType != "board_node" || refs[0].RefID != "image-1" {
		t.Fatalf("board refs after add = %#v, want asset A board_node image-1", refs)
	}

	board.State = boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetBID, RefType: "board_node", RefID: "image-2", Role: "result"}})
	board.UpdatedAt = now.Add(time.Second)
	if err := repo.Upsert(ctx, board); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	refs, err = q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{OwnerUserID: owner.ID, BoardID: board.BoardID})
	if err != nil {
		t.Fatalf("ListMediaRefsByBoardID after remove returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].MediaAssetID != assetBID || refs[0].RefID != "image-2" {
		t.Fatalf("board refs after remove = %#v, want only asset B image-2", refs)
	}
	assetARefCount, err := q.CountMediaRefsByAssetID(ctx, assetAID)
	if err != nil {
		t.Fatalf("CountMediaRefsByAssetID returned error: %v", err)
	}
	if assetARefCount != 1 {
		t.Fatalf("asset A refs = %d, want task_output ref to remain", assetARefCount)
	}
	assetA, err := q.GetMediaAssetByID(ctx, assetAID)
	if err != nil {
		t.Fatalf("asset A should not be deleted: %v", err)
	}
	if !assetA.ReclaimAfter.Valid || assetA.ReclaimStartedAt.Valid {
		t.Fatalf("asset A reclamation state = after:%v started:%v, want scheduled and unclaimed", assetA.ReclaimAfter, assetA.ReclaimStartedAt)
	}

	reloaded, err := repo.GetByOwnerAndBoardID(ctx, owner.ID, board.BoardID)
	if err != nil {
		t.Fatalf("GetByOwnerAndBoardID returned error: %v", err)
	}
	var state BoardStateEnvelope
	if err := json.Unmarshal([]byte(reloaded.State), &state); err != nil {
		t.Fatalf("decode reloaded state failed: %v", err)
	}
	if len(state.MediaRefs) != 1 || state.MediaRefs[0].MediaAssetID != assetBID || state.MediaRefs[0].RefType != "board_node" {
		t.Fatalf("reloaded state media refs = %#v, want typed asset B ref", state.MediaRefs)
	}

	if err := repo.DeleteBoardCascade(ctx, owner.ID, board.BoardID); err != nil {
		t.Fatalf("DeleteBoardCascade returned error: %v", err)
	}
	refs, err = q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{OwnerUserID: owner.ID, BoardID: board.BoardID})
	if err != nil {
		t.Fatalf("ListMediaRefsByBoardID after delete returned error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("board refs after delete = %#v, want none", refs)
	}
	assetB, err := q.GetMediaAssetByID(ctx, assetBID)
	if err != nil {
		t.Fatalf("asset B should remain after board cleanup: %v", err)
	}
	if !assetB.ReclaimAfter.Valid {
		t.Fatalf("asset B should be scheduled after board cleanup: %#v", assetB)
	}
}

func TestBoardRepositoryMediaReclamationWaitsForLastBoardReferenceAndCancelsOnUndo(t *testing.T) {
	ctx := context.Background()
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	userRepo := users.NewUserRepository(db)
	q := dbsqlc.New(db)

	owner := &users.User{DisplayName: "shared-media-owner", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("seed owner failed: %v", err)
	}
	assetID := seedBoardMediaAsset(t, q, owner.ID, "shared-board-asset", "uploads/tasks/42/shared/output.png")
	now := time.Now()
	boardA := &UserBoard{BoardID: "shared-a", OwnerUserID: owner.ID, CreatedBy: owner.ID, Name: "Shared A", State: boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetID, RefType: "board_node", RefID: "image-1", Role: "source"}}), StateSchemaVersion: 1, CreatedAt: now, UpdatedAt: now}
	boardB := &UserBoard{BoardID: "shared-b", OwnerUserID: owner.ID, CreatedBy: owner.ID, Name: "Shared B", State: boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetID, RefType: "board_node", RefID: "image-1", Role: "source"}}), StateSchemaVersion: 1, CreatedAt: now, UpdatedAt: now}
	for _, board := range []*UserBoard{boardA, boardB} {
		if err := repo.Upsert(ctx, board); err != nil {
			t.Fatalf("seed board %s failed: %v", board.BoardID, err)
		}
	}
	for _, boardID := range []string{boardA.BoardID, boardB.BoardID} {
		refs, err := q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{OwnerUserID: owner.ID, BoardID: boardID})
		if err != nil || len(refs) != 1 || refs[0].MediaAssetID != assetID {
			t.Fatalf("shared board %s refs=%#v err=%v, want independent media ref", boardID, refs, err)
		}
	}

	boardA.State = boardStateWithMediaRefs(t, nil)
	boardA.UpdatedAt = now.Add(time.Second)
	if err := repo.Upsert(ctx, boardA); err != nil {
		t.Fatalf("remove first reference failed: %v", err)
	}
	asset, err := q.GetMediaAssetByID(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ReclaimAfter.Valid {
		t.Fatalf("asset reclaim_after=%v, want shared board reference to keep it live", asset.ReclaimAfter)
	}

	boardB.State = boardStateWithMediaRefs(t, nil)
	boardB.UpdatedAt = now.Add(2 * time.Second)
	if err := repo.Upsert(ctx, boardB); err != nil {
		t.Fatalf("remove final reference failed: %v", err)
	}
	asset, err = q.GetMediaAssetByID(ctx, assetID)
	if err != nil || !asset.ReclaimAfter.Valid {
		t.Fatalf("asset after last remove = %#v err=%v, want scheduled", asset, err)
	}

	claimNow := time.Now().Add(48 * time.Hour)
	claimToken := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	if _, err := q.ClaimDueMediaAssetReclamation(ctx, dbsqlc.ClaimDueMediaAssetReclamationParams{
		ID:                assetID,
		ReclaimGeneration: asset.ReclaimGeneration,
		ClaimedAt:         sql.NullTime{Time: claimNow, Valid: true},
		ClaimToken:        claimToken,
		NowAt:             sql.NullTime{Time: claimNow, Valid: true},
		StaleClaimBefore:  sql.NullTime{Time: claimNow.Add(-48 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("claim scheduled asset failed: %v", err)
	}
	boardA.State = boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetID, RefType: "board_node", RefID: "image-1", Role: "source"}})
	boardA.UpdatedAt = now.Add(3 * time.Second)
	if err := repo.Upsert(ctx, boardA); !errors.Is(err, ErrMediaAssetNoLongerAvailable) {
		t.Fatalf("undo after reclamation claim error=%v, want ErrMediaAssetNoLongerAvailable", err)
	}
	if err := q.ReleaseMediaAssetReclamationClaim(ctx, dbsqlc.ReleaseMediaAssetReclamationClaimParams{
		ID:                assetID,
		ReclaimGeneration: asset.ReclaimGeneration,
		ClaimToken:        claimToken,
	}); err != nil {
		t.Fatalf("release test reclamation claim: %v", err)
	}
	if err := repo.Upsert(ctx, boardA); err != nil {
		t.Fatalf("undo before deletion failed: %v", err)
	}
	asset, err = q.GetMediaAssetByID(ctx, assetID)
	if err != nil || asset.ReclaimAfter.Valid || asset.ReclaimStartedAt.Valid {
		t.Fatalf("asset after undo = %#v err=%v, want active unscheduled", asset, err)
	}
}

func TestBoardUploadRegistrationConvergesThroughSaveDeleteAndGraceScheduling(t *testing.T) {
	ctx := context.Background()
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	owner := &users.User{DisplayName: "upload-reclamation-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	assetID, err := mediaassets.NewMediaAssetRegistry(db).RegisterBoardUpload(ctx, mediaassets.BoardUploadAsset{
		OwnerUserID: owner.ID,
		BoardID:     "upload-reclamation-board",
		Stored: storage.StoredUploadRef{
			URL:            "/uploads/canvas/owner/upload-reclamation-board/content/image.png",
			StorageKey:     "uploads/canvas/owner/upload-reclamation-board/content/image.png",
			StorageProfile: "builtin-local",
			MIMEType:       "image/png",
			Width:          640,
			Height:         480,
			SizeBytes:      2048,
		},
	})
	if err != nil {
		t.Fatalf("register uploaded media asset: %v", err)
	}
	q := dbsqlc.New(db)
	uploaded, err := q.GetMediaAssetByID(ctx, assetID)
	if err != nil || !uploaded.ReclaimAfter.Valid || uploaded.ReclaimGeneration != 1 {
		t.Fatalf("registered upload = %#v err=%v, want initial generation scheduled", uploaded, err)
	}

	now := time.Now()
	board := &UserBoard{
		BoardID:            "upload-reclamation-board",
		OwnerUserID:        owner.ID,
		CreatedBy:          owner.ID,
		Name:               "Upload Reclamation",
		State:              boardStateWithMediaRefs(t, []BoardStateMediaRef{{MediaAssetID: assetID, RefType: "board_node", RefID: "image-1", Role: "source"}}),
		StateSchemaVersion: CurrentBoardStateSchemaVersion,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Upsert(ctx, board); err != nil {
		t.Fatalf("save board with live upload reference: %v", err)
	}
	live, err := q.GetMediaAssetByID(ctx, assetID)
	if err != nil || live.ReclaimAfter.Valid || live.ReclaimGeneration != 1 {
		t.Fatalf("live upload = %#v err=%v, want initial reclaim cancelled", live, err)
	}

	board.State = boardStateWithMediaRefs(t, nil)
	board.UpdatedAt = now.Add(time.Second)
	if err := repo.Upsert(ctx, board); err != nil {
		t.Fatalf("save board after node deletion: %v", err)
	}
	scheduled, err := q.GetMediaAssetByID(ctx, assetID)
	if err != nil || !scheduled.ReclaimAfter.Valid || scheduled.ReclaimGeneration != 2 {
		t.Fatalf("deleted-node upload = %#v err=%v, want new grace generation", scheduled, err)
	}
}

func TestBoardRepositoryStableBoardIdentityConstraints(t *testing.T) {
	ctx := context.Background()
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	userRepo := users.NewUserRepository(db)

	owner := &users.User{DisplayName: "identity-owner", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("seed owner failed: %v", err)
	}
	creator := &users.User{DisplayName: "identity-creator", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, creator); err != nil {
		t.Fatalf("seed creator failed: %v", err)
	}
	member := &users.User{DisplayName: "identity-member", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, member); err != nil {
		t.Fatalf("seed member failed: %v", err)
	}

	now := time.Now()
	board := &UserBoard{
		BoardID:            "stable-identity-board",
		OwnerUserID:        owner.ID,
		CreatedBy:          creator.ID,
		Name:               "Stable Identity",
		State:              "{}",
		Revision:           1,
		StateSchemaVersion: 1,
		LastSavedBy:        &member.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Upsert(ctx, board); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	reloaded, err := repo.GetByOwnerAndBoardID(ctx, owner.ID, board.BoardID)
	if err != nil {
		t.Fatalf("GetByOwnerAndBoardID returned error: %v", err)
	}
	if reloaded.OwnerUserID != owner.ID || reloaded.CreatedBy != creator.ID || reloaded.LastSavedBy == nil || *reloaded.LastSavedBy != member.ID {
		t.Fatalf("identity fields = owner:%d created:%d saved:%v, want owner:%d created:%d saved:%d", reloaded.OwnerUserID, reloaded.CreatedBy, reloaded.LastSavedBy, owner.ID, creator.ID, member.ID)
	}

	otherOwner := &users.User{DisplayName: "identity-other-owner", Status: 1, Role: "user"}
	if err := userRepo.Create(ctx, otherOwner); err != nil {
		t.Fatalf("seed other owner failed: %v", err)
	}
	conflicting := *board
	conflicting.OwnerUserID = otherOwner.ID
	conflicting.CreatedBy = otherOwner.ID
	conflicting.Name = "Should Not Replace"
	if err := repo.Upsert(ctx, &conflicting); !errors.Is(err, ErrBoardIDConflict) {
		t.Fatalf("conflicting Upsert err = %v, want ErrBoardIDConflict", err)
	}

	if err := repo.UpsertMember(ctx, owner.ID, board.BoardID, member.ID, BoardAccessRoleViewer); err != nil {
		t.Fatalf("UpsertMember viewer returned error: %v", err)
	}
	if err := repo.UpsertMember(ctx, otherOwner.ID, board.BoardID, member.ID, BoardAccessRoleEditor); err != nil {
		t.Fatalf("UpsertMember editor returned error: %v", err)
	}
	var memberRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM board_members WHERE board_id = $1 AND user_id = $2`, board.BoardID, member.ID).Scan(&memberRows); err != nil {
		t.Fatalf("count board_members failed: %v", err)
	}
	if memberRows != 1 {
		t.Fatalf("board member rows = %d, want 1", memberRows)
	}
	role, err := repo.GetMemberRole(ctx, board.BoardID, member.ID)
	if err != nil {
		t.Fatalf("GetMemberRole returned error: %v", err)
	}
	if role != BoardAccessRoleEditor {
		t.Fatalf("member role = %s, want editor", role)
	}

	token := &BoardShareToken{
		BoardID:     board.BoardID,
		TokenHash:   boardShareTokenHash("stable-token"),
		TokenPrefix: "stable-t",
		Role:        string(BoardAccessRoleViewer),
		CreatedBy:   creator.ID,
	}
	if err := repo.CreateShareToken(ctx, token); err != nil {
		t.Fatalf("CreateShareToken returned error: %v", err)
	}
	loadedToken, err := repo.GetShareTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetShareTokenByHash returned error: %v", err)
	}
	if loadedToken.BoardID != board.BoardID || loadedToken.CreatedBy != creator.ID {
		t.Fatalf("share token identity = board:%q created:%d, want board:%q created:%d", loadedToken.BoardID, loadedToken.CreatedBy, board.BoardID, creator.ID)
	}
	duplicateToken := *token
	duplicateToken.ID = 0
	duplicateToken.TokenPrefix = "dupe"
	if err := repo.CreateShareToken(ctx, &duplicateToken); !database.IsDuplicateConstraintError(err) {
		t.Fatalf("duplicate share token err = %v, want duplicate constraint", err)
	}
}

func seedBoardMediaAsset(t *testing.T, q *dbsqlc.Queries, ownerID int64, assetID string, storageKey string) int64 {
	t.Helper()
	id, err := q.UpsertMediaAsset(context.Background(), dbsqlc.UpsertMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      ownerID,
		StorageProfileID: "builtin-local",
		StorageKey:       storageKey,
		Url:              "/uploads/" + storageKey,
		SourceUrl:        "",
		MediaKind:        "image",
		MimeType:         "image/png",
		Width:            64,
		Height:           64,
		SizeBytes:        1024,
		ChecksumSha256:   "",
		Status:           "ready",
		Metadata:         json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("seed media asset %q failed: %v", assetID, err)
	}
	return id
}

func boardStateWithMediaRefs(t *testing.T, refs []BoardStateMediaRef) string {
	t.Helper()
	if refs == nil {
		refs = []BoardStateMediaRef{}
	}
	state := validBoardState(CurrentBoardStateSchemaVersion, 100)
	state["mediaRefs"] = refs
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal board state failed: %v", err)
	}
	return string(payload)
}
