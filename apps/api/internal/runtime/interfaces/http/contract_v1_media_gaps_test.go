package httpapi

// Backend gap batch for #809-close: media/upload/avatar/vision-query
// coverage (declare -> PUT bytes -> complete -> asset, avatar,
// batch metadata, vision file/external refs, incomplete-complete
// rejection). Uses serveAny=false byte fakes so unstaged slots
// genuinely miss; the permissive serveAny fixture in
// contract_v1_media_test.go cannot prove the complete-after-bytes
// gate.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/users/domain"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// fakeAvatarUID is a deterministic V1AvatarUIDDirectory for tests.
type fakeAvatarUID struct {
	uid          string
	err          error
	avatar       string
	avatarUserID int64
	writeErr     error
	writes       int
}

// errFakeAvatarUserNotFound is the fixture's unknown-uid signal. It
// aliases the platform not-found sentinel so the public avatar route
// converges it to 404 exactly like production.
var errFakeAvatarUserNotFound = database.ErrNotFound

func (f *fakeAvatarUID) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if strings.TrimSpace(f.uid) == "" {
		return "", errV1AvatarUIDUnavailable
	}
	return f.uid, nil
}

func (f *fakeAvatarUID) UpdateUserAvatar(ctx context.Context, userID int64, avatar string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.avatar, f.avatarUserID = avatar, userID
	f.writes++
	return nil
}

// GetUserByID keeps fakeAvatarUID satisfying the extended
// V1AvatarProfileDirectory (storage/status/timestamps projection).
// Unknown users project an empty row; only the avatar-owning user
// carries the persisted URL.
func (f *fakeAvatarUID) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	user := &domain.User{ID: userID, StorageUID: f.uid, Status: domain.StatusActive}
	if f.avatarUserID == userID {
		user.Avatar = f.avatar
	}
	return user, nil
}

// GetUserByStorageUID resolves the same single-uid fixture by storage
// namespace: a mismatched uid reads as not-found (the public avatar
// route converges it to 404).
func (f *fakeAvatarUID) GetUserByStorageUID(ctx context.Context, storageUID string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if strings.TrimSpace(storageUID) == "" || strings.TrimSpace(f.uid) == "" ||
		strings.TrimSpace(storageUID) != strings.TrimSpace(f.uid) {
		return nil, errFakeAvatarUserNotFound
	}
	return f.GetUserByID(ctx, f.avatarUserID)
}

type v1MediaGapsFixture struct {
	store   *tenancy.InMemoryStore
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeV1MediaObjects
	dir     *staticUserDirectory
	engine  *gin.Engine
	ws      *tenancy.Workspace
	ws2     *tenancy.Workspace
	owner   int64
	member  int64
	other   int64
}

func newV1MediaGapsFixture(t *testing.T, avatarUID V1AvatarUIDDirectory) *v1MediaGapsFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	// serveAny=false: only PUT-staged keys read back; unstaged slots
	// miss so the complete-after-bytes gate is genuinely exercised.
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-gaps", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	ws2, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-gaps-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws2.PublicID, owner, tenancy.RoleMember)
	deps := ContractV1Deps{Media: media, MediaObjects: objects, AvatarUID: avatarUID, AvatarObjects: objects, MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}})}
	if profile, ok := avatarUID.(V1AvatarProfileDirectory); ok {
		deps.AvatarProfile = profile
	}
	engine := newV1MediaTestEngine(store, dir, deps)
	return &v1MediaGapsFixture{store: store, media: media, objects: objects, dir: dir, engine: engine,
		ws: ws, ws2: ws2, owner: owner, member: member, other: other}
}

