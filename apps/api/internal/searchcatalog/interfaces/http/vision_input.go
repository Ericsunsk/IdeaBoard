package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/outboundhttp"
	"ideaboard/api/internal/searchcatalog/application"
	"ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

type VisionInputReader interface {
	Read(*http.Request, int64) (domain.VisionInput, error)
}

type multipartVisionInputReader struct {
	materializer visionReferenceMaterializer
}

func newMultipartVisionInputReader(libraryReader VisionLibraryAssetReader) VisionInputReader {
	return multipartVisionInputReader{materializer: visionReferenceMaterializer{libraryReader: libraryReader}}
}

func NewVisionInputReader(libraryReader VisionLibraryAssetReader) VisionInputReader {
	return newMultipartVisionInputReader(libraryReader)
}

func (r multipartVisionInputReader) Read(request *http.Request, ownerUserID int64) (domain.VisionInput, error) {
	if request == nil {
		return domain.VisionInput{}, domain.ErrVisionInput
	}
	if err := request.ParseMultipartForm(domain.VisionMaxBytes); err != nil {
		return domain.VisionInput{}, domain.ErrVisionInput
	}
	defer request.MultipartForm.RemoveAll()
	files := request.MultipartForm.File["images[]"]
	if len(files) == 0 {
		files = request.MultipartForm.File["images"]
	}
	rawAssetIDs := request.MultipartForm.Value["library_asset_ids[]"]
	if len(rawAssetIDs) == 0 {
		rawAssetIDs = request.MultipartForm.Value["library_asset_ids"]
	}
	rawExternalRefs := request.MultipartForm.Value["external_references[]"]
	if len(rawExternalRefs) == 0 {
		rawExternalRefs = request.MultipartForm.Value["external_references"]
	}
	assetIDs, err := parseLibraryAssetIDs(rawAssetIDs)
	if err != nil {
		return domain.VisionInput{}, domain.ErrVisionInput
	}
	externalRefs, err := parseExternalReferences(rawExternalRefs)
	if err != nil {
		return domain.VisionInput{}, domain.ErrVisionInput
	}
	totalRefs := len(files) + len(assetIDs) + len(externalRefs)
	if totalRefs < 1 || totalRefs > domain.VisionMaxImages {
		return domain.VisionInput{}, domain.ErrVisionInput
	}
	images := make([]domain.VisionImage, 0, totalRefs)
	for _, header := range files {
		image, err := readMultipartVisionImage(header)
		if err != nil {
			return domain.VisionInput{}, domain.ErrVisionInput
		}
		images = append(images, image)
	}
	materialized, err := r.materializer.Materialize(request.Context(), ownerUserID, assetIDs, externalRefs)
	if err != nil {
		return domain.VisionInput{}, err
	}
	images = append(images, materialized...)
	if err := application.ValidateVisionImages(images); err != nil {
		return domain.VisionInput{}, err
	}
	return domain.VisionInput{Images: images, UserText: request.FormValue("user_text")}, nil
}

func readMultipartVisionImage(header *multipart.FileHeader) (domain.VisionImage, error) {
	if header == nil {
		return domain.VisionImage{}, domain.ErrVisionInput
	}
	file, err := header.Open()
	if err != nil {
		return domain.VisionImage{}, domain.ErrVisionInput
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, domain.VisionMaxBytes+1))
	if err != nil || len(data) > domain.VisionMaxBytes {
		return domain.VisionImage{}, domain.ErrVisionInput
	}
	return domain.VisionImage{Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data}, nil
}

type VisionLibraryAssetReader interface {
	GetAsset(context.Context, int64, int64) (library.Asset, error)
}

// visionReferenceMaterializer owns every server-side reference ingress. The
// multipart reader only parses the transport; this module enforces ownership,
// CDN policy, byte limits, and content metadata for Library and Search refs.
type visionReferenceMaterializer struct {
	libraryReader VisionLibraryAssetReader
}

