package searchcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
)

type VisionInputReader interface {
	Read(*http.Request, int64) (VisionInput, error)
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

func (r multipartVisionInputReader) Read(request *http.Request, ownerUserID int64) (VisionInput, error) {
	if request == nil {
		return VisionInput{}, ErrVisionInput
	}
	if err := request.ParseMultipartForm(visionMaxBytes); err != nil {
		return VisionInput{}, ErrVisionInput
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
		return VisionInput{}, ErrVisionInput
	}
	externalRefs, err := parseExternalReferences(rawExternalRefs)
	if err != nil {
		return VisionInput{}, ErrVisionInput
	}
	totalRefs := len(files) + len(assetIDs) + len(externalRefs)
	if totalRefs < 1 || totalRefs > visionMaxImages {
		return VisionInput{}, ErrVisionInput
	}
	images := make([]VisionImage, 0, totalRefs)
	for _, header := range files {
		image, err := readMultipartVisionImage(header)
		if err != nil {
			return VisionInput{}, ErrVisionInput
		}
		images = append(images, image)
	}
	materialized, err := r.materializer.Materialize(request.Context(), ownerUserID, assetIDs, externalRefs)
	if err != nil {
		return VisionInput{}, err
	}
	images = append(images, materialized...)
	if err := validateVisionImages(images); err != nil {
		return VisionInput{}, err
	}
	return VisionInput{Images: images, UserText: request.FormValue("user_text")}, nil
}

func readMultipartVisionImage(header *multipart.FileHeader) (VisionImage, error) {
	if header == nil {
		return VisionImage{}, ErrVisionInput
	}
	file, err := header.Open()
	if err != nil {
		return VisionImage{}, ErrVisionInput
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, visionMaxBytes+1))
	if err != nil || len(data) > visionMaxBytes {
		return VisionImage{}, ErrVisionInput
	}
	return VisionImage{Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data}, nil
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
) ([]VisionImage, error) {
	if len(assetIDs) > 0 && m.libraryReader == nil {
		return nil, ErrVisionInput
	}
	images := make([]VisionImage, 0, len(assetIDs)+len(externalRefs))
	for _, assetID := range assetIDs {
		asset, err := m.libraryReader.GetAsset(ctx, ownerUserID, assetID)
		if err != nil {
			return nil, ErrVisionImageUnavailable
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

func readLibraryVisionImage(ctx context.Context, asset library.Asset) (VisionImage, error) {
	rawURL := asset.OriginalURL
	if strings.TrimSpace(rawURL) == "" {
		rawURL = asset.PreviewURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || !isLibraryImageHost(parsed.Hostname()) {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	response, err := getVisionImage(ctx, parsed)
	if err != nil {
		return VisionImage{}, fmt.Errorf("%w: %w", ErrVisionImageUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, visionMaxBytes+1))
	if err != nil {
		return VisionImage{}, fmt.Errorf("%w: %w", ErrVisionImageUnavailable, err)
	}
	if len(data) > visionMaxBytes {
		return VisionImage{}, ErrVisionImageInvalid
	}
	contentType := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	return VisionImage{Filename: "library-reference", ContentType: contentType, Data: data}, nil
}

func getVisionImage(ctx context.Context, imageURL *url.URL) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL.String(), nil)
	if err != nil {
		return nil, err
	}
	return visionImageRequest(ctx, request)
}

// visionImageRequest is kept as a narrow seam for deterministic tests. The
// production implementation still runs every request through safehttp and
// combines the runtime outbound policy with the exact, curated image CDN
// hosts used by Search references.
var visionImageRequest = func(ctx context.Context, request *http.Request) (*http.Response, error) {
	return safehttp.DoRequestWithAllowHosts(ctx, request, visionSearchTimeout, visionImageAllowHosts(ctx)...)
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
			return nil, ErrVisionInput
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

func pathExt(filename string) string {
	if index := strings.LastIndex(filename, "."); index >= 0 {
		return filename[index:]
	}
	return ""
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
			return nil, ErrVisionInput
		}
		ref, err := decodeExternalReference(trimmed)
		if err != nil {
			return nil, ErrVisionInput
		}
		ref.Provider = strings.ToLower(strings.TrimSpace(ref.Provider))
		ref.ID = strings.TrimSpace(ref.ID)
		ref.PreviewURL = strings.TrimSpace(ref.PreviewURL)
		if ref.Provider == "" || ref.ID == "" || ref.PreviewURL == "" {
			return nil, ErrVisionInput
		}
		if !isAllowedExternalProvider(ref.Provider) {
			return nil, ErrVisionInput
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
func readExternalVisionImage(ctx context.Context, ref VisionExternalReference) (VisionImage, error) {
	parsed, err := url.Parse(strings.TrimSpace(ref.PreviewURL))
	if err != nil || parsed.Scheme != "https" || !isLibraryImageHost(parsed.Hostname()) {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	// Reject anything that is not on the exact CDN host allowlist. The allowlist
	// is shared with the library-asset path so the server-side materialization
	// never depends on third-party CORS headers.
	if !isExternalReferenceHost(parsed.Hostname(), ref.Provider) {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	response, err := getVisionImage(ctx, parsed)
	if err != nil {
		return VisionImage{}, fmt.Errorf("%w: %w", ErrVisionImageUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, visionMaxBytes+1))
	if err != nil {
		return VisionImage{}, fmt.Errorf("%w: %w", ErrVisionImageUnavailable, err)
	}
	if len(data) == 0 || len(data) > visionMaxBytes {
		return VisionImage{}, ErrVisionImageInvalid
	}
	contentType := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	return VisionImage{Filename: ref.Provider + "-" + ref.ID, ContentType: contentType, Data: data}, nil
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
