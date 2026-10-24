package control

// Control-plane deterministic user directory for tests (plane split:
// duplicated from v1's staticUserDirectory so control tests stay
// independent of v1; both implement the kernel runtimehttp.V1UserDirectory).
// User N maps to a stable UUID; only ensured ids resolve, unknown uuids
// 404.

import (
	"context"
	"fmt"
	"sync"

	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
)

type staticUserDirectory struct {
	mu     sync.Mutex
	byID   map[int64]uuid.UUID
	byUUID map[uuid.UUID]int64
}

func newStaticUserDirectory() *staticUserDirectory {
	return &staticUserDirectory{byID: map[int64]uuid.UUID{}, byUUID: map[uuid.UUID]int64{}}
}

func testUUIDForUser(userID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("ideaboard-test-user-%d", userID)))
}

func (d *staticUserDirectory) ensure(userID int64) uuid.UUID {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id
	}
	id := testUUIDForUser(userID)
	d.byID[userID] = id
	d.byUUID[id] = userID
	return id
}

func (d *staticUserDirectory) ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if userID, ok := d.byUUID[publicID]; ok {
		return userID, nil
	}
	return 0, tenancy.ErrNotFound
}

func (d *staticUserDirectory) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

func (d *staticUserDirectory) GetUserPublicIDs(ctx context.Context, userIDs []int64) (map[int64]uuid.UUID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[int64]uuid.UUID, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if id, ok := d.byID[userID]; ok {
			out[userID] = id
		}
	}
	return out, nil
}

var _ runtimehttp.V1UserDirectory = (*staticUserDirectory)(nil)