func (f *v1MediaGapsFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaGapsFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

// v1GapsPNGBytes builds deterministic sniff-valid PNG bytes of exactly
// size bytes (magic header + zero body). The complete gate sniffs
// staged bytes, so fixtures must carry real format magic.
func v1GapsPNGBytes(size int) []byte {
	if size < 8 {
		size = 8
	}
	out := make([]byte, size)
	copy(out, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	return out
}

// v1GapsGIFBytes builds deterministic sniff-valid GIF bytes.
func v1GapsGIFBytes(size int) []byte {
	if size < 6 {
		size = 6
	}
	out := make([]byte, size)
	copy(out, []byte("GIF89a"))
	return out
}

func v1GapsChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// v1GapsErrorFields extracts error.details.fields from an error
// envelope for field-level assertions.
func v1GapsErrorFields(t *testing.T, payload map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	errBody, _ := payload["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	fields, _ := details["fields"].(map[string]any)
	for k, v := range fields {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// v1GapsJSONStringArray renders a JSON string array for batch bodies.
func v1GapsJSONStringArray(ids []string) string {
	var sb strings.Builder
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%q", id)
	}
	return sb.String()
}

func v1GapsNewUploadBody(filename, checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"filename":%q,"mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"}}`,
		filename, size, checksum)
}

func v1GapsFinishUploadBody(checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"mime_type":"image/png","media_kind":"image","width":100,"height":80}}`,
		checksum, size)
}

// v1GapsDeclare creates a session whose declaration matches body.
func v1GapsDeclare(t *testing.T, engine *gin.Engine, uploadsPath string, userID int64, key, filename string, body []byte) string {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "POST", path: uploadsPath,
		body:   v1GapsNewUploadBody(filename, v1GapsChecksum(body), len(body)),
		userID: userID, headers: map[string]string{"Idempotency-Key": key}})
	if status != http.StatusCreated {
		t.Fatalf("declare = %d %v, want 201", status, payload)
	}
	return v1MediaUploadID(t, v1Data(t, payload))
}

// v1GapsStage PUTs raw bytes and asserts the 200 session envelope.
func v1GapsStage(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "PUT", path: path, body: string(body),
		userID: userID, headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": key}})
	if status != http.StatusOK {
		t.Fatalf("stage bytes = %d %v, want 200", status, payload)
	}
}

// v1GapsComplete completes a staged session and returns the asset id.
func v1GapsComplete(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) string {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "POST", path: path,
		body:   v1GapsFinishUploadBody(v1GapsChecksum(body), len(body)),
		userID: userID, headers: map[string]string{"Idempotency-Key": key}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])
}

// TestV1UploadBytesFullLifecycle proves the real transfer path end to
// end: declare -> PUT bytes -> complete -> asset -> content serves the
// staged bytes -> derivatives -> archive.
func TestV1UploadBytesFullLifecycle(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(1024)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-life-create", "photo.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.member, "gaps-life-stage")
	assetID := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.member, "gaps-life-complete")

	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "test-gaps-")
	if status != http.StatusOK || body != string(staged) {
		t.Fatalf("content = %d len %d, want 200 staged bytes len %d", status, len(body), len(staged))
	}
	assertV1DisplayTierImage(t, f.engine, v1TestImgproxyConfig(t),
		f.assetsPath()+"/"+assetID+"/derivatives/thumb", f.member,
		derivativedomain.TierThumb, derivativedomain.TierSpecs[derivativedomain.TierThumb].MaxSide)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/" + assetID + "/archive",
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-life-archive"}})
	if status != http.StatusOK {
		t.Fatalf("archive = %d %v, want 200", status, payload)
	}
}

// TestV1CompleteWithoutStagedBytesRejected proves declare+complete
// without the byte PUT fails loudly (422, nothing materialized) and
// stays retryable: PUT then complete succeeds on the same session.
func TestV1CompleteWithoutStagedBytesRejected(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(512)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-nostage-create", "photo.png", staged)
	completeBody := v1GapsFinishUploadBody(v1GapsChecksum(staged), len(staged))
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST",
		path: f.uploadsPath() + "/" + uploadID + "/complete", body: completeBody,
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-nostage-complete"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("incomplete complete = %d %v, want 422 upload_validation_failed", status, payload)
	}
	if fields := v1GapsErrorFields(t, payload); fields["bytes"] == "" {
		t.Fatalf("incomplete complete fields = %v, want bytes=missing", fields)
	}

	// Nothing materialized: the asset list stays empty.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	if items, _ := v1Data(t, payload).([]any); len(items) != 0 {
		t.Fatalf("list after rejected complete = %v, want empty", items)
	}

	// The session stays live (transferring) for the real PUT...
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET",
		path: f.uploadsPath() + "/" + uploadID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get session = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "transferring" {
		t.Fatalf("session status = %q, want transferring (retryable)", got)
	}

	// ...after which the same completion succeeds.
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.owner, "gaps-nostage-stage")
	assetID := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.owner, "gaps-nostage-complete-2")
	if !contract.IsAssetID(assetID) {
		t.Fatalf("recovered asset id = %q, want asset_ shape", assetID)
	}
}

