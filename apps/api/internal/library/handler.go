package library

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

type folderDTO struct {
	ID         int64  `json:"id"`
	QueryKey   string `json:"query_key"`
	Name       string `json:"name"`
	IsSystem   bool   `json:"is_system"`
	IsDefault  bool   `json:"is_default"`
	IsVirtual  bool   `json:"is_virtual"`
	AssetCount int64  `json:"asset_count"`
}

type assetDTO struct {
	ID          int64  `json:"id"`
	FolderID    int64  `json:"folder_id"`
	Provider    string `json:"provider"`
	SourceID    string `json:"source_id"`
	SourceURL   string `json:"source_url"`
	PreviewURL  string `json:"preview_url"`
	OriginalURL string `json:"original_url"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Width       int32  `json:"width"`
	Height      int32  `json:"height"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type folderListResponse struct {
	Folders []folderDTO `json:"folders"`
}

type assetListResponse struct {
	Items      []assetDTO `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}

type createFolderRequest struct {
	Name string `json:"name"`
}

type saveAssetRequest struct {
	FolderID      *int64     `json:"folder_id"`
	NewFolderName string     `json:"new_folder_name"`
	Asset         AssetInput `json:"asset"`
}

type moveAssetRequest struct {
	FolderID int64 `json:"folder_id"`
}

type lookupAssetsRequest struct {
	Keys []AssetKey `json:"keys"`
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) ListFolders(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	folders, err := h.service.ListFolders(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取素材文件夹失败", err)
		return
	}
	result := make([]folderDTO, 0, len(folders))
	for _, folder := range folders {
		result = append(result, toFolderDTO(folder))
	}
	response.SuccessDTO(c, folderListResponse{Folders: result})
}

func (h *Handler) CreateFolder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeLibraryError(c, ErrInvalidFolderName)
		return
	}
	folder, err := h.service.CreateFolder(c.Request.Context(), userID, req.Name)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	response.SuccessDTO(c, toFolderDTO(folder))
}

func (h *Handler) RenameFolder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	folderID, ok := parsePositiveID(c.Param("folder_id"))
	if !ok {
		writeLibraryError(c, ErrFolderNotFound)
		return
	}
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeLibraryError(c, ErrInvalidFolderName)
		return
	}
	folder, err := h.service.RenameFolder(c.Request.Context(), userID, folderID, req.Name)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	response.SuccessDTO(c, toFolderDTO(folder))
}

func (h *Handler) DeleteFolder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	folderID, ok := parsePositiveID(c.Param("folder_id"))
	if !ok {
		writeLibraryError(c, ErrSystemFolderImmutable)
		return
	}
	if err := h.service.DeleteFolder(c.Request.Context(), userID, folderID); err != nil {
		writeLibraryError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *Handler) ListAssets(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	selection, err := ParseFolderSelection(c.Query("folder_id"))
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	result, err := h.service.ListAssets(c.Request.Context(), userID, selection, c.Query("cursor"), limit)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	items := make([]assetDTO, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAssetDTO(item))
	}
	response.SuccessDTO(c, assetListResponse{Items: items, NextCursor: result.NextCursor, HasMore: result.HasMore})
}

func (h *Handler) SaveAsset(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req saveAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeLibraryError(c, ErrInvalidSource)
		return
	}
	selection, err := selectionFromFolderID(req.FolderID)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	asset, err := h.service.SaveAsset(c.Request.Context(), SaveAssetCommand{OwnerUserID: userID, FolderSelection: selection, NewFolderName: req.NewFolderName, Asset: req.Asset})
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	response.SuccessDTO(c, toAssetDTO(asset))
}

func selectionFromFolderID(folderID *int64) (*FolderSelection, error) {
	if folderID == nil {
		return nil, nil
	}
	if *folderID <= 0 {
		return nil, ErrInvalidTargetFolder
	}
	selection, err := ParseFolderSelection(strconv.FormatInt(*folderID, 10))
	if err != nil {
		return nil, ErrInvalidTargetFolder
	}
	return &selection, nil
}

func (h *Handler) MoveAsset(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	assetID, ok := parsePositiveID(c.Param("asset_id"))
	if !ok {
		writeLibraryError(c, ErrAssetNotFound)
		return
	}
	var req moveAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeLibraryError(c, ErrInvalidTargetFolder)
		return
	}
	asset, err := h.service.MoveAsset(c.Request.Context(), userID, assetID, req.FolderID)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	response.SuccessDTO(c, toAssetDTO(asset))
}

func (h *Handler) DeleteAsset(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	assetID, ok := parsePositiveID(c.Param("asset_id"))
	if !ok {
		writeLibraryError(c, ErrAssetNotFound)
		return
	}
	if err := h.service.DeleteAsset(c.Request.Context(), userID, assetID); err != nil {
		writeLibraryError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *Handler) LookupAssets(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req lookupAssetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeLibraryError(c, ErrInvalidSource)
		return
	}
	assets, err := h.service.LookupAssets(c.Request.Context(), userID, req.Keys)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	items := make([]assetDTO, 0, len(assets))
	for _, asset := range assets {
		items = append(items, toAssetDTO(asset))
	}
	response.SuccessDTO(c, assetListResponse{Items: items})
}

func requireUserID(c *gin.Context) (int64, bool) {
	identity, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identity.UserID(), true
}

func parsePositiveID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return id, err == nil && id > 0
}

func parseLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultPageSize, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > MaxPageSize {
		return 0, ErrInvalidPageSize
	}
	return limit, nil
}

func writeLibraryError(c *gin.Context, err error) {
	status, code := http.StatusBadRequest, "library_request_invalid"
	switch {
	case errors.Is(err, ErrFolderNameConflict):
		status, code = http.StatusConflict, "library_folder_name_conflict"
	case errors.Is(err, ErrSystemFolderImmutable):
		status, code = http.StatusConflict, "library_folder_system_immutable"
	case errors.Is(err, ErrFolderNotFound):
		status, code = http.StatusNotFound, "library_folder_not_found"
	case errors.Is(err, ErrAssetNotFound):
		status, code = http.StatusNotFound, "library_asset_not_found"
	case errors.Is(err, ErrInvalidProvider):
		code = "library_asset_provider_invalid"
	case errors.Is(err, ErrInvalidSource):
		code = "library_asset_source_invalid"
	case errors.Is(err, ErrInvalidTargetFolder):
		code = "library_asset_target_folder_invalid"
	case errors.Is(err, ErrInvalidFolderName):
		code = "library_folder_name_invalid"
	case errors.Is(err, ErrInvalidCursor):
		code = "library_cursor_invalid"
	case errors.Is(err, ErrInvalidPageSize):
		code = "library_page_size_invalid"
	case errors.Is(err, ErrInvalidOwner):
		status = http.StatusUnauthorized
	}
	response.MachineError(c, status, code, "素材库请求失败")
}

func toFolderDTO(folder Folder) folderDTO {
	return folderDTO{ID: folder.ID, QueryKey: folder.QueryKey, Name: folder.Name, IsSystem: folder.IsSystem, IsDefault: folder.IsDefault, IsVirtual: folder.IsVirtual, AssetCount: folder.AssetCount}
}

func toAssetDTO(asset Asset) assetDTO {
	return assetDTO{ID: asset.ID, FolderID: asset.FolderID, Provider: asset.Provider, SourceID: asset.SourceID, SourceURL: asset.SourceURL, PreviewURL: asset.PreviewURL, OriginalURL: asset.OriginalURL, Title: asset.Title, Author: asset.Author, Width: asset.Width, Height: asset.Height, CreatedAt: asset.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"), UpdatedAt: asset.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")}
}
