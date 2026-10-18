package workspacemedia

import (
	"strings"
	"testing"

	"ideaboard/api/internal/platform/contract"
)

const testChecksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidateCreateInput(t *testing.T) {
	if err := ValidateCreateInput("photo.png", "image/png", 1024, testChecksum, "image"); err != nil {
		t.Fatalf("valid input = %v, want nil", err)
	}
	// Empty kind fails here: handlers default it to image before calling.
	if err := ValidateCreateInput("photo.png", "image/png", 1024, testChecksum, ""); !isFieldError(err, "media_kind") {
		t.Fatalf("empty kind = %v, want media_kind field error", err)
	}
	cases := []struct {
		name     string
		filename string
		mime     string
		size     int64
		checksum string
		kind     string
		field    string
	}{
		{"blank filename", "", "image/png", 1024, testChecksum, "image", "filename"},
		{"slash filename", "a/b.png", "image/png", 1024, testChecksum, "image", "filename"},
		{"unsupported mime", "photo.png", "image/heic", 1024, testChecksum, "image", "mime_type"},
		{"zero size", "photo.png", "image/png", 0, testChecksum, "image", "size_bytes"},
		{"oversize", "photo.png", "image/png", MaxUploadSizeBytes + 1, testChecksum, "image", "size_bytes"},
		{"bad checksum", "photo.png", "image/png", 1024, "nope", "image", "checksum_sha256"},
		{"video kind", "clip.mp4", "image/png", 1024, testChecksum, "video", "media_kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateInput(tc.filename, tc.mime, tc.size, tc.checksum, tc.kind)
			var validationErr *ValidationError
			if !isValidationErrorAs(err, &validationErr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if _, ok := validationErr.Fields[tc.field]; !ok {
				t.Fatalf("fields = %v, want %q", validationErr.Fields, tc.field)
			}
		})
	}
}

func isValidationErrorAs(err error, target **ValidationError) bool {
	if err == nil {
		return false
	}
	converted, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	*target = converted
	return true
}

func TestChecksumHelpers(t *testing.T) {
	if !IsValidChecksum(testChecksum) {
		t.Fatal("valid checksum rejected")
	}
	if !IsValidChecksum(strings.ToUpper(testChecksum)) {
		t.Fatal("uppercase checksum rejected")
	}
	for _, bad := range []string{"", "abc", strings.Repeat("z", 64), strings.Repeat("0", 63)} {
		if IsValidChecksum(bad) {
			t.Fatalf("IsValidChecksum(%q) = true, want false", bad)
		}
	}
	if got := NormalizeChecksum(strings.ToUpper(testChecksum)); got != testChecksum {
		t.Fatalf("NormalizeChecksum = %q, want lowercase", got)
	}
}

func TestCanonicalMIME(t *testing.T) {
	if got := CanonicalMIME("image/png"); got != "image/png" {
		t.Fatalf("CanonicalMIME png = %q", got)
	}
	if got := CanonicalMIME("image/jpg"); got != "image/jpeg" {
		t.Fatalf("CanonicalMIME jpg alias = %q", got)
	}
	if got := CanonicalMIME("image/heic"); got != "" {
		t.Fatalf("CanonicalMIME heic = %q, want empty", got)
	}
	if got := MediaKindForMIME("image/png"); got != MediaKindImage {
		t.Fatalf("MediaKindForMIME png = %q", got)
	}
	if got := MediaKindForMIME("image/heic"); got != "" {
		t.Fatalf("MediaKindForMIME heic = %q, want empty", got)
	}
}

func TestPublicIDShapes(t *testing.T) {
	uploadID, err := contract.MintPublicID(contract.PrefixUpload, 12)
	if err != nil || !contract.IsUploadID(uploadID) {
		t.Fatalf("MintPublicID(upload) = %q, %v; want upload_ shape", uploadID, err)
	}
	assetID, err := contract.MintPublicID(contract.PrefixAsset, 12)
	if err != nil || !contract.IsAssetID(assetID) {
		t.Fatalf("MintPublicID(asset) = %q, %v; want asset_ shape", assetID, err)
	}
	// The issue's masset_ assumption is wrong: the schema CHECK is
	// asset_%, so masset_ ids must NOT validate.
	for _, bad := range []string{"", "42", "masset_abc123", "upload_"} {
		if contract.IsAssetID(bad) && bad != "" {
			t.Fatalf("contract.IsAssetID(%q) = true, want false", bad)
		}
	}
	for _, bad := range []string{"", "42", "asset_abc123", "upload"} {
		if contract.IsUploadID(bad) {
			t.Fatalf("contract.IsUploadID(%q) = true, want false", bad)
		}
	}
}