// TestV1CompleteCorruptedStagedBytesFailsSession proves backend
// corruption/eviction between PUT and complete fails loudly AND
// terminally (mirroring completion-echo mismatch semantics).
func TestV1CompleteCorruptedStagedBytesFailsSession(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(256)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-corrupt-create", "photo.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.owner, "gaps-corrupt-stage")

	// Corrupt the staged object out of band (eviction/corruption analog).
	session, err := f.media.GetSession(context.Background(), f.ws.PublicID, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	f.objects.mu.Lock()
	f.objects.objects[session.StorageKey] = v1GapsGIFBytes(len(staged))
	f.objects.mu.Unlock()

	completeBody := v1GapsFinishUploadBody(v1GapsChecksum(staged), len(staged))
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST",
		path: f.uploadsPath() + "/" + uploadID + "/complete", body: completeBody,
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-corrupt-complete"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("corrupt complete = %d %v, want 422 upload_validation_failed", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET",
		path: f.uploadsPath() + "/" + uploadID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get session = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "failed" {
		t.Fatalf("session status = %q, want failed", got)
	}
}

// TestV1PutUploadBytesValidation covers the byte-PUT contract: exact
// size echo, checksum echo, sniffed-MIME echo, terminal-state 409,
// idempotent replay/conflict, and the 404-vs-400 matrix.
func TestV1PutUploadBytesValidation(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(1024)
	bytesPath := func(uploadID string) string { return f.uploadsPath() + "/" + uploadID + "/bytes" }
	put := func(uploadID string, body []byte, userID int64, key, contentType string) (int, map[string]any) {
		headers := map[string]string{"Idempotency-Key": key}
		if contentType != "" {
			headers["Content-Type"] = contentType
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: bytesPath(uploadID),
			body: string(body), userID: userID, headers: headers})
		return status, payload
	}

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-put-create", "photo.png", staged)

	// Wrong size (short body, same declaration).
	if status, payload := put(uploadID, staged[:100], f.owner, "gaps-put-short", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("short PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Same size, different content (checksum mismatch).
	mutated := append([]byte(nil), staged...)
	mutated[len(mutated)-1] ^= 0xFF
	if status, payload := put(uploadID, mutated, f.owner, "gaps-put-badsum", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("checksum-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Same size, wrong format (GIF bytes against a PNG declaration).
	if status, payload := put(uploadID, v1GapsGIFBytes(len(staged)), f.owner, "gaps-put-badmime", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("mime-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Header lies while bytes are right.
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-badheader", "image/gif"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("header-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Empty body.
	if status, payload := put(uploadID, nil, f.owner, "gaps-put-empty", "image/png"); status != http.StatusUnprocessableEntity {
		t.Fatalf("empty PUT = %d %v, want 422", status, payload)
	}

	// Happy-path PUT, then idempotent replay + conflict.
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-ok", "image/png"); status != http.StatusOK {
		t.Fatalf("PUT = %d %v, want 200", status, payload)
	}
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-ok", "image/png"); status != http.StatusOK {
		t.Fatalf("PUT replay = %d %v, want 200", status, payload)
	}
	other := v1GapsPNGBytes(512)
	otherID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-put-other", "other.png", other)
	// Same key against a different (valid) session: hash diverges → 409.
	otherHeaders := map[string]string{"Content-Type": "image/png", "Idempotency-Key": "gaps-put-ok"}
	status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: bytesPath(otherID),
		body: string(other), userID: f.owner, headers: otherHeaders})
	if status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadIdempotencyConflict) {
		t.Fatalf("PUT conflict = %d %v, want 409 upload_idempotency_conflict", status, payload)
	}

	// Missing key, malformed id, outsider, and terminal-state PUT.
	noKeyCall := v1Call{method: "PUT", path: bytesPath(uploadID), body: string(staged),
		userID: f.owner, headers: map[string]string{"Content-Type": "image/png"}}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless PUT = %d %v, want 200 (server-minted content-hash key)", status, payload)
	}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless PUT replay = %d %v, want 200", status, payload)
	}
	// A present-but-malformed (overlong) key still fails closed with 400.
	if status, payload := put(uploadID, staged, f.owner, strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), "image/png"); status != http.StatusBadRequest ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("overlong key PUT = %d %v, want 400 idempotency_key_required", status, payload)
	}
	if status, payload := put("nope", staged, f.owner, "gaps-put-malformed", "image/png"); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("malformed PUT = %d %v, want 404 upload_not_found", status, payload)
	}
	if status, payload := put(uploadID, staged, f.other, "gaps-put-outsider", "image/png"); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider PUT = %d %v, want 404 workspace_not_found", status, payload)
	}
	v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.owner, "gaps-put-then-complete")
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-terminal", "image/png"); status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadStateConflict) {
		t.Fatalf("terminal PUT = %d %v, want 409 upload_state_conflict", status, payload)
	}
}

