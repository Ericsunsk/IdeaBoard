package taskmedia

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
)

type Validator interface {
	ValidateSubmitMediaRef(ctx context.Context, userID int64, raw string) error
}

type TaskOutputCleaner interface {
	DeleteTaskOutputsByUserTask(ctx context.Context, userID int64, taskID string) error
}

type UploadReader interface {
	ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*StoredObject, error)
}

type ResultStore interface {
	UsesObjectStorage(ctx context.Context) bool
	StoreRemoteResult(ctx context.Context, userID int64, taskID string, remoteURL string, opts StoreOptions) (*StoredRef, error)
	StoreBytesResult(ctx context.Context, userID int64, taskID string, filename string, data []byte, opts StoreOptions) (*StoredRef, error)
	DeleteObjectByKey(ctx context.Context, objectKey string) error
}

type RuntimeMediaStore interface {
	UploadReader
	ResultStore
}

type StoreOptions struct {
	AllowHosts []string
}

type StoredObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
	MIMEType    string
	Ext         string
}

type StoredRef struct {
	URL            string
	StorageKey     string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	StorageProfile string
}

type SubmitMediaValidator struct {
	publicURLs settings.PublicURLSettingProvider
	authorizer UploadAuthorizer
}

type UploadAuthorizer interface {
	AuthorizeWorkspaceUpload(ctx context.Context, userID int64, rawPath string) error
}

const workspaceUploadPrefix = "/uploads/workspace/"

func NewSubmitMediaValidator(publicURLs settings.PublicURLSettingProvider, authorizer UploadAuthorizer) *SubmitMediaValidator {
	return &SubmitMediaValidator{
		publicURLs: publicURLs,
		authorizer: authorizer,
	}
}

func (v *SubmitMediaValidator) ValidateSubmitMediaRef(ctx context.Context, userID int64, raw string) error {
	normalized := v.CanonicalizePublicUploadRef(ctx, raw)
	if err := ValidateSubmitMediaRef(normalized); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(normalized)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return nil
	}
	if !strings.HasPrefix(trimmed, workspaceUploadPrefix) {
		return fmt.Errorf("仅支持工作区上传资源")
	}
	if v == nil || v.authorizer == nil {
		return fmt.Errorf("无法校验工作区上传资源权限")
	}
	if err := v.authorizer.AuthorizeWorkspaceUpload(ctx, userID, trimmed); err != nil {
		return fmt.Errorf("无权使用该工作区上传资源")
	}
	return nil
}

func (v *SubmitMediaValidator) CanonicalizePublicUploadRef(ctx context.Context, raw string) string {
	if v == nil {
		return CanonicalizeConfiguredPublicUploadRef(raw)
	}
	return CanonicalizeConfiguredPublicUploadRefWithProvider(ctx, v.publicURLs, raw)
}

func ValidateSubmitMediaRef(raw string) error {
	return mediarefs.ValidateMediaRef(CanonicalizeConfiguredPublicUploadRef(raw), outbound.AllowHostsFromEnv(), MaxSubmitTaskMediaDataURLBytes)
}

const MaxSubmitTaskMediaDataURLBytes = 10 * 1024 * 1024

func CanonicalizeConfiguredPublicUploadRef(raw string) string {
	return CanonicalizeConfiguredPublicUploadRefWithProvider(context.Background(), nil, raw)
}

func CanonicalizeConfiguredPublicUploadRefWithProvider(ctx context.Context, provider settings.PublicURLSettingProvider, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "/uploads/") || !strings.Contains(trimmed, "://") {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	cleanedPath := path.Clean(parsed.Path)
	if !strings.HasPrefix(cleanedPath, "/uploads/") {
		return trimmed
	}
	for _, base := range settings.PublicUploadBaseURLs(ctx, provider) {
		if sameURLOrigin(parsed, base) {
			return cleanedPath
		}
	}
	return trimmed
}

func sameURLOrigin(a *url.URL, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
