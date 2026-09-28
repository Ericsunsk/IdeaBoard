package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type recordingImpersonationAuditor struct {
	records []audit.RecordParams
	err     error
}

func (f *recordingImpersonationAuditor) AppendRecord(_ context.Context, params audit.RecordParams) error {
	f.records = append(f.records, params)
	return f.err
}

var _ ImpersonationAuditor = (*recordingImpersonationAuditor)(nil)

func serveImpersonateWithAuditor(t *testing.T, handler *AdminHandler, actorID int64, publicIDParam, requestID string) (*httptest.ResponseRecorder, *recordingImpersonationAuditor) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	auditor := &recordingImpersonationAuditor{}
	handler.SetImpersonationAuditor(auditor)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/"+publicIDParam+"/impersonate", nil)
	c.Params = gin.Params{{Key: "public_id", Value: publicIDParam}}
	c.Set(response.RequestIDHeader, requestID)
	testsupport.SetPrincipal(t, c, actorID, principal.RoleAdmin)
	handler.ImpersonateUser(c)
	return recorder, auditor
}

func TestAdminImpersonateUserAuditsEveryAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const targetPublicID = "00000000-0000-0000-0000-000000000001"
	authUserID := "auth-impersonated"

	newTargetService := func(mut func(*User)) *fakeAdminUserService {
		target := &User{ID: 8, PublicID: uuid.MustParse(targetPublicID), DisplayName: "target", AuthUserID: &authUserID, Role: RoleUser, Status: 1}
		if mut != nil {
			mut(target)
		}
		return &fakeAdminUserService{user: target}
	}

	tests := []struct {
		name        string
		publicID    string
		service     func() *fakeAdminUserService
		impersonate *fakeAdminUserImpersonator
		wantStatus  int
		wantOutcome audit.Outcome
		wantReason  string
		wantTarget  string
		wantAuth    string
	}{
		{
			name:        "success",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(nil) },
			impersonate: &fakeAdminUserImpersonator{setCookies: []string{"session=impersonated; Path=/; HttpOnly"}},
			wantStatus:  http.StatusOK,
			wantOutcome: "success",
			wantReason:  "",
			wantTarget:  targetPublicID,
			wantAuth:    authUserID,
		},
		{
			name:        "denied self",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(func(u *User) { u.ID = 7 }) },
			impersonate: &fakeAdminUserImpersonator{},
			wantStatus:  http.StatusBadRequest,
			wantOutcome: "denied",
			wantReason:  "不能模拟当前登录管理员自己",
			wantTarget:  targetPublicID,
		},
		{
			name:        "denied admin target",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(func(u *User) { u.Role = RoleAdmin }) },
			impersonate: &fakeAdminUserImpersonator{},
			wantStatus:  http.StatusBadRequest,
			wantOutcome: "denied",
			wantReason:  "不能模拟管理员账号",
			wantTarget:  targetPublicID,
		},
		{
			name:        "denied missing auth identity",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(func(u *User) { u.AuthUserID = nil }) },
			impersonate: &fakeAdminUserImpersonator{},
			wantStatus:  http.StatusBadRequest,
			wantOutcome: "denied",
			wantReason:  "目标用户缺少 auth_user_id，无法模拟登录",
			wantTarget:  targetPublicID,
		},
		{
			name:        "failure invalid public id",
			publicID:    "not-a-uuid",
			service:     func() *fakeAdminUserService { return newTargetService(nil) },
			impersonate: &fakeAdminUserImpersonator{},
			wantStatus:  http.StatusBadRequest,
			wantOutcome: "failure",
			wantReason:  "无效的用户 public_id",
			wantTarget:  "not-a-uuid",
		},
		{
			name:        "failure target not found",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return &fakeAdminUserService{err: database.ErrNotFound} },
			impersonate: &fakeAdminUserImpersonator{},
			wantStatus:  http.StatusNotFound,
			wantOutcome: "failure",
			wantReason:  "用户不存在",
			wantTarget:  targetPublicID,
		},
		{
			name:        "failure upstream rejected",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(nil) },
			impersonate: &fakeAdminUserImpersonator{err: adminImpersonationStatusError(http.StatusForbidden)},
			wantStatus:  http.StatusForbidden,
			wantOutcome: "failure",
			wantReason:  "认证服务拒绝伪装登录",
			wantTarget:  targetPublicID,
			wantAuth:    authUserID,
		},
		{
			name:        "failure upstream unavailable",
			publicID:    targetPublicID,
			service:     func() *fakeAdminUserService { return newTargetService(nil) },
			impersonate: &fakeAdminUserImpersonator{err: errors.New("upstream failure")},
			wantStatus:  http.StatusBadGateway,
			wantOutcome: "failure",
			wantReason:  "认证服务伪装登录失败",
			wantTarget:  targetPublicID,
			wantAuth:    authUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAdminHandler(tt.service(), nil, nil, tt.impersonate)
			recorder, auditor := serveImpersonateWithAuditor(t, handler, 7, tt.publicID, "req-"+tt.name)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if len(auditor.records) != 1 {
				t.Fatalf("audit records = %d, want exactly 1", len(auditor.records))
			}
			got := auditor.records[0]
			if got.OperatorUserID != 7 {
				t.Errorf("operator = %d, want 7", got.OperatorUserID)
			}
			if got.Operation != "users.impersonate" || got.Capability != "control:users" || got.TargetType != "user" {
				t.Errorf("record identity = %+v, want users.impersonate/control:users/user", got)
			}
			if got.TargetID != tt.wantTarget {
				t.Errorf("target = %q, want %q", got.TargetID, tt.wantTarget)
			}
			if got.AuthUserID != tt.wantAuth {
				t.Errorf("auth_user_id = %q, want %q", got.AuthUserID, tt.wantAuth)
			}
			if got.RequestID != "req-"+tt.name {
				t.Errorf("request_id = %q, want %q", got.RequestID, "req-"+tt.name)
			}
			if got.Outcome != tt.wantOutcome {
				t.Errorf("outcome = %q, want %q", got.Outcome, tt.wantOutcome)
			}
			if got.ErrorMessage != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.ErrorMessage, tt.wantReason)
			}
		})
	}
}

