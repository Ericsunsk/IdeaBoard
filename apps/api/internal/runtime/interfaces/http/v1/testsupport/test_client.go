package testsupport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/tenancy/invitations"
	roottestsupport "ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StaticUserDirectory is a deterministic runtimehttp.V1UserDirectory for tests.
type StaticUserDirectory struct {
	mu                 sync.Mutex
	byID               map[int64]uuid.UUID
	byUUID             map[uuid.UUID]int64
	failPublicIDLookup atomic.Bool
}

var _ runtimehttp.V1UserDirectory = (*StaticUserDirectory)(nil)

func NewStaticUserDirectory() *StaticUserDirectory {
	return &StaticUserDirectory{byID: map[int64]uuid.UUID{}, byUUID: map[uuid.UUID]int64{}}
}

func (d *StaticUserDirectory) SetFailPublicIDLookup(fail bool) {
	d.failPublicIDLookup.Store(fail)
}

func TestUUIDForUser(userID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("ideaboard-test-user-%d", userID)))
}

func testUUIDForUser(userID int64) uuid.UUID {
	return TestUUIDForUser(userID)
}

func (d *StaticUserDirectory) Ensure(userID int64) uuid.UUID {
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

func (d *StaticUserDirectory) ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if userID, ok := d.byUUID[publicID]; ok {
		return userID, nil
	}
	return 0, tenancy.ErrNotFound
}

func (d *StaticUserDirectory) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	if d.failPublicIDLookup.Load() {
		return uuid.Nil, tenancy.ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

func (d *StaticUserDirectory) GetUserPublicIDs(ctx context.Context, userIDs []int64) (map[int64]uuid.UUID, error) {
	if d.failPublicIDLookup.Load() {
		return nil, tenancy.ErrNotFound
	}
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

var testRequestSeq atomic.Int64

type Call struct {
	Method  string
	Path    string
	Body    string
	UserID  int64
	Headers map[string]string
}

func DoRoundTrip(t *testing.T, engine *gin.Engine, call Call, reqIDPrefix string) (int, http.Header, string) {
	t.Helper()
	reader := strings.NewReader(call.Body)
	req := httptest.NewRequest(call.Method, call.Path, reader)
	if call.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	requestID := fmt.Sprintf("%s%d", reqIDPrefix, testRequestSeq.Add(1))
	req.Header.Set(contract.RequestIDHeader, requestID)
	if call.UserID > 0 {
		roottestsupport.SetTestUser(req, call.UserID)
	}
	for key, value := range call.Headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if got := recorder.Header().Get(contract.RequestIDHeader); got != requestID {
		t.Fatalf("%s %s: Request-Id = %q, want %q", call.Method, call.Path, got, requestID)
	}
	return recorder.Code, recorder.Header(), recorder.Body.String()
}

func Do(t *testing.T, engine *gin.Engine, call Call) (int, http.Header, map[string]any) {
	t.Helper()
	status, header, body := DoRoundTrip(t, engine, call, "test-req-")
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("%s %s: response is not JSON: %v; body=%s", call.Method, call.Path, err, body)
	}
	return status, header, payload
}

func Data(t *testing.T, payload map[string]any) any {
	t.Helper()
	data, ok := payload["data"]
	if !ok {
		t.Fatalf("success envelope requires data: %v", payload)
	}
	if _, hasError := payload["error"]; hasError {
		t.Fatalf("success envelope must not carry error: %v", payload)
	}
	return data
}

func ErrorCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %v", payload)
	}
	code, _ := errBody["code"].(string)
	if code == "" {
		t.Fatalf("error envelope requires code: %v", payload)
	}
	requestID, _ := errBody["request_id"].(string)
	if requestID == "" {
		t.Fatalf("error envelope requires request_id: %v", payload)
	}
	if _, hasData := payload["data"]; hasData {
		t.Fatalf("error envelope must not carry data: %v", payload)
	}
	return code
}

func ErrorFields(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %v", payload)
	}
	details, ok := errBody["details"].(map[string]any)
	if !ok {
		t.Fatalf("unavailable error requires details: %v", payload)
	}
	fields, ok := details["fields"].(map[string]any)
	if !ok {
		t.Fatalf("unavailable error requires details.fields: %v", payload)
	}
	return fields
}