func (m visionReferenceMaterializer) Materialize(
	ctx context.Context,
	ownerUserID int64,
	assetIDs []int64,
	externalRefs []VisionExternalReference,
) ([]domain.VisionImage, error) {
	if len(assetIDs) > 0 && m.libraryReader == nil {
		return nil, domain.ErrVisionInput
	}
	images := make([]domain.VisionImage, 0, len(assetIDs)+len(externalRefs))
	for _, assetID := range assetIDs {
		asset, err := m.libraryReader.GetAsset(ctx, ownerUserID, assetID)
		if err != nil {
			return nil, domain.ErrVisionImageUnavailable
		}
		image, err := readLibraryVisionImage(ctx, asset)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	for _, ref := range externalRefs {
		image, err := readExternalVisionImage(ctx, ref)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func readLibraryVisionImage(ctx context.Context, asset library.Asset) (domain.VisionImage, error) {
	rawURL := asset.OriginalURL
	if strings.TrimSpace(rawURL) == "" {
		rawURL = asset.PreviewURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || !isLibraryImageHost(parsed.Hostname()) {
		return domain.VisionImage{}, domain.ErrVisionImageUnavailable
	}
	response, err := getVisionImage(ctx, parsed)
	if err != nil {
		return domain.VisionImage{}, fmt.Errorf("%w: %w", domain.ErrVisionImageUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.VisionImage{}, domain.ErrVisionImageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, domain.VisionMaxBytes+1))
	if err != nil {
		return domain.VisionImage{}, fmt.Errorf("%w: %w", domain.ErrVisionImageUnavailable, err)
	}
	if len(data) > domain.VisionMaxBytes {
		return domain.VisionImage{}, domain.ErrVisionImageInvalid
	}
	contentType := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	return domain.VisionImage{Filename: "library-reference", ContentType: contentType, Data: data}, nil
}

func getVisionImage(ctx context.Context, imageURL *url.URL) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL.String(), nil)
	if err != nil {
		return nil, err
	}
	return visionImageRequest(ctx, request)
}

// visionImageOutbound is the shared standard outbound client for reference
// image fetches: timeout cap, bounded retries and Retry-After respect match
// the gateway migration (#766). Exhausted retryable statuses surface as
// errors and map to ErrVisionImageUnavailable at the callers, exactly like
// the single-shot non-2xx responses they replace.
var visionImageOutbound = outboundhttp.NewClient(outboundhttp.Config{})

// visionInputTarget names reference image egress in outbound logs/metrics.
const visionInputTarget = "vision-input"

// visionImageRequest is kept as a narrow seam for deterministic tests. The
// production implementation still runs every request through the standard
// outbound client and combines the runtime outbound policy with the exact,
// curated image CDN hosts used by Search references.
// visionInputFetchTimeout bounds one external reference image fetch,
// separate from the gateway extraction timeout.
const visionInputFetchTimeout = 30 * time.Second

var visionImageRequest = func(ctx context.Context, request *http.Request) (*http.Response, error) {
	return visionImageOutbound.Do(ctx, request, outboundhttp.RequestOptions{
		Timeout:    visionInputFetchTimeout,
		AllowHosts: visionImageAllowHosts(ctx),
		Target:     visionInputTarget,
	})
}

func visionImageAllowHosts(ctx context.Context) []string {
	configured := outbound.AllowHosts(ctx)
	allowed := make([]string, 0, len(configured)+3)
	seen := make(map[string]struct{}, len(configured)+3)
	for _, host := range configured {
		normalized := strings.ToLower(strings.TrimSpace(host))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		allowed = append(allowed, host)
	}
	for _, host := range []string{"images.unsplash.com", "plus.unsplash.com", "images.pexels.com"} {
		normalized := strings.ToLower(strings.TrimSpace(host))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		allowed = append(allowed, host)
	}
	return allowed
}

func parseLibraryAssetIDs(rawIDs []string) ([]int64, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(rawIDs))
	seen := make(map[int64]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id <= 0 {
			return nil, domain.ErrVisionInput
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func isLibraryImageHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "images.unsplash.com", "plus.unsplash.com", "images.pexels.com":
		return true
	default:
		return false
	}
}

// VisionExternalReference is the metadata the browser sends for a Search
// reference whose image is owned by a third-party provider (Unsplash, Pexels).
// The browser must never fetch the third-party image; the API retrieves,
// validates, and forwards the bytes to the Vision provider. Allowing direct
// fetches would re-introduce the cross-origin / CORS coupling that the
// same-origin gateway contract removes.
type VisionExternalReference struct {
	Provider   string `json:"provider"`
	ID         string `json:"id"`
	PreviewURL string `json:"preview_url"`
}

func parseExternalReferences(raw []string) ([]VisionExternalReference, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	refs := make([]VisionExternalReference, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, rawRef := range raw {
		trimmed := strings.TrimSpace(rawRef)
		if trimmed == "" {
			return nil, domain.ErrVisionInput
		}
		ref, err := decodeExternalReference(trimmed)
		if err != nil {
			return nil, domain.ErrVisionInput
		}
		ref.Provider = strings.ToLower(strings.TrimSpace(ref.Provider))
		ref.ID = strings.TrimSpace(ref.ID)
		ref.PreviewURL = strings.TrimSpace(ref.PreviewURL)
		if ref.Provider == "" || ref.ID == "" || ref.PreviewURL == "" {
			return nil, domain.ErrVisionInput
		}
		if !isAllowedExternalProvider(ref.Provider) {
			return nil, domain.ErrVisionInput
		}
		key := ref.Provider + ":" + ref.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	return refs, nil
}

func decodeExternalReference(raw string) (VisionExternalReference, error) {
	var ref VisionExternalReference
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		return VisionExternalReference{}, err
	}
	return ref, nil
}

func isAllowedExternalProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "unsplash", "pexels":
		return true
	default:
		return false
	}
}

// readExternalVisionImage fetches, validates, and decodes the bytes the
// browser would otherwise have to download directly. The HTTP transport is
// the same one the library-asset path uses so the allowlist, timeout, and
// size cap stay consistent.
func readExternalVisionImage(ctx context.Context, ref VisionExternalReference) (domain.VisionImage, error) {
	parsed, err := url.Parse(strings.TrimSpace(ref.PreviewURL))
	if err != nil || parsed.Scheme != "https" || !isLibraryImageHost(parsed.Hostname()) {
		return domain.VisionImage{}, domain.ErrVisionImageUnavailable
	}
	// Reject anything that is not on the exact CDN host allowlist. The allowlist
	// is shared with the library-asset path so the server-side materialization
	// never depends on third-party CORS headers.
	if !isExternalReferenceHost(parsed.Hostname(), ref.Provider) {
		return domain.VisionImage{}, domain.ErrVisionImageUnavailable
	}
	response, err := getVisionImage(ctx, parsed)
	if err != nil {
		return domain.VisionImage{}, fmt.Errorf("%w: %w", domain.ErrVisionImageUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.VisionImage{}, domain.ErrVisionImageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, domain.VisionMaxBytes+1))
	if err != nil {
		return domain.VisionImage{}, fmt.Errorf("%w: %w", domain.ErrVisionImageUnavailable, err)
	}
	if len(data) == 0 || len(data) > domain.VisionMaxBytes {
		return domain.VisionImage{}, domain.ErrVisionImageInvalid
	}
	contentType := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	return domain.VisionImage{Filename: ref.Provider + "-" + ref.ID, ContentType: contentType, Data: data}, nil
}

func isExternalReferenceHost(host, provider string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "unsplash":
		return host == "images.unsplash.com" || host == "plus.unsplash.com"
	case "pexels":
		return host == "images.pexels.com"
	}
	return false
}