func TestAdminImpersonateUserAuditWriteFailureNeverFailsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const targetPublicID = "00000000-0000-0000-0000-000000000001"
	authUserID := "auth-impersonated"
	handler := NewAdminHandler(
		&fakeAdminUserService{user: &User{ID: 8, AuthUserID: &authUserID, Role: RoleUser, Status: 1}},
		nil, nil,
		&fakeAdminUserImpersonator{setCookies: []string{"session=impersonated; Path=/; HttpOnly"}},
	)
	auditor := &recordingImpersonationAuditor{err: errors.New("audit store down")}
	handler.SetImpersonationAuditor(auditor)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/"+targetPublicID+"/impersonate", nil)
	c.Params = gin.Params{{Key: "public_id", Value: targetPublicID}}
	testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
	handler.ImpersonateUser(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(auditor.records) != 1 || auditor.records[0].Outcome != "success" {
		t.Fatalf("audit records = %+v, want one success attempt", auditor.records)
	}
}

func TestAdminImpersonateUserPersistsDurableAuditRows(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	var actorID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ('audit-actor', 'auth-audit-actor', 'AUDAC1', 'admin', 1)
RETURNING id`).Scan(&actorID); err != nil {
		t.Fatalf("seed actor user: %v", err)
	}
	var targetID int64
	var targetPublicID string
	targetAuthID := "auth-audit-target"
	if err := pool.QueryRow(ctx, `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ('audit-target', $1, 'AUDTG1', 'user', 1)
RETURNING id, public_id::text`, targetAuthID).Scan(&targetID, &targetPublicID); err != nil {
		t.Fatalf("seed target user: %v", err)
	}

	serve := func(t *testing.T, target *User, impersonate *fakeAdminUserImpersonator, requestID string) int {
		t.Helper()
		gin.SetMode(gin.TestMode)
		handler := NewAdminHandler(&fakeAdminUserService{user: target}, nil, nil, impersonate)
		handler.SetImpersonationAuditor(audit.NewStore(pool))
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/"+targetPublicID+"/impersonate", nil)
		c.Params = gin.Params{{Key: "public_id", Value: targetPublicID}}
		c.Set(response.RequestIDHeader, requestID)
		actor, err := principal.New(actorID, uuid.New(), "auth-audit-actor", "audit actor", principal.RoleAdmin.String())
		if err != nil {
			t.Fatalf("create actor principal: %v", err)
		}
		if err := principal.Set(c, actor); err != nil {
			t.Fatalf("set actor principal: %v", err)
		}
		handler.ImpersonateUser(c)
		return recorder.Code
	}

	if code := serve(t, &User{ID: targetID, AuthUserID: &targetAuthID, Role: RoleUser, Status: 1},
		&fakeAdminUserImpersonator{setCookies: []string{"session=impersonated; Path=/; HttpOnly"}}, "req-success"); code != http.StatusOK {
		t.Fatalf("success status = %d, want %d", code, http.StatusOK)
	}
	if code := serve(t, &User{ID: actorID, AuthUserID: &targetAuthID, Role: RoleUser, Status: 1},
		&fakeAdminUserImpersonator{}, "req-denied"); code != http.StatusBadRequest {
		t.Fatalf("denied status = %d, want %d", code, http.StatusBadRequest)
	}
	if code := serve(t, &User{ID: targetID, AuthUserID: &targetAuthID, Role: RoleUser, Status: 1},
		&fakeAdminUserImpersonator{err: errors.New("upstream down")}, "req-failure"); code != http.StatusBadGateway {
		t.Fatalf("failure status = %d, want %d", code, http.StatusBadGateway)
	}

	type auditRow struct {
		operator   int64
		operation  string
		capability string
		targetType string
		targetID   string
		outcome    string
		reason     string
		requestID  string
		authID     string
		publicID   string
	}
	rows, err := pool.Query(ctx, `
SELECT operator_user_id, operation_type, capability, target_type, target_id,
       outcome, error_message, request_correlation_id,
       metadata->>'auth_user_id', public_id
FROM control_audit_records WHERE operator_user_id = $1 ORDER BY internal_id`, actorID)
	if err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	var got []auditRow
	for rows.Next() {
		var row auditRow
		var authID *string
		if err := rows.Scan(&row.operator, &row.operation, &row.capability, &row.targetType,
			&row.targetID, &row.outcome, &row.reason, &row.requestID, &authID, &row.publicID); err != nil {
			rows.Close()
			t.Fatalf("scan audit row: %v", err)
		}
		if authID != nil {
			row.authID = *authID
		}
		got = append(got, row)
	}
	rows.Close()
	if rows.Err() != nil {
		t.Fatalf("iterate audit rows: %v", rows.Err())
	}
	if len(got) != 3 {
		t.Fatalf("audit rows = %+v, want 3 (success, denied, failure)", got)
	}
	want := []auditRow{
		{operator: actorID, operation: "users.impersonate", capability: "control:users", targetType: "user", targetID: targetPublicID, outcome: "success", reason: "", requestID: "req-success", authID: targetAuthID},
		{operator: actorID, operation: "users.impersonate", capability: "control:users", targetType: "user", targetID: targetPublicID, outcome: "denied", reason: "不能模拟当前登录管理员自己", requestID: "req-denied"},
		{operator: actorID, operation: "users.impersonate", capability: "control:users", targetType: "user", targetID: targetPublicID, outcome: "failure", reason: "认证服务伪装登录失败", requestID: "req-failure", authID: targetAuthID},
	}
	for i, w := range want {
		if got[i].operator != w.operator || got[i].operation != w.operation || got[i].capability != w.capability ||
			got[i].targetType != w.targetType || got[i].targetID != w.targetID || got[i].outcome != w.outcome ||
			got[i].reason != w.reason || got[i].requestID != w.requestID || got[i].authID != w.authID {
			t.Errorf("row %d = %+v, want %+v", i, got[i], w)
		}
		if got[i].publicID == "" {
			t.Errorf("row %d missing public_id", i)
		}
	}
	var createdCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE operator_user_id = $1 AND created_at IS NOT NULL`, actorID).Scan(&createdCount); err != nil {
		t.Fatalf("count timestamped rows: %v", err)
	}
	if createdCount != 3 {
		t.Errorf("timestamped rows = %d, want 3", createdCount)
	}
}
