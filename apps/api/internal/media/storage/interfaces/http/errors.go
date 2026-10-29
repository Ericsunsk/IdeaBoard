package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"

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
	errorrender.Render(c, http.StatusBadRequest, codeStorageInvalidInput, errorrender.MsgStorageInvalidInput)
}

// writeStorageUnavailable renders a missing service dependency. A nil
// service is a server misconfiguration, so it shares the fail-closed 503
// service_unavailable guard with every other HTTP edge.
func writeStorageUnavailable(c *gin.Context) {
	platformhttp.RequireDependencyCode(c, false, codeStorageServiceUnavailable, errorrender.MsgStorageServiceUnavailable)
}

// writeStorageListError renders profile-listing failures. Listing only fails
// on infrastructure errors, so it is always a logged 5xx.
func writeStorageListError(c *gin.Context, err error) {
	errorrender.RenderDefault(c, "storage list operation failed",
		http.StatusInternalServerError, codeStorageOperationFailed, errorrender.MsgStorageOperationFailed, err)
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
