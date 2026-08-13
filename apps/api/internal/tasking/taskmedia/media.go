package taskmedia

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/tasks"
)

type Validator interface {
	ValidateSubmitMediaRef(ctx context.Context, userID int64, raw string) error
}

type UploadReader interface {
	ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*StoredObject, error)
}

type ResultMaterializer interface {
	MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts ResultMaterializeOptions) (MaterializedResults, error)
	MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts ResultMaterializeOptions) (MaterializedResults, error)
}

// GenerationMediaPreparer materializes external or temporary inputs before
// Task submission. It must not use the Task/Billing/River transaction.
type GenerationMediaPreparer interface {
	PrepareGenerationMedia(context.Context, int64, string, *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error)
}

type ResultMaterializeOptions struct {
	AllowHosts []string
}

type MaterializedResults struct {
	Items []MaterializedResult
}

func (r MaterializedResults) ImageURLs() []string {
	urls := make([]string, 0, len(r.Items))
	for _, item := range r.Items {
		urls = append(urls, item.ImageURL)
	}
	return urls
}

func (r MaterializedResults) Outputs() []tasks.TaskOutput {
	outputs := make([]tasks.TaskOutput, 0, len(r.Items))
	for _, item := range r.Items {
		outputs = append(outputs, item.Output)
	}
	return outputs
}

type MaterializedResult struct {
	ImageURL string
	Output   tasks.TaskOutput
}

type StoredObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
	MIMEType    string
	Ext         string
}

type SubmitMediaValidator struct {
	publicOrigins config.PublicOriginsConfig
	authorizer    UploadAuthorizer
}

type UploadAuthorizer interface {
	AuthorizeCanvasUpload(ctx context.Context, userID int64, rawPath string) error
}

const canvasUploadPrefix = "/uploads/canvas/"

func NewSubmitMediaValidator(publicOrigins config.PublicOriginsConfig, authorizer UploadAuthorizer) *SubmitMediaValidator {
	return &SubmitMediaValidator{
		publicOrigins: publicOrigins,
		authorizer:    authorizer,
	}
}

func (v *SubmitMediaValidator) ValidateSubmitMediaRef(ctx context.Context, userID int64, raw string) error {
	normalized := v.CanonicalizePublicUploadRef(ctx, raw)
	if err := ValidateSubmitMediaRef(ctx, normalized); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(normalized)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return nil
	}
	if !strings.HasPrefix(trimmed, canvasUploadPrefix) {
		return fmt.Errorf("仅支持画布上传资源")
	}
	if v == nil || v.authorizer == nil {
		return fmt.Errorf("无法校验画布上传资源权限")
	}
	if err := v.authorizer.AuthorizeCanvasUpload(ctx, userID, trimmed); err != nil {
		return fmt.Errorf("无权使用该画布上传资源")
	}
	return nil
}

func (v *SubmitMediaValidator) CanonicalizePublicUploadRef(ctx context.Context, raw string) string {
	if v == nil {
		return CanonicalizePublicUploadRef(raw)
	}
	return CanonicalizePublicUploadRefWithOrigins(v.publicOrigins, raw)
}

func ValidateSubmitMediaRef(ctx context.Context, raw string) error {
	return mediarefs.ValidateMediaRef(CanonicalizePublicUploadRef(raw), outbound.AllowHosts(ctx), MaxSubmitTaskMediaDataURLBytes)
}

const MaxSubmitTaskMediaDataURLBytes = 10 * 1024 * 1024

func CanonicalizePublicUploadRef(raw string) string {
	return CanonicalizePublicUploadRefWithOrigins(config.PublicOriginsConfig{}, raw)
}

func CanonicalizePublicUploadRefWithOrigins(publicOrigins config.PublicOriginsConfig, raw string) string {
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
	base := publicOrigins.APIParsedURL()
	if base != nil && sameURLOrigin(parsed, base) {
		return cleanedPath
	}
	return trimmed
}

func sameURLOrigin(a *url.URL, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
