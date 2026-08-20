package searchcatalog

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ideaboard/api/internal/library"
)

type VisionInputReader interface {
	Read(*http.Request, int64) (VisionInput, error)
}

type multipartVisionInputReader struct {
	libraryReader VisionLibraryAssetReader
}

func newMultipartVisionInputReader(libraryReader VisionLibraryAssetReader) VisionInputReader {
	return multipartVisionInputReader{libraryReader: libraryReader}
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
	assetIDs, err := parseLibraryAssetIDs(rawAssetIDs)
	if err != nil || len(files)+len(assetIDs) < 1 || len(files)+len(assetIDs) > visionMaxImages {
		return VisionInput{}, ErrVisionInput
	}
	images := make([]VisionImage, 0, len(files)+len(assetIDs))
	for _, header := range files {
		image, err := readMultipartVisionImage(header)
		if err != nil {
			return VisionInput{}, ErrVisionInput
		}
		images = append(images, image)
	}
	if len(assetIDs) > 0 && r.libraryReader == nil {
		return VisionInput{}, ErrVisionInput
	}
	for _, assetID := range assetIDs {
		asset, err := r.libraryReader.GetAsset(request.Context(), ownerUserID, assetID)
		if err != nil {
			return VisionInput{}, ErrVisionImageUnavailable
		}
		image, err := readLibraryVisionImage(request.Context(), asset)
		if err != nil {
			return VisionInput{}, err
		}
		images = append(images, image)
	}
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
		return VisionImage{}, fmt.Errorf("%w: %v", ErrVisionImageUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return VisionImage{}, ErrVisionImageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, visionMaxBytes+1))
	if err != nil {
		return VisionImage{}, fmt.Errorf("%w: %v", ErrVisionImageUnavailable, err)
	}
	if len(data) > visionMaxBytes {
		return VisionImage{}, ErrVisionImageInvalid
	}
	contentType := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	return VisionImage{Filename: "library-reference", ContentType: contentType, Data: data}, nil
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