func WorkspaceFixture(t *testing.T, ctx context.Context, store *tenancy.InMemoryStore, name string, owner int64) *tenancy.Workspace {
	t.Helper()
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: name, OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

// UserAuthMiddleware provides test auth handling for test engines.
func UserAuthMiddleware(dir *StaticUserDirectory) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(roottestsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(roottestsupport.HeaderTestRole))
		if role == "" {
			role = "user"
		}
		id := dir.Ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	}
}

// NewTestEngine builds a test gin.Engine with contract request ID and test user auth middleware.
func NewTestEngine(dir *StaticUserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(UserAuthMiddleware(dir))
	return router
}

// OrganizationWorkspaceFixture provisions an organization workspace with candidate members.
func OrganizationWorkspaceFixture(t *testing.T, ctx context.Context, store *tenancy.InMemoryStore, name string, owner int64, candidates ...int64) *tenancy.Workspace {
	t.Helper()
	org, err := store.CreateOrganization(ctx, owner, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range candidates {
		if userID == owner {
			continue
		}
		if _, err := store.AddOrganizationMember(ctx, org.PublicID, userID, tenancy.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: name, OwnerUserID: owner, OrganizationID: &org.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

// ConfigureWorkspaceDefaultQuota seeds a billing account for the workspace in PostgreSQL fixtures.
func ConfigureWorkspaceDefaultQuota(t *testing.T, db *pgxpool.Pool, workspace *tenancy.Workspace) {
	t.Helper()
	ctx := context.Background()
	if _, err := tenancypg.NewBillingAccountStore(db).EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID); err != nil {
		t.Fatal(err)
	}
}

// TestInvitations provisions an invitations.Service for transport tests.
func TestInvitations(store tenancy.Store, transfers invitations.TransferStore, events tenancy.CollaborationEventDispatcher, identities invitations.IdentityReader) *invitations.Service {
	if events == nil {
		events = tenancyevents.NewCollaborationEventDispatcher(notifications.NewMemoryStore(), nil)
	}
	return &invitations.Service{
		Store: store, Transfers: transfers, Events: events, Identities: identities,
		Transaction: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	}
}

// V1MemberLookupProvider matches the member lookup seam.
type V1MemberLookupProvider interface {
	LookupUserPublicID(ctx context.Context, email string) (uuid.UUID, error)
}

type invitationIdentityReader struct {
	display shared.MemberDisplayProvider
	lookup  V1MemberLookupProvider
	users   runtimehttp.V1UserDirectory
}

// NewInvitationIdentityReader adapts trusted identity readers to the Invitation module for tests.
func NewInvitationIdentityReader(display shared.MemberDisplayProvider, lookup V1MemberLookupProvider, users runtimehttp.V1UserDirectory) invitations.IdentityReader {
	return invitationIdentityReader{display: display, lookup: lookup, users: users}
}

func (r invitationIdentityReader) Email(ctx context.Context, userID int64) (string, error) {
	if r.display == nil {
		return "", errors.New("invitation identity: display reader unavailable")
	}
	displays, err := r.display.GetMemberDisplay(ctx, []int64{userID})
	if err != nil {
		return "", err
	}
	return displays[userID].Email, nil
}

func (r invitationIdentityReader) FindUser(ctx context.Context, email string) (int64, error) {
	if r.lookup == nil || r.users == nil {
		return 0, errors.New("invitation identity: user lookup unavailable")
	}
	publicID, err := r.lookup.LookupUserPublicID(ctx, email)
	if err != nil || publicID == uuid.Nil {
		return 0, err
	}
	return r.users.ResolveUserID(ctx, publicID)
}

type memberDisplayService struct {
	repo   identityports.UserRepository
	emails identityports.UserEmailReader
}

// NewMemberDisplayService creates a MemberDisplayProvider for tests.
func NewMemberDisplayService(repo identityports.UserRepository, emails identityports.UserEmailReader) shared.MemberDisplayProvider {
	return &memberDisplayService{repo: repo, emails: emails}
}

func (s *memberDisplayService) GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]shared.MemberDisplay, error) {
	out := make(map[int64]shared.MemberDisplay)
	if s == nil || s.repo == nil {
		return out, nil
	}
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return out, nil
	}
	usersByID, err := s.repo.GetByIDs(ctx, deduped)
	if err != nil {
		return nil, err
	}
	return memberDisplayForUsers(ctx, usersByID, s.emails)
}