// TestV1AvatarUpload covers the user-scoped avatar PUT: sniffed MIME +
// size validation, canonical content-hashed keying through the storage
// abstraction, public-route-URL projection, idempotency, superseded-key
// cleanup, and the fail-closed seams.
func TestV1AvatarUpload(t *testing.T) {
	profile := &fakeAvatarUID{uid: "AVTR01"}
	f := newV1MediaGapsFixture(t, profile)
	avatar := v1GapsPNGBytes(2048)
	wantKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(avatar)[:16] + ".png"
	wantURL := v1PublicAvatarURLForRow("AVTR01", wantKey)
	put := func(body []byte, userID int64, key, contentType string) (int, map[string]any) {
		headers := map[string]string{"Idempotency-Key": key}
		if contentType != "" {
			headers["Content-Type"] = contentType
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: "/api/v1/user/avatar",
			body: string(body), userID: userID, headers: headers})
		return status, payload
	}

	status, payload := put(avatar, f.owner, "avatar-1", "image/png")
	if status != http.StatusOK {
		t.Fatalf("avatar PUT = %d %v, want 200", status, payload)
	}
	url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string)
	if url != wantURL {
		t.Fatalf("avatar_url = %q, want the public avatar route %q", url, wantURL)
	}
	// The row keeps the canonical storage key, never the route URL.
	if profile.avatar != wantKey || profile.avatarUserID != f.owner {
		t.Fatalf("avatar projection = %q for %d, want key %q for owner", profile.avatar, profile.avatarUserID, wantKey)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["avatar"] != wantURL {
		t.Fatalf("user avatar projection = %d %v, want the public route URL", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.other})
	if _, leaked := v1Data(t, payload).(map[string]any)["avatar"]; status != http.StatusOK || leaked {
		t.Fatalf("cross-user avatar = %d %v, want no avatar", status, payload)
	}
	// The staged object is readable through the byte seam at that key.
	object, err := f.objects.ReadObjectAtProfile(context.Background(), workspacemedia.DefaultStorageProfile,
		wantKey, v1AvatarMaxBytes)
	if err != nil || string(object.Data) != string(avatar) {
		t.Fatalf("staged avatar = %v %v, want the PUT bytes", object, err)
	}

	// Replay converges; a different body under the same key conflicts.
	if status, _ := put(avatar, f.owner, "avatar-1", "image/png"); status != http.StatusOK {
		t.Fatalf("avatar replay = %d, want 200", status)
	}
	if profile.writes != 1 {
		t.Fatalf("replay avatar writes = %d, want 1", profile.writes)
	}
	if status, payload := put(v1GapsPNGBytes(1024), f.owner, "avatar-1", "image/png"); status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaIdempotencyConflict) {
		t.Fatalf("avatar conflict = %d %v, want 409 media_idempotency_conflict", status, payload)
	}

	// Non-image bytes, oversize body, anonymous, missing key.
	if status, payload := put([]byte("not an image at all...."), f.owner, "avatar-bad", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("bad avatar = %d %v, want 422 media_validation_failed", status, payload)
	}
	if status, payload := put(make([]byte, v1AvatarMaxBytes+1), f.owner, "avatar-big", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("oversize avatar = %d %v, want 422 media_validation_failed", status, payload)
	}
	anonCall := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar),
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-anon"}}
	if status, _, _ := doV1(t, f.engine, anonCall); status != http.StatusUnauthorized {
		t.Fatalf("anonymous avatar = %d, want 401", status)
	}
	noKeyCall := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar),
		userID: f.owner, headers: map[string]string{"Content-Type": "image/png"}}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless avatar = %d %v, want 200 (server-minted content-hash key)", status, payload)
	} else if url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string); url != wantURL {
		t.Fatalf("keyless avatar_url = %q, want the public avatar route", url)
	}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless avatar replay = %d %v, want 200", status, payload)
	}
	if status, payload := put(avatar, f.owner, strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), "image/png"); status != http.StatusBadRequest ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("overlong key avatar = %d %v, want 400 idempotency_key_required", status, payload)
	}

	// A replaced avatar stores under its own content key, projects the
	// same route URL, and removes the superseded object best-effort.
	replacement := v1GapsGIFBytes(2048)
	replacementKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(replacement)[:16] + ".gif"
	if status, payload := put(replacement, f.owner, "avatar-2", "image/gif"); status != http.StatusOK {
		t.Fatalf("avatar replacement = %d %v, want 200", status, payload)
	} else if url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string); url != v1PublicAvatarURLForRow("AVTR01", replacementKey) {
		t.Fatalf("replacement avatar_url = %q, want the replacement version", url)
	}
	if profile.avatar != replacementKey {
		t.Fatalf("replacement projection = %q, want key %q", profile.avatar, replacementKey)
	}
	if profile.writes != 2 {
		t.Fatalf("replacement avatar writes = %d, want 2", profile.writes)
	}
	f.objects.mu.Lock()
	_, superseded := f.objects.objects[wantKey]
	f.objects.mu.Unlock()
	if superseded {
		t.Fatalf("superseded avatar key %q was not cleaned up", wantKey)
	}
}

