package workspacemedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	storageports "ideaboard/api/internal/media/storage/ports"
)

type ingestObjects struct {
	profile string
	key     string
	mime    string
	data    []byte
	put     func() error
}

func (o *ingestObjects) PutObjectAtProfile(_ context.Context, profile, key string, data []byte, mime string) error {
	if o.put != nil {
		if err := o.put(); err != nil {
			return err
		}
	}
	o.profile, o.key, o.mime = profile, key, mime
	o.data = append([]byte(nil), data...)
	return nil
}

func (o *ingestObjects) ReadObjectAtProfile(context.Context, string, string, int64) (*storageports.StoredUploadObject, error) {
	return nil, errors.New("unexpected read")
}

type ingestSessions struct {
	SessionStore
	session     *Session
	completeErr error
	failCtxErr  error
	hasDeadline bool
	failCalls   int
}

func (s *ingestSessions) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	session, err := s.SessionStore.CreateSession(ctx, params)
	s.session = session
	return session, err
}

func (s *ingestSessions) CompleteSession(ctx context.Context, workspaceID, uploadID string, facts CompleteFacts, params CreateAssetParams) (*Session, *Asset, error) {
	if s.completeErr != nil {
		return nil, nil, s.completeErr
	}
	return s.SessionStore.CompleteSession(ctx, workspaceID, uploadID, facts, params)
}

func (s *ingestSessions) FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error) {
	s.failCalls++
	s.failCtxErr = ctx.Err()
	_, s.hasDeadline = ctx.Deadline()
	return s.SessionStore.FailSession(ctx, workspaceID, uploadID, reason)
}

func ingestPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestIngestImageCompletesValidatedAsset(t *testing.T) {
	data := ingestPNG(t)
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	store := NewInMemoryWorkspaceMediaStoreForTests()
	sessions := &ingestSessions{SessionStore: store}
	objects := &ingestObjects{}
	asset, err := IngestImage(context.Background(), sessions, objects, IngestImageParams{
		WorkspaceID: " ws_image ", OwnerUserID: 42, FilenameStem: "result", Data: data,
		ExpectedChecksumSHA256: " " + strings.ToUpper(checksum) + " ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.WorkspaceID != "ws_image" || asset.OwnerUserID != 42 || asset.ChecksumSHA256 != checksum || asset.SizeBytes != int64(len(data)) || asset.MIMEType != "image/png" || asset.Width != 3 || asset.Height != 2 {
		t.Fatalf("asset facts = %+v", asset)
	}
	if objects.profile != DefaultStorageProfile || objects.key != asset.StorageKey || objects.mime != asset.MIMEType || !bytes.Equal(objects.data, data) {
		t.Fatal("stored bytes differ from validated asset")
	}
	completed, err := store.GetSession(context.Background(), "ws_image", sessions.session.ID)
	if err != nil || completed.Status != StatusCompleted || completed.AssetID != asset.ID || completed.Filename != "result.png" {
		t.Fatalf("session = %+v, err = %v", completed, err)
	}
	if sessions.failCalls != 0 {
		t.Fatal("successful ingestion must leave its session completed")
	}
}

func TestIngestImageRejectsBytesBeforeReservation(t *testing.T) {
	valid := ingestPNG(t)
	for _, tc := range []struct {
		name     string
		data     []byte
		checksum string
	}{
		{name: "empty"},
		{name: "oversize", data: make([]byte, MaxUploadSizeBytes+1)},
		{name: "invalid image", data: []byte("not an image")},
		{name: "checksum mismatch", data: valid, checksum: strings.Repeat("f", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &ingestSessions{SessionStore: NewInMemoryWorkspaceMediaStoreForTests()}
			objects := &ingestObjects{}
			asset, err := IngestImage(context.Background(), sessions, objects, IngestImageParams{
				WorkspaceID: "ws_image", OwnerUserID: 42, FilenameStem: "result",
				Data: tc.data, ExpectedChecksumSHA256: tc.checksum,
			})
			if err == nil || asset != nil || sessions.session != nil || objects.key != "" {
				t.Fatalf("invalid bytes reserved resources: asset=%v, err=%v", asset, err)
			}
			if tc.name == "invalid image" {
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) || validationErr.Fields["file"] == "" {
					t.Fatalf("invalid image error = %v, want typed file validation", err)
				}
			}
		})
	}
}

func TestIngestImageFailureLeavesNoAssetAndFailsSession(t *testing.T) {
	for _, stage := range []string{"put", "complete"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := NewInMemoryWorkspaceMediaStoreForTests()
			sessions := &ingestSessions{SessionStore: store}
			objects := &ingestObjects{}
			cause := errors.New("ingest storage failure")
			if stage == "put" {
				objects.put = func() error {
					cancel()
					return cause
				}
			} else {
				sessions.completeErr = cause
			}
			asset, err := IngestImage(ctx, sessions, objects, IngestImageParams{
				WorkspaceID: "ws_image", OwnerUserID: 42, FilenameStem: "result", Data: ingestPNG(t),
			})
			if !errors.Is(err, cause) || asset != nil {
				t.Fatalf("asset=%v err=%v, want original cause and no asset", asset, err)
			}
			if sessions.failCalls != 1 || sessions.failCtxErr != nil || !sessions.hasDeadline {
				t.Fatal("failure cleanup must use a bounded context independent of request cancellation")
			}
			failed, err := store.GetSession(context.Background(), "ws_image", sessions.session.ID)
			if err != nil || failed.Status != StatusFailed {
				t.Fatalf("session=%+v err=%v, want failed", failed, err)
			}
			assets, err := store.ListAssets(context.Background(), "ws_image", 10, 0)
			if err != nil || len(assets) != 0 {
				t.Fatalf("failed ingestion created assets: %v, err=%v", assets, err)
			}
		})
	}
}