func memberDisplayForUsers(ctx context.Context, usersByID map[int64]*identitydomain.User, emails identityports.UserEmailReader) (map[int64]shared.MemberDisplay, error) {
	out := make(map[int64]shared.MemberDisplay, len(usersByID))
	for numeric, user := range usersByID {
		if user == nil {
			continue
		}
		if url := runtimehttp.V1PublicAvatarURLForRow(user.StorageUID, user.Avatar); url != "" {
			entry := out[numeric]
			entry.Avatar = url
			out[numeric] = entry
		}
	}
	authIDs := make([]string, 0, len(usersByID))
	authSeen := make(map[string]struct{}, len(usersByID))
	authByNumeric := make(map[int64]string, len(usersByID))
	for numeric, user := range usersByID {
		if user == nil || user.AuthUserID == nil {
			continue
		}
		authID := strings.TrimSpace(*user.AuthUserID)
		if authID == "" {
			continue
		}
		authByNumeric[numeric] = authID
		if _, ok := authSeen[authID]; !ok {
			authSeen[authID] = struct{}{}
			authIDs = append(authIDs, authID)
		}
	}
	if len(authIDs) == 0 || emails == nil {
		return out, nil
	}
	identities, err := emails.GetUserEmails(ctx, authIDs)
	if err != nil {
		return nil, err
	}
	for numeric, authID := range authByNumeric {
		identity, ok := identities[authID]
		if !ok {
			continue
		}
		if email := strings.TrimSpace(identity.Email); email != "" {
			entry := out[numeric]
			entry.Email = email
			out[numeric] = entry
		}
	}
	return out, nil
}

// FakeMemberDisplay is a deterministic MemberDisplayProvider for tests.
type FakeMemberDisplay struct {
	mu       sync.Mutex
	Facts    map[int64]shared.MemberDisplay
	FailNext error
	Calls    int
}

func NewFakeMemberDisplay() *FakeMemberDisplay {
	return &FakeMemberDisplay{Facts: make(map[int64]shared.MemberDisplay)}
}

func (f *FakeMemberDisplay) GetMemberDisplay(_ context.Context, userIDs []int64) (map[int64]shared.MemberDisplay, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if f.FailNext != nil {
		err := f.FailNext
		f.FailNext = nil
		return nil, err
	}
	out := make(map[int64]shared.MemberDisplay, len(userIDs))
	for _, id := range userIDs {
		if fact, ok := f.Facts[id]; ok {
			out[id] = fact
		}
	}
	return out, nil
}

// TestImgproxyConfig provides a signed renderer configuration that echoes its requested path.
func TestImgproxyConfig(t *testing.T) imgproxy.Config {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	t.Cleanup(upstream.Close)
	return imgproxy.Config{BaseURL: upstream.URL, KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
}

// FakeMediaObjects is a map-backed MediaObjects fake for transport tests.
type FakeMediaObjects struct {
	mu       sync.Mutex
	objects  map[string][]byte
	serveAny bool
}

func NewFakeMediaObjects() *FakeMediaObjects {
	return &FakeMediaObjects{objects: make(map[string][]byte)}
}

func (f *FakeMediaObjects) SetServeAny(val bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.serveAny = val
}

func (f *FakeMediaObjects) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data, ok := f.objects[objectKey]; ok && len(data) > 0 {
		return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: append([]byte(nil), data...)}, nil
	}
	if f.serveAny {
		return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: []byte("fake-png-bytes")}, nil
	}
	return nil, storagedomain.ErrStoredObjectNotFound
}

func (f *FakeMediaObjects) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[objectKey] = append([]byte(nil), data...)
	return nil
}

func (f *FakeMediaObjects) StoreBytesRef(ctx context.Context, filename string, data []byte, opts storageports.StorageStoreOptions) (*storageports.StoredUploadRef, error) {
	if strings.TrimSpace(opts.ObjectKey) == "" {
		return nil, storagedomain.ErrStoredObjectNotFound
	}
	if err := f.PutObjectAtProfile(ctx, "", opts.ObjectKey, data, "image/png"); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return &storageports.StoredUploadRef{
		StorageKey:     strings.TrimSpace(opts.ObjectKey),
		MIMEType:       "image/png",
		SizeBytes:      int64(len(data)),
		ChecksumSHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func (f *FakeMediaObjects) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, objectKey)
	return nil
}

func GapsChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// GapsPNGBytes pads a valid PNG to the requested size.
func GapsPNGBytes(size int) []byte {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		panic(err)
	}
	out := make([]byte, max(size, encoded.Len()))
	copy(out, encoded.Bytes())
	return out
}

func GapsNewUploadBody(filename, checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"filename":%q,"mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"}}`,
		filename, size, checksum)
}

func GapsFinishUploadBody(checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"mime_type":"image/png","media_kind":"image","width":100,"height":80}}`,
		checksum, size)
}

func MediaUploadID(t *testing.T, data any) string {
	t.Helper()
	dto, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("upload dto = %T, want object", data)
	}
	id, _ := dto["id"].(string)
	if !contract.IsUploadID(id) {
		t.Fatalf("upload id = %q, want upload_ shape", id)
	}
	return id
}

func MediaAssetID(t *testing.T, data any) string {
	t.Helper()
	dto, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("asset dto = %T, want object", data)
	}
	id, _ := dto["id"].(string)
	return id
}

func GapsDeclare(t *testing.T, engine *gin.Engine, uploadsPath string, userID int64, key, filename string, body []byte) string {
	t.Helper()
	status, _, payload := Do(t, engine, Call{
		Method: "POST", Path: uploadsPath,
		Body:   GapsNewUploadBody(filename, GapsChecksum(body), len(body)),
		UserID: userID, Headers: map[string]string{"Idempotency-Key": key},
	})
	if status != http.StatusCreated {
		t.Fatalf("declare = %d %v, want 201", status, payload)
	}
	return MediaUploadID(t, Data(t, payload))
}

func GapsStage(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) {
	t.Helper()
	status, _, payload := Do(t, engine, Call{
		Method: "PUT", Path: path, Body: string(body),
		UserID: userID, Headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": key},
	})
	if status != http.StatusOK {
		t.Fatalf("stage bytes = %d %v, want 200", status, payload)
	}
}

func GapsComplete(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) string {
	t.Helper()
	status, _, payload := Do(t, engine, Call{
		Method: "POST", Path: path,
		Body:   GapsFinishUploadBody(GapsChecksum(body), len(body)),
		UserID: userID, Headers: map[string]string{"Idempotency-Key": key},
	})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return MediaAssetID(t, Data(t, payload).(map[string]any)["asset"])
}

// StaticMemberDisplayProvider is a memory-backed shared.MemberDisplayProvider for tests.
type StaticMemberDisplayProvider struct {
	Emails  map[int64]string
	Avatars map[int64]string
}

func NewStaticMemberDisplayProvider(emails map[int64]string, avatars map[int64]string) *StaticMemberDisplayProvider {
	if emails == nil {
		emails = make(map[int64]string)
	}
	if avatars == nil {
		avatars = make(map[int64]string)
	}
	return &StaticMemberDisplayProvider{Emails: emails, Avatars: avatars}
}

func (p *StaticMemberDisplayProvider) GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]shared.MemberDisplay, error) {
	out := make(map[int64]shared.MemberDisplay)
	for _, id := range userIDs {
		out[id] = shared.MemberDisplay{
			Email:  p.Emails[id],
			Avatar: p.Avatars[id],
		}
	}
	return out, nil
}

// TestInvitationReader mirrors public identity/display facts supplied by transport fixtures.
func TestInvitationReader(dir runtimehttp.V1UserDirectory, display shared.MemberDisplayProvider) invitations.Reader {
	return invitations.Reader{People: func(ctx context.Context, ids []int64) map[int64]invitations.Person {
		people := make(map[int64]invitations.Person, len(ids))
		publicIDs, _ := dir.GetUserPublicIDs(ctx, ids)
		displays := shared.BatchDisplayFacts(ctx, display, ids)
		for _, id := range ids {
			person := invitations.Person{Email: displays[id].Email, Avatar: displays[id].Avatar}
			if publicID := publicIDs[id]; publicID != uuid.Nil {
				person.PublicID = tenancy.FormatUserPublicID(publicID)
			}
			people[id] = person
		}
		return people
	}}
}
