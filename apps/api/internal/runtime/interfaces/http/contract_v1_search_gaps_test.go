package httpapi

// Vision file/external-ref gaps for #809-close: upload_ids
// (stage-then-reference, never raw bytes) and image_urls (tree CDN
// allowlist, fail-closed). Billing behavior is untouched: these tests
// assert the same quote/hold/capture flow around the new inputs.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func newV1VisionGapsFixture(t *testing.T) (*v1MediaGapsFixture, *library.InMemoryWorkspaceLibraryStore, *fakeVisionExtractor, *fakeLibraryBilling) {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	lib := library.NewInMemoryWorkspaceLibraryStoreForTests()
	extractor := &fakeVisionExtractor{query: "warm cafe"}
	billing := &fakeLibraryBilling{}
	quoter := &fakeLibraryQuoter{amount: 3}
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "vision-gaps", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	ws2, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "vision-gaps-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws2.PublicID, owner, tenancy.RoleMember)
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{
		Media: media, MediaObjects: objects,
		Search: ContractV1SearchDeps{
			Backend: &fakeSearchBackend{}, Library: lib,
			Quoter: quoter, Settler: billing, Extractor: extractor,
		},
	})
	f := &v1MediaGapsFixture{store: store, media: media, objects: objects, dir: dir, engine: engine,
		ws: ws, ws2: ws2, owner: owner, member: member, other: other}
	return f, lib, extractor, billing
}

func v1VisionGapsPost(t *testing.T, f *v1MediaGapsFixture, key, body string, userID int64) (int, map[string]any) {
	t.Helper()
	call := v1Call{method: http.MethodPost,
		path: "/api/v1/workspaces/" + f.ws.PublicID + "/search/vision-queries",
		body: body, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: key}}
	status, _, payload := doV1(t, f.engine, call)
	return status, payload
}

// TestV1VisionQueryStagedUploadRef proves stage-then-reference: bytes
// ride the upload-session PUT, the vision call carries only the
// upload_ id, and the extractor receives a server-resolved locator
// (never client bytes).
func TestV1VisionQueryStagedUploadRef(t *testing.T) {
	f, _, extractor, billing := newV1VisionGapsFixture(t)
	staged := v1GapsPNGBytes(768)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.member, "vision-up-create", "shot.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.member, "vision-up-stage")

	body := fmt.Sprintf(`{"data":{"user_text":"sunset","upload_ids":[%q]}}`, uploadID)
	status, created := v1VisionGapsPost(t, f, "vision-up-1", body, f.member)
	if status != http.StatusCreated {
		t.Fatalf("vision query = %d %v, want 201", status, created)
	}
	if billing.captures != 1 || billing.releases != 0 {
		t.Fatalf("billing captures=%d releases=%d, want 1/0", billing.captures, billing.releases)
	}
	refs := extractor.lastInput.References
	if len(refs) != 1 || refs[0].UploadID != uploadID || refs[0].ObjectKey == "" {
		t.Fatalf("extractor refs = %+v, want one resolved upload locator", refs)
	}
	if refs[0].Kind != "upload" {
		t.Fatalf("extractor ref kind = %q, want upload", refs[0].Kind)
	}

	// A completed session stays usable as a vision ref.
	assetDone := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.member, "vision-up-complete")
	if assetDone == "" {
		t.Fatal("complete returned no asset")
	}
	status, _ = v1VisionGapsPost(t, f, "vision-up-2", body, f.member)
	if status != http.StatusCreated {
		t.Fatalf("vision query on completed upload = %d, want 201", status)
	}
}