// assertV1PublicAvatarImage verifies a public size variant and its internal source.
func assertV1PublicAvatarImage(t *testing.T, engine *gin.Engine, cfg imgproxy.Config, path string, userID int64, size int, key string) {
	t.Helper()
	call := v1Call{method: "GET", path: path}
	if userID > 0 {
		call.userID = userID
	}
	assertImgproxyImage(t, engine, cfg, call, "test-avatar-",
		path, "no-store",
		fmt.Sprintf("/rs:fill:%d:%d/plain/local:///%s@webp", size, size, key))
}

// TestV1PublicAvatarRoute covers the public avatar GET: sizeless and
// size variants (anonymous included), the size whitelist, unknown/malformed
// uids, and the bundled default SVG for callers without an avatar.
func TestV1PublicAvatarRoute(t *testing.T) {
	profile := &fakeAvatarUID{uid: "AVTR01"}
	f := newV1MediaGapsFixture(t, profile)
	cfg := v1TestImgproxyConfig(t)
	route := "/api/v1/public/users/AVTR01/avatar"

	// No custom avatar yet: the bundled default SVG serves inline.
	status, header, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: route}, "test-avatar-")
	if status != http.StatusOK {
		t.Fatalf("default avatar = %d, want 200", status)
	}
	if ct := header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("default avatar Content-Type = %q, want image/svg+xml", ct)
	}
	if cc := header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("default avatar Cache-Control = %q, want no-store", cc)
	}
	if !strings.Contains(body, "<svg") {
		t.Fatalf("default avatar body is not SVG: %q", body)
	}

	avatar := v1GapsPNGBytes(2048)
	wantKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(avatar)[:16] + ".png"
	status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: "/api/v1/user/avatar",
		body: string(avatar), userID: f.owner,
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-route-1"}})
	if status != http.StatusOK {
		t.Fatalf("avatar PUT = %d %v, want 200", status, payload)
	}

	// Sizeless defaults to 64; explicit sizes render their fill
	// geometry. Anonymous callers pass (avatars are public).
	assertV1PublicAvatarImage(t, f.engine, cfg, route, f.owner, 64, wantKey)
	assertV1PublicAvatarImage(t, f.engine, cfg, route, 0, 64, wantKey)
	for _, size := range []int{32, 128, 256} {
		assertV1PublicAvatarImage(t, f.engine, cfg, route+"?size="+strconv.Itoa(size), f.owner, size, wantKey)
	}

	// Outside the whitelist (or unparsable) reads as plain 400.
	for _, size := range []string{"100", "0", "-1", "abc", "64.5"} {
		status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET",
			path: route + "?size=" + size, userID: f.owner}, "test-avatar-")
		if status != http.StatusBadRequest {
			t.Fatalf("avatar size %q = %d, want plain 400", size, status)
		}
	}

	// Well-shaped but unknown uids converge to 404; malformed uids to
	// 400. Both stay envelope-free like the binary endpoints.
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: "/api/v1/public/users/ZZZZ99/avatar", userID: f.owner}, "test-avatar-")
	if status != http.StatusNotFound {
		t.Fatalf("unknown avatar uid = %d, want plain 404", status)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: "/api/v1/public/users/bad%20uid!/avatar", userID: f.owner}, "test-avatar-")
	if status != http.StatusBadRequest {
		t.Fatalf("malformed avatar uid = %d, want plain 400", status)
	}
}

