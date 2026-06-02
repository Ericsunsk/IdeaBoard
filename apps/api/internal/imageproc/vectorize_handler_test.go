package imageproc

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/internal/boards"
	"api/internal/platform/settings"
	"api/internal/storage"
	"api/internal/testdb"
	"api/internal/users"
)

func TestVectorizeLocalUploadImageRequiresWorkspaceAccess(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	mustWriteVectorizeTestFile(t, filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "content", "source.png"), minimalPNGBytes())

	db, userRepo, boardService := newVectorizeAccessTestDeps(t)
	owner := mustCreateVectorizeUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateVectorizeUser(t, userRepo, "member", "member-auth", "MEMB1A")
	outsider := mustCreateVectorizeUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")
	boardRepo := boards.NewBoardRepository(db)
	now := time.Now()
	if err := boardRepo.Upsert(ctx, &boards.UserBoard{BoardID: "board-1", UserID: owner.ID, Name: "Demo", State: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if err := boardRepo.UpsertMember(ctx, "board-1", member.ID, boards.BoardAccessRoleViewer); err != nil {
		t.Fatalf("create board member failed: %v", err)
	}

	handler := NewVectorizeHandler(nil, nil, VectorizeDeps{
		UploadBoards: testVectorizeUploadBoardAccessResolver{service: boardService},
		UploadOwners: testVectorizeUploadOwnerStorageUIDResolver{repo: userRepo},
	})
	data, err := handler.loadLocalUploadImage(ctx, member.ID, "/uploads/workspace/OWNR1A/board-1/content/source.png")
	if err != nil {
		t.Fatalf("expected member to load upload: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected image data")
	}
	if _, err := handler.loadLocalUploadImage(ctx, outsider.ID, "/uploads/workspace/OWNR1A/board-1/content/source.png"); err == nil {
		t.Fatalf("expected outsider to be rejected")
	}
	if _, err := handler.loadLocalUploadImage(ctx, member.ID, "/uploads/workspace/WRONG1/board-1/content/source.png"); err == nil {
		t.Fatalf("expected owner segment mismatch to be rejected")
	}
}

func TestVectorizeLocalUploadImageRejectsLegacyGenericUploadPath(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	mustWriteVectorizeTestFile(t, filepath.Join("uploads", "2026", "05", "13", "source.png"), minimalPNGBytes())

	_, userRepo, boardService := newVectorizeAccessTestDeps(t)
	user := mustCreateVectorizeUser(t, userRepo, "member", "member-auth", "MEMB1A")
	handler := NewVectorizeHandler(nil, nil, VectorizeDeps{
		UploadBoards: testVectorizeUploadBoardAccessResolver{service: boardService},
		UploadOwners: testVectorizeUploadOwnerStorageUIDResolver{repo: userRepo},
	})

	if _, err := handler.loadLocalUploadImage(ctx, user.ID, "/uploads/2026/05/13/source.png"); err == nil {
		t.Fatalf("expected legacy generic upload path to be rejected")
	}
}

func TestVectorizePersistSVGPutsOutputUnderUserProfilePrefix(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	db, userRepo, _ := newVectorizeAccessTestDeps(t)
	user := mustCreateVectorizeUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	configRepo := settings.NewSystemConfigRepository(db)
	storageService := storage.NewStorageService(configRepo)
	handler := NewVectorizeHandler(nil, storageService, VectorizeDeps{
		UploadOwners: testVectorizeUploadOwnerStorageUIDResolver{repo: userRepo},
	})

	url, err := handler.persistSVG(ctx, user.ID, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	if err != nil {
		t.Fatalf("persist svg failed: %v", err)
	}
	if !strings.HasPrefix(url, "/uploads/profile/OWNR1A/vector/") {
		t.Fatalf("svg url = %q, want profile vector prefix", url)
	}
}

type testVectorizeUploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r testVectorizeUploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID uint64, boardID string) (uint64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.UserID, nil
}

type testVectorizeUploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r testVectorizeUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID uint64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}

func newVectorizeAccessTestDeps(t *testing.T) (*sql.DB, *users.UserRepository, *boards.BoardService) {
	t.Helper()
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	boardService := boards.NewBoardService(boards.NewBoardRepository(db), userRepo)
	return db, userRepo, boardService
}

func mustCreateVectorizeUser(t *testing.T, repo *users.UserRepository, username string, authUserID string, storageUID string) *users.User {
	t.Helper()
	user := &users.User{
		Username:   username,
		AuthUserID: &authUserID,
		StorageUID: storageUID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %s failed: %v", username, err)
	}
	return user
}

func mustWriteVectorizeTestFile(t *testing.T, name string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(name), err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatalf("write %s failed: %v", name, err)
	}
}

func minimalPNGBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}
