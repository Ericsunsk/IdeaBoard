package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// Storage error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display and never carry credentials,
// endpoints, or provider payloads.
//
// Every storage failure below renders through response.MachineError, never
// through the numeric response.Error envelope. 5xx outcomes are logged
// server-side (InternalError semantics) so they stay attributable; 4xx
// outcomes render without logging.
//
// The storage_profile_invalid outcome preserves the curated UserMessage copy
// produced by the provider drivers (the same safe-message contract the
// vision search edge renders through MachineError): those messages are
// written for display and carry no secrets. The full underlying error only
// ever reaches server logs.
// The values below alias the kernel registry
// (platform/contract.CodeStorage*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeStorageInvalidInput       = string(contract.CodeStorageInvalidInput)
	codeStorageServiceUnavailable = string(contract.CodeStorageServiceUnavailable)
	codeStorageProfileInvalid     = string(contract.CodeStorageProfileInvalid)
	codeStorageOperationFailed    = string(contract.CodeStorageOperationFailed)
)

// writeStorageInputError renders request-shape failures (malformed JSON)
// through the machine envelope so callers branch on a stable code.
func writeStorageInputError(c *gin.Context) {
	errorrender.Render(c, http.StatusBadRequest, codeStorageInvalidInput, msgStorageInvalidInput)
}

// writeStorageUnavailable renders a missing service dependency. A nil
// service is a server misconfiguration, so it is logged like a 5xx.
func writeStorageUnavailable(c *gin.Context) {
	errorrender.RenderDefault(c, "storage service unavailable",
		http.StatusServiceUnavailable, codeStorageServiceUnavailable, msgStorageServiceUnavailable, nil)
}

// writeStorageListError renders profile-listing failures. Listing only fails
// on infrastructure errors, so it is always a logged 5xx.
func writeStorageListError(c *gin.Context, err error) {
	errorrender.RenderDefault(c, "storage list operation failed",
		http.StatusInternalServerError, codeStorageOperationFailed, msgStorageOperationFailed, err)
}

func writeStorageCommandError(c *gin.Context, fallback string, err error) {
	var userMsgErr userMessageError
	if err != nil && errors.As(err, &userMsgErr) {
		errorrender.Render(c, http.StatusBadRequest, codeStorageProfileInvalid, userMsgErr.UserMessage())
		return
	}
	errorrender.RenderDefault(c, "storage operation failed",
		http.StatusInternalServerError, codeStorageOperationFailed, fallback, err)
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgStorageInvalidInput        = "参数错误"                      // codeStorageInvalidInput
	msgStorageServiceUnavailable  = "存储服务暂不可用"                  // codeStorageServiceUnavailable
	msgStorageOperationFailed     = "获取存储列表失败"                  // codeStorageOperationFailed
	msgCreateProfile              = "保存失败，请检查配置或稍后重试"           // CreateProfile @ handler.go
	msgDeleteProfile              = "删除失败，请检查配置或稍后重试"           // DeleteProfile @ handler.go
	msgDeleteProfile2             = "删除成功"                      // DeleteProfile @ handler.go
	msgApplyProfile               = "设为当前失败，请检查配置或稍后重试"         // ApplyProfile @ handler.go
	msgApplyProfile2              = "已设为当前存储"                   // ApplyProfile @ handler.go
	msgTestProfile                = "连接测试失败，请检查配置或稍后重试"         // TestProfile @ handler.go
	msgTestProfile2               = "连接成功"                      // TestProfile @ handler.go
	msgGetBucketInfo              = "获取 Bucket 信息失败，请检查配置或稍后重试" // GetBucketInfo @ handler.go
	msgSaveProfile                = "存储服务未初始化"                  // SaveProfile @ handler.go
	msgStorageSaveProfileMessage  = "已保存并设为当前存储"                // StorageSaveProfileMessage @ handler.go
	msgStorageSaveProfileMessage2 = "保存成功"                      // StorageSaveProfileMessage @ handler.go
)
