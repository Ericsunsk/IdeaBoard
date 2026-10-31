package testsupport

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

var ErrV1BillingRestricted = errors.New("billing restricted")

type FakeLibraryBilling struct {
	mu         sync.Mutex
	Holds      []int64
	Captures   []int64
	Releases   []int64
	NextHoldID int
	HoldErr    error
	CaptureErr error
	ReleaseErr error
}

func (f *FakeLibraryBilling) Hold(ctx context.Context, workspacePublicID string, actorUserID int64, amountCredits int64, idempotencyKey, reason string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.HoldErr != nil {
		return "", f.HoldErr
	}
	f.Holds = append(f.Holds, amountCredits)
	f.NextHoldID++
	return fmt.Sprintf("hold_vision_%d", f.NextHoldID), nil
}

func (f *FakeLibraryBilling) Capture(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CaptureErr != nil {
		return f.CaptureErr
	}
	f.Captures = append(f.Captures, amountCredits)
	return nil
}

func (f *FakeLibraryBilling) Release(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ReleaseErr != nil {
		return f.ReleaseErr
	}
	f.Releases = append(f.Releases, amountCredits)
	return nil
}

type FakeLibraryQuoter struct {
	Amount int64
	Err    error
}

func (f *FakeLibraryQuoter) QuoteVisionQuery(ctx context.Context, workspacePublicID string, actorUserID int64, mediaAssetID string) (int64, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	return f.Amount, nil
}

type MediaGapsFixture struct {
	Store   *tenancy.InMemoryStore
	Media   *workspacemedia.InMemoryWorkspaceMediaStore
	Objects *FakeMediaObjects
	Dir     *StaticUserDirectory
	Engine  *gin.Engine
	Ws      *tenancy.Workspace
	Ws2     *tenancy.Workspace
	Owner   int64
	Member  int64
	Other   int64
}

func (f *MediaGapsFixture) UploadsPath() string {
	return "/api/v1/workspaces/" + f.Ws.PublicID + "/uploads"
}

func (f *MediaGapsFixture) AssetsPath() string {
	return "/api/v1/workspaces/" + f.Ws.PublicID + "/media-assets"
}