// TestV1VisionQueryUnstagedUploadRejected proves the vision call
// never triggers a transfer itself: unstaged sessions 422 before any
// billing hold, and unknown/cross-workspace ids 404.
func TestV1VisionQueryUnstagedUploadRejected(t *testing.T) {
	f, _, _, billing := newV1VisionGapsFixture(t)
	staged := v1GapsPNGBytes(256)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.member, "vision-nostage-create", "shot.png", staged)
	body := fmt.Sprintf(`{"data":{"upload_ids":[%q]}}`, uploadID)
	status, payload := v1VisionGapsPost(t, f, "vision-nostage-1", body, f.member)
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("unstaged vision = %d %v, want 422 validation_failed", status, payload)
	}
	if fields := v1GapsErrorFields(t, payload); fields["upload_ids"] == "" {
		t.Fatalf("unstaged vision fields = %v, want upload_ids", fields)
	}
	if billing.captures != 0 || billing.releases != 0 {
		t.Fatalf("billing captures=%d releases=%d, want 0/0 (rejected before hold)", billing.captures, billing.releases)
	}

	// Malformed upload ids fail validation; well-shaped unknown ids 404.
	status, payload = v1VisionGapsPost(t, f, "vision-badshape-1", `{"data":{"upload_ids":["nope"]}}`, f.member)
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("malformed upload ref = %d %v, want 422 validation_failed", status, payload)
	}
	status, payload = v1VisionGapsPost(t, f, "vision-unknown-1", `{"data":{"upload_ids":["upload_missing00000001"]}}`, f.member)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("unknown upload ref = %d %v, want 404 upload_not_found", status, payload)
	}

	// Cross-workspace upload ids are invisible.
	foreignBody := v1GapsPNGBytes(128)
	foreignUpload := v1GapsDeclare(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads", f.other, "vision-foreign-create", "f.png", foreignBody)
	v1GapsStage(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads/"+foreignUpload+"/bytes", foreignBody, f.other, "vision-foreign-stage")
	foreignBodyStr := fmt.Sprintf(`{"data":{"upload_ids":[%q]}}`, foreignUpload)
	status, payload = v1VisionGapsPost(t, f, "vision-foreign-1", foreignBodyStr, f.member)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("cross-workspace upload ref = %d %v, want 404 upload_not_found", status, payload)
	}
}

// TestV1VisionQueryExternalURLs proves external refs are validated
// against the tree CDN allowlist (https exact-host) and fail closed
// otherwise.
func TestV1VisionQueryExternalURLs(t *testing.T) {
	f, _, extractor, _ := newV1VisionGapsFixture(t)

	good := `{"data":{"user_text":"dunes","image_urls":["https://images.unsplash.com/photo-1"]}}`
	status, created := v1VisionGapsPost(t, f, "vision-ext-1", good, f.member)
	if status != http.StatusCreated {
		t.Fatalf("allowlisted vision = %d %v, want 201", status, created)
	}
	refs := extractor.lastInput.References
	if len(refs) != 1 || refs[0].ImageURL != "https://images.unsplash.com/photo-1" || refs[0].Kind != "external" {
		t.Fatalf("extractor refs = %+v, want one external ref", refs)
	}

	for name, url := range map[string]string{
		"evil host":    "https://evil.example/photo-1",
		"lookalike":    "https://images.unsplash.com.attacker.example/photo-1",
		"plain http":   "http://images.unsplash.com/photo-1",
		"non-url":      "not a url",
		"data URL":     "data:image/png;base64,abcd",
		"unlisted cdn": "https://cdn.example.com/photo-1",
	} {
		body := fmt.Sprintf(`{"data":{"image_urls":[%q]}}`, url)
		status, payload := v1VisionGapsPost(t, f, "vision-ext-bad-"+strings.ReplaceAll(name, " ", "-"), body, f.member)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("external %s = %d %v, want 422 validation_failed", name, status, payload)
		}
	}
}

// TestV1VisionQueryRefCapAndHash proves the 4-ref cap spans all three
// ref kinds combined and the idempotency hash covers the new fields.
func TestV1VisionQueryRefCapAndHash(t *testing.T) {
	f, _, _, _ := newV1VisionGapsFixture(t)

	over := `{"data":{"image_urls":["https://images.unsplash.com/1","https://images.unsplash.com/2","https://images.unsplash.com/3","https://images.pexels.com/4","https://images.pexels.com/5"]}}`
	status, payload := v1VisionGapsPost(t, f, "vision-cap-1", over, f.member)
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("over-cap vision = %d %v, want 422 validation_failed", status, payload)
	}

	// Same key, different upload_ids: conflict, not replay.
	staged := v1GapsPNGBytes(192)
	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.member, "vision-hash-create", "h.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.member, "vision-hash-stage")
	withRef := fmt.Sprintf(`{"data":{"user_text":"cafe","upload_ids":[%q]}}`, uploadID)
	status, _ = v1VisionGapsPost(t, f, "vision-hash-1", `{"data":{"user_text":"cafe"}}`, f.member)
	if status != http.StatusCreated {
		t.Fatalf("text-only vision = %d, want 201", status)
	}
	status, payload = v1VisionGapsPost(t, f, "vision-hash-1", withRef, f.member)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeSearchIdempotencyConflict) {
		t.Fatalf("hash-divergent vision = %d %v, want 409 search_idempotency_conflict", status, payload)
	}
}
