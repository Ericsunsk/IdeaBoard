package tenancy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// userRenameRepository is a minimal UserRepository double for the
// PATCH /user seam: only the profile read and the display-name write
// are exercised; every other method rides the embedded nil interface
// and must never be called by this surface.
type userRenameRepository struct {
	identityports.UserRepository
	users      map[int64]*identitydomain.User
	updatedIDs []int64
	writeErr   error
}

func (r *userRenameRepository) GetByID(_ context.Context, id int64) (*identitydomain.User, error) {
	return r.users[id], nil
}

func (r *userRenameRepository) UpdateDisplayNameByID(_ context.Context, id int64, displayName string) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.updatedIDs = append(r.updatedIDs, id)
	r.users[id].DisplayName = strings.TrimSpace(displayName)
	return nil
}

func newUserRenameEngine(repo *userRenameRepository) *gin.Engine {
	return newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(),
		newStaticUserDirectory(),
		ContractV1Deps{UserProfile: runtimehttp.NewV1UserProfileDirectory(repo)},
	)
}

func seedUserRenameRepo() *userRenameRepository {
	return &userRenameRepository{users: map[int64]*identitydomain.User{
		1: {ID: 1, DisplayName: "旧名字", StorageUID: "ABC123", Role: "user", Status: identitydomain.StatusActive},
		2: {ID: 2, DisplayName: "另一位", StorageUID: "DEF456", Role: "user", Status: identitydomain.StatusActive},
	}}
}

func TestV1UpdateUserRenamesSelf(t *testing.T) {
	repo := seedUserRenameRepo()
	engine := newUserRenameEngine(repo)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPatch, path: "/api/v1/user", userID: 1,
		body: `{"data":{"display_name":"  新名字  "}}`,
	})
	if status != http.StatusOK {
		t.Fatalf("PATCH /user status = %d, body=%v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["display_name"] != "新名字" {
		t.Fatalf("display_name = %v, want trimmed 新名字", data["display_name"])
	}
	if data["id"] != tenancy.FormatUserPublicID(testUUIDForUser(1)) {
		t.Fatalf("id = %v, want the caller's own usr_ identity", data["id"])
	}
	if data["status"] != float64(identitydomain.StatusActive) {
		t.Fatalf("status = %v, want %d", data["status"], identitydomain.StatusActive)
	}
	if personal, _ := data["personal_workspace_id"].(string); !contract.IsWorkspaceID(personal) {
		t.Fatalf("personal_workspace_id = %v, want provisioned ws_...", data["personal_workspace_id"])
	}
	// Only the caller's own row is written; the write is trimmed.
	if repo.users[1].DisplayName != "新名字" {
		t.Fatalf("stored display_name = %q, want 新名字", repo.users[1].DisplayName)
	}
	if repo.users[2].DisplayName != "另一位" {
		t.Fatalf("other user display_name = %q, must stay untouched", repo.users[2].DisplayName)
	}
	if len(repo.updatedIDs) != 1 || repo.updatedIDs[0] != 1 {
		t.Fatalf("updated ids = %v, want exactly [1]", repo.updatedIDs)
	}
}

func TestV1UpdateUserDisplayNameBounds(t *testing.T) {
	for _, tt := range []struct {
		name    string
		display string
		want    int
	}{
		{"exactly column width", strings.Repeat("名", identitydomain.MaxDisplayNameLength), http.StatusOK},
		{"one over column width", strings.Repeat("名", identitydomain.MaxDisplayNameLength+1), http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := seedUserRenameRepo()
			engine := newUserRenameEngine(repo)
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodPatch, path: "/api/v1/user", userID: 1,
				body: fmt.Sprintf(`{"data":{"display_name":%q}}`, tt.display),
			})
			if status != tt.want {
				t.Fatalf("PATCH /user status = %d, want %d (%v)", status, tt.want, payload)
			}
			if tt.want == http.StatusOK {
				if v1Data(t, payload).(map[string]any)["display_name"] != tt.display {
					t.Fatalf("display_name not echoed: %v", payload)
				}
			} else {
				if v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
					t.Fatalf("code = %v, want validation_failed", payload)
				}
				if repo.users[1].DisplayName != "旧名字" {
					t.Fatalf("rejected write changed stored name to %q", repo.users[1].DisplayName)
				}
			}
		})
	}
}

func TestV1UpdateUserRejectsBlankName(t *testing.T) {
	repo := seedUserRenameRepo()
	engine := newUserRenameEngine(repo)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPatch, path: "/api/v1/user", userID: 1,
		body: `{"data":{"display_name":"   "}}`,
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH /user blank status = %d, want 422 (%v)", status, payload)
	}
	if v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("code = %v, want validation_failed", payload)
	}
	fields := v1GapsErrorFields(t, payload)
	if fields["display_name"] == "" {
		t.Fatalf("details.fields.display_name missing: %v", payload)
	}
	if repo.users[1].DisplayName != "旧名字" {
		t.Fatalf("rejected write changed stored name to %q", repo.users[1].DisplayName)
	}
}

func TestV1UpdateUserRequiresLogin(t *testing.T) {
	engine := newUserRenameEngine(seedUserRenameRepo())

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPatch, path: "/api/v1/user",
		body: `{"data":{"display_name":"新名字"}}`,
	})
	if status != http.StatusUnauthorized || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon PATCH /user = %d %v, want 401 unauthorized", status, payload)
	}
}