// TestV1AvatarFailClosed proves the avatar endpoint fails closed
// without its seams (501 without the UID directory, 503 without the
// byte backend).
func TestV1AvatarFailClosed(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	avatar := v1GapsPNGBytes(512)
	call := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar), userID: f.owner,
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-closed-1"}}
	if status, _, payload := doV1(t, f.engine, call); status != http.StatusNotImplemented ||
		v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("avatar without directory = %d %v, want 501", status, payload)
	}

	// UID directory present but no byte backend: 503.
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	dir.ensure(f.owner)
	ws, err := store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "avatar-closed", OwnerUserID: f.owner})
	if err != nil {
		t.Fatal(err)
	}
	_ = ws
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{Media: media, AvatarUID: &fakeAvatarUID{uid: "AVTR01"}})
	call.headers = map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-closed-2"}
	if status, _, payload := doV1(t, engine, call); status != http.StatusServiceUnavailable ||
		v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("avatar without objects = %d %v, want 503 service_unavailable", status, payload)
	}
}

// TestV1BatchGetMediaAssets covers the bounded batch metadata read:
// order preservation, 404 convergence per invisible id, bounds, and
// auth.
func TestV1BatchGetMediaAssets(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	first := v1GapsPNGBytes(256)
	second := v1GapsPNGBytes(512)

	firstUpload := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-batch-c1", "a.png", first)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+firstUpload+"/bytes", first, f.owner, "gaps-batch-s1")
	firstAsset := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+firstUpload+"/complete", first, f.owner, "gaps-batch-f1")
	secondUpload := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-batch-c2", "b.png", second)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+secondUpload+"/bytes", second, f.owner, "gaps-batch-s2")
	secondAsset := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+secondUpload+"/complete", second, f.owner, "gaps-batch-f2")

	batch := func(ids []string, userID int64) (int, map[string]any) {
		status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
			body: fmt.Sprintf(`{"data":{"asset_ids":[%s]}}`, v1GapsJSONStringArray(ids)), userID: userID})
		return status, payload
	}

	// Reversed request order comes back reversed (order preserved).
	status, payload := batch([]string{secondAsset, firstAsset}, f.member)
	if status != http.StatusOK {
		t.Fatalf("batch = %d %v, want 200", status, payload)
	}
	items, ok := v1Data(t, payload).(map[string]any)["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("batch items = %v, want 2", v1Data(t, payload))
	}
	if items[0].(map[string]any)["id"] != secondAsset || items[1].(map[string]any)["id"] != firstAsset {
		t.Fatalf("batch order = %v, want request order", items)
	}

	// Invisible ids converge to 404 without leaking which half failed.
	for name, ids := range map[string][]string{
		"unknown":   {"asset_missing00000001"},
		"malformed": {"nope"},
	} {
		if status, payload := batch(ids, f.owner); status != http.StatusNotFound ||
			v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
			t.Fatalf("batch %s = %d %v, want 404 media_not_found", name, status, payload)
		}
	}
	// Cross-workspace id: stage an asset in ws2, then batch it from ws1.
	foreignBody := v1GapsPNGBytes(128)
	foreignUpload := v1GapsDeclare(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads", f.other, "gaps-batch-c3", "c.png", foreignBody)
	v1GapsStage(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads/"+foreignUpload+"/bytes", foreignBody, f.other, "gaps-batch-s3")
	foreignAsset := v1GapsComplete(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads/"+foreignUpload+"/complete", foreignBody, f.other, "gaps-batch-f3")
	if status, payload := batch([]string{foreignAsset}, f.owner); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("batch cross-workspace = %d %v, want 404 media_not_found", status, payload)
	}

	// Bounds: empty and over-limit render 422; outsiders 404.
	if status, payload := batch(nil, f.owner); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("batch empty = %d %v, want 422 media_validation_failed", status, payload)
	}
	big := make([]string, maxV1MediaBatchAssets+1)
	for i := range big {
		big[i] = firstAsset
	}
	if status, payload := batch(big, f.owner); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("batch over-limit = %d %v, want 422 media_validation_failed", status, payload)
	}
	if status, payload := batch([]string{firstAsset}, f.other); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("batch outsider = %d %v, want 404 workspace_not_found", status, payload)
	}
}