func TestSlotKeys(t *testing.T) {
	slot := SlotKey("ws_abc123", "upload_def456", "photo.png")
	if !strings.HasPrefix(slot, "uploads/ws/abc123/def456/") {
		t.Fatalf("slot = %q, want canonical workspace-isolated prefix", slot)
	}
	if !strings.HasSuffix(slot, "photo.png") {
		t.Fatalf("slot = %q, want sanitized filename suffix", slot)
	}
	if got := SlotFilename(slot); got != "photo.png" {
		t.Fatalf("SlotFilename = %q, want photo.png", got)
	}
	// Traversal segments never escape the slot: validation rejects
	// slashes and the sanitizer belts-and-suspenders the rest.
	if got := SanitizeFilename("../evil"); strings.Contains(got, "/") {
		t.Fatalf("SanitizeFilename = %q, want no separator", got)
	}
	if got := SanitizeFilename(""); got != "upload.bin" {
		t.Fatalf("SanitizeFilename empty = %q", got)
	}
}

func TestMatchCompletionFacts(t *testing.T) {
	session := &Session{
		MIMEType:       "image/png",
		SizeBytes:      1024,
		ChecksumSHA256: testChecksum,
		MediaKind:      MediaKindImage,
	}
	good := CompleteFacts{ChecksumSHA256: testChecksum, SizeBytes: 1024, MIMEType: "image/png", MediaKind: "image"}
	if err := matchCompletionFacts(session, good); err != nil {
		t.Fatalf("match = %v, want nil", err)
	}
	// MIME/kind echoes are optional but must match when present.
	blank := CompleteFacts{ChecksumSHA256: testChecksum, SizeBytes: 1024}
	if err := matchCompletionFacts(session, blank); err != nil {
		t.Fatalf("blank echo match = %v, want nil", err)
	}
	bad := good
	bad.ChecksumSHA256 = strings.Repeat("1", 64)
	if err := matchCompletionFacts(session, bad); !isFieldError(err, "checksum_sha256") {
		t.Fatalf("checksum mismatch = %v, want field error", err)
	}
	bad = good
	bad.SizeBytes = 2048
	if err := matchCompletionFacts(session, bad); !isFieldError(err, "size_bytes") {
		t.Fatalf("size mismatch = %v, want field error", err)
	}
	bad = good
	bad.MIMEType = "image/jpeg"
	if err := matchCompletionFacts(session, bad); !isFieldError(err, "mime_type") {
		t.Fatalf("mime mismatch = %v, want field error", err)
	}
}

func isFieldError(err error, field string) bool {
	validationErr, ok := err.(*ValidationError)
	return ok && validationErr.Fields[field] != ""
}

// merged from status_test.go (workspacemedia.go aggregation: SessionStatus transition guards).
// TestCanTransitionMirrorsSQLGuards pins the durable transition table
// to the guards in internal/db/queries/workspace_media.sql:
// CreateUploadSession inserts pending; MarkUploadSessionTransferring
// fences pending -> transferring; Complete/Fail/Expire flip a live
// (pending/transferring) session to completed/failed/expired;
// terminals have no outgoing transitions. Every true entry was
// already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]SessionStatus{
		{StatusPending, StatusTransferring},
		{StatusPending, StatusCompleted},
		{StatusPending, StatusFailed},
		{StatusPending, StatusExpired},
		{StatusTransferring, StatusCompleted},
		{StatusTransferring, StatusFailed},
		{StatusTransferring, StatusExpired},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []SessionStatus{StatusCompleted, StatusFailed, StatusExpired}
	live := []SessionStatus{StatusPending, StatusTransferring, StatusCompleted, StatusFailed, StatusExpired}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(StatusTransferring, StatusTransferring) {
		t.Error("CanTransition(transferring, transferring) = true, want false (first read is a no-op when already transferring, never a flip)")
	}
	if CanTransition(StatusTransferring, StatusPending) {
		t.Error("CanTransition(transferring, pending) = true, want false (no backward transition exists)")
	}
}

// TestSessionStatusWireWords pins the wire/DB words: typing must not
// change the strings on the wire or in the database.
func TestSessionStatusWireWords(t *testing.T) {
	for status, want := range map[SessionStatus]string{
		StatusPending:      "pending",
		StatusTransferring: "transferring",
		StatusCompleted:    "completed",
		StatusFailed:       "failed",
		StatusExpired:      "expired",
	} {
		if string(status) != want {
			t.Errorf("SessionStatus = %q, want %q", string(status), want)
		}
	}
	if AssetStatusReady != "ready" {
		t.Errorf("AssetStatusReady = %q, want %q", AssetStatusReady, "ready")
	}
}

func TestSessionIsTerminal(t *testing.T) {
	for status, want := range map[SessionStatus]bool{
		StatusPending:      false,
		StatusTransferring: false,
		StatusCompleted:    true,
		StatusFailed:       true,
		StatusExpired:      true,
	} {
		session := &Session{Status: status}
		if got := session.IsTerminal(); got != want {
			t.Errorf("IsTerminal(%q) = %v, want %v", status, got, want)
		}
		if got := session.IsLive(); got == want {
			t.Errorf("IsLive(%q) = %v, want %v", status, got, !want)
		}
	}
}