// writeVisionError maps domain vision failures to stable machine codes.
// Relocated from the deleted legacy vision HTTP handler file (its only
// production caller is gone; the mapping stays pinned by
// TestWriteVisionErrorUsesStableMachineCodes as the stable vision
// error-code contract).
func writeVisionError(c *gin.Context, err error) {
	status, code, message := http.StatusBadGateway, string(contract.CodeVisionSearchProviderError), "视觉搜索暂时不可用"
	switch {
	case errors.Is(err, domain.ErrVisionInput):
		status, code, message = http.StatusBadRequest, string(contract.CodeVisionSearchInputInvalid), "请添加 1–4 张参考图"
	case errors.Is(err, domain.ErrVisionImageInvalid):
		status, code, message = http.StatusBadRequest, string(contract.CodeVisionSearchImageInvalid), "参考图格式、尺寸或总大小无效"
	case errors.Is(err, domain.ErrVisionImageUnavailable):
		status, code, message = http.StatusBadRequest, string(contract.CodeVisionSearchImageUnavailable), "参考图暂时无法读取"
	case errors.Is(err, domain.ErrVisionNotConfigured):
		status, code, message = http.StatusServiceUnavailable, string(contract.CodeVisionSearchNotConfigured), "视觉搜索暂未配置"
	case errors.Is(err, domain.ErrVisionUnavailable):
		status, code, message = http.StatusServiceUnavailable, string(contract.CodeVisionSearchUnavailable), "视觉搜索暂不可用"
	case errors.Is(err, domain.ErrVisionSchema):
		status, code, message = http.StatusBadGateway, string(contract.CodeVisionSearchSchemaError), "视觉搜索返回格式无效"
	default:
		var providerErr *domain.VisionProviderError
		if errors.As(err, &providerErr) {
			switch providerErr.Kind {
			case domain.VisionProviderFailureCredentials:
				status, code, message = http.StatusBadGateway, string(contract.CodeVisionSearchCredentials), "视觉搜索凭证无效"
			case domain.VisionProviderFailureQuota:
				status, code, message = http.StatusBadGateway, string(contract.CodeVisionSearchQuota), "视觉搜索权限或配额不可用"
			case domain.VisionProviderFailureTimeout:
				status, code, message = http.StatusGatewayTimeout, string(contract.CodeVisionSearchTimeout), "视觉搜索超时，请重试"
			case domain.VisionProviderFailureNetwork:
				status, code, message = http.StatusBadGateway, string(contract.CodeVisionSearchNetwork), "视觉搜索连接失败，请重试"
			}
		}
	}
	errorrender.Render(c, status, code, message)
}
