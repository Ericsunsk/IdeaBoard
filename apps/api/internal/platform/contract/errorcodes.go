package contract

import (
	"net/http"

	"ideaboard/api/internal/platform/idempotency"
)

// Code is a stable machine-readable error code (Baseline 5.3). Clients
// must branch on Code, never on message text. Codes are wire-stable: do
// not rename without coordinating the OpenAPI contract and callers.
type Code string

const (
	// CodePaginationCursorInvalid renders when a list cursor is malformed,
	// oversized, or bound to different principal/filters/sort (400).
	CodePaginationCursorInvalid Code = "pagination_cursor_invalid"
	// CodeTaskIdempotencyConflict renders when an Idempotency-Key is reused
	// with a different task request (409).
	CodeTaskIdempotencyConflict Code = "task_idempotency_conflict"

	// CodeIdempotencyKeyRequired and CodeIdempotencyConflict are promoted
	// to global from platform/idempotency. The values are aliased (not
	// copied) so the kernel registry and the legacy edge helper cannot
	// drift apart.
	CodeIdempotencyKeyRequired Code = Code(idempotency.CodeRequired)
	CodeIdempotencyConflict    Code = Code(idempotency.CodeConflict)

	// Generic kernel codes mapped from the Baseline 5.4 HTTP status
	// matrix. Per-resource codes freeze in #801+; these cover the
	// transport-level outcomes the kernel itself renders.
	CodeBadRequest         Code = "bad_request"
	CodeUnauthorized       Code = "unauthorized"
	CodeForbidden          Code = "forbidden"
	CodeNotFound           Code = "not_found"
	CodeConflict           Code = "conflict"
	CodeValidationFailed   Code = "validation_failed"
	CodeRateLimited        Code = "rate_limited"
	CodeServiceUnavailable Code = "service_unavailable"

	// CodeContractNotImplemented is a kernel-stub placeholder (HTTP 501)
	// for the additive /api/v1 and /api/control/v1 stub groups. It is not
	// a public error code: real handlers in #801+ replace the stubs and
	// this code goes away with them.
	CodeContractNotImplemented Code = "contract_not_implemented"

	// Workspace / organization lifecycle codes (issue #801). Wire-stable:
	// do not rename without coordinating the OpenAPI contract and callers.
	// Hidden resources (cross-workspace invisible, unknown ids, malformed
	// id shapes at lookup) converge to the *_not_found codes (404) per
	// Baseline 4 anti-enumeration; visible-but-forbidden renders 403.
	CodeWorkspaceNotFound               Code = "workspace_not_found"
	CodeWorkspaceForbidden              Code = "workspace_forbidden"
	CodeWorkspaceIdempotencyConflict    Code = "workspace_idempotency_conflict"
	CodeOrganizationNotFound            Code = "organization_not_found"
	CodeOrganizationForbidden           Code = "organization_forbidden"
	CodeOrganizationIdempotencyConflict Code = "organization_idempotency_conflict"
	CodeWorkspaceMemberNotFound         Code = "workspace_member_not_found"
	CodeMembershipTransitionForbidden   Code = "membership_transition_forbidden"

	// Workspace-rooted board collaboration codes (issue #802). Wire-stable:
	// do not rename without coordinating the OpenAPI contract and callers.
	// Hidden boards (unknown ids, malformed board_ shapes at lookup, or the
	// caller holds no board access) converge to board_not_found (404) per
	// Baseline 4 anti-enumeration; visible-but-forbidden renders 403.
	// Stale If-Match renders 409 board_revision_conflict; Idempotency-Key
	// reuse with a different request renders 409
	// board_idempotency_conflict.
	CodeBoardNotFound            Code = "board_not_found"
	CodeBoardForbidden           Code = "board_forbidden"
	CodeBoardRevisionConflict    Code = "board_revision_conflict"
	CodeBoardIdempotencyConflict Code = "board_idempotency_conflict"
	CodeBoardMemberNotFound      Code = "board_member_not_found"
	CodeBoardShareTokenNotFound  Code = "board_share_token_not_found"
	CodeBoardShareTokenInvalid   Code = "board_share_token_invalid"

	// Workspace upload session + media asset codes (issue #803).
	// Wire-stable: do not rename without coordinating the OpenAPI
	// contract and callers. Upload sessions are ephemeral transfer
	// coordination (pending -> transferring -> completed, with
	// transferring -> failed and pending/transferring -> expired);
	// only a successful complete creates the durable Media Asset row.
	// Hidden resources (cross-workspace invisible, unknown ids,
	// malformed id shapes at lookup) converge to the *_not_found
	// codes (404) per Baseline 4 anti-enumeration. Expired sessions
	// and archived assets render 410 so clients stop retrying.
	CodeUploadNotFound            Code = "upload_not_found"
	CodeUploadExpired             Code = "upload_expired"
	CodeUploadStateConflict       Code = "upload_state_conflict"
	CodeUploadIdempotencyConflict Code = "upload_idempotency_conflict"
	CodeUploadValidationFailed    Code = "upload_validation_failed"
	CodeMediaNotFound             Code = "media_not_found"
	CodeMediaNotReady             Code = "media_not_ready"
	CodeMediaArchived             Code = "media_archived"
	CodeMediaIdempotencyConflict  Code = "media_idempotency_conflict"
	CodeMediaValidationFailed     Code = "media_validation_failed"

	// Workspace billing codes (issue #804). Wire-stable: do not rename
	// without coordinating the OpenAPI contract and callers. The wallet
	// gate renders billing_wallet_restricted as 403 for frozen/restricted
	// wallets and 404 for missing wallets (same code, no provisioning
	// state leak); checkout key reuse renders billing_checkout_conflict
	// (409); unknown sessions render billing_session_not_found (404);
	// server-side price snapshot failures render
	// billing_quote_unavailable (503).
	CodeBillingWalletRestricted Code = "billing_wallet_restricted"
	CodeBillingCheckoutConflict Code = "billing_checkout_conflict"
	CodeBillingSessionNotFound  Code = "billing_session_not_found"
	CodeBillingQuoteUnavailable Code = "billing_quote_unavailable"

	// Workspace task lifecycle codes (issue #805). Wire-stable: do not
	// rename without coordinating the OpenAPI contract and callers.
	// Hidden tasks (cross-workspace invisible, unknown ids, malformed
	// task_ shapes at lookup) converge to task_not_found (404) per
	// Baseline 4 anti-enumeration; visible-but-forbidden renders 403
	// task_forbidden. Terminal-state mutations render 409
	// task_state_conflict; Idempotency-Key reuse with a different
	// request renders 409 task_idempotency_conflict (kernel code).
	// Insufficient Ideas render 402 task_insufficient_credits.
	CodeTaskNotFound            Code = "task_not_found"
	CodeTaskForbidden           Code = "task_forbidden"
	CodeTaskStateConflict       Code = "task_state_conflict"
	CodeTaskInsufficientCredits Code = "task_insufficient_credits"

	// Workspace library codes (issue #806). Wire-stable: do not rename
	// without coordinating the OpenAPI contract and callers. Library
	// assets are saved external references (provider URL + source
	// metadata, never bytes). Hidden resources (cross-workspace
	// invisible, unknown ids, malformed id shapes at lookup — including
	// legacy integer ids, which v1 never accepts) converge to the
	// *_not_found codes (404) per Baseline 4 anti-enumeration.
	// Idempotency-Key reuse with a different request renders
	// library_idempotency_conflict (409). Billing failures reuse the
	// #804 codes (billing_quote_unavailable 503,
	// billing_wallet_restricted 403); no new billing codes here.
	CodeLibraryFolderNotFound          Code = "library_folder_not_found"
	CodeLibraryAssetNotFound           Code = "library_asset_not_found"
	CodeLibraryMaterializationNotFound Code = "library_materialization_not_found"
	CodeLibraryIdempotencyConflict     Code = "library_idempotency_conflict"

	// Workspace search codes (issue #806). Wire-stable: do not rename
	// without coordinating the OpenAPI contract and callers. Query and
	// provider-filter validation reuse validation_failed (422); malformed
	// cursors reuse pagination_cursor_invalid (400). All providers
	// failed or unconfigured renders search_provider_unavailable (503);
	// vision provider failures render search_vision_provider_error (502)
	// and unconfigured vision renders search_vision_not_configured (503).
	// Vision billing failures reuse the #804 codes. Idempotency-Key reuse
	// with a different vision request renders search_idempotency_conflict
	// (409).
	CodeSearchProviderUnavailable Code = "search_provider_unavailable"
	CodeSearchVisionProviderError Code = "search_vision_provider_error"
	CodeSearchVisionNotConfigured Code = "search_vision_not_configured"
	CodeSearchIdempotencyConflict Code = "search_idempotency_conflict"

	// Workspace event stream + webhook codes (issue #807). Wire-stable:
	// do not rename without coordinating the openapi.events.yaml contract
	// and callers. Hidden workspaces (malformed ws_ shapes, unknown ids,
	// or caller holds no membership) converge to event_not_found (404)
	// per Baseline 4 anti-enumeration; visible-but-forbidden subscription
	// management renders 403 event_forbidden. Unknown subscription ids
	// converge to event_subscription_not_found (404); Idempotency-Key
	// reuse with a different request renders 409
	// event_subscription_conflict. Control audit records are never
	// exposed on this surface (no audit codes exist here by
	// construction; see #808).
	CodeEventNotFound             Code = "event_not_found"
	CodeEventForbidden            Code = "event_forbidden"
	CodeEventSubscriptionNotFound Code = "event_subscription_not_found"
	CodeEventSubscriptionConflict Code = "event_subscription_conflict"

	// Control Plane codes (issue #808). Wire-stable: do not rename
	// without coordinating openapi.control.yaml and callers. All
	// mutating control routes require an operator reason
	// (control_reason_required, 400); capability denials reuse
	// forbidden (403); idempotency reuse-with-different-request reuses
	// the kernel idempotency_conflict (409); hidden control resources
	// converge to control_not_found (404) per Baseline 4
	// anti-enumeration. Financial control failures reuse the #804
	// billing codes (billing_wallet_restricted, billing_checkout_conflict,
	// billing_quote_unavailable); no second billing code table here.
	CodeControlReasonRequired Code = "control_reason_required"
	CodeControlNotFound       Code = "control_not_found"
)

// StatusOf maps a kernel code to its HTTP status (Baseline 5.4). Unknown
// codes fail closed to 500: an unregistered code is a server bug, never a
// caller mistake.
func StatusOf(code Code) int {
	switch code {
	case CodePaginationCursorInvalid, CodeIdempotencyKeyRequired, CodeBadRequest, CodeBoardShareTokenInvalid,
		CodeControlReasonRequired:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, CodeWorkspaceForbidden, CodeOrganizationForbidden, CodeMembershipTransitionForbidden,
		CodeBoardForbidden, CodeBillingWalletRestricted, CodeTaskForbidden, CodeEventForbidden:
		return http.StatusForbidden
	case CodeNotFound, CodeWorkspaceNotFound, CodeOrganizationNotFound, CodeWorkspaceMemberNotFound,
		CodeBoardNotFound, CodeBoardMemberNotFound, CodeBoardShareTokenNotFound,
		CodeUploadNotFound, CodeMediaNotFound, CodeBillingSessionNotFound,
		CodeLibraryFolderNotFound, CodeLibraryAssetNotFound, CodeLibraryMaterializationNotFound,
		CodeTaskNotFound, CodeEventNotFound, CodeEventSubscriptionNotFound, CodeControlNotFound:
		return http.StatusNotFound
	case CodeUploadExpired, CodeMediaArchived:
		return http.StatusGone
	case CodeTaskIdempotencyConflict, CodeIdempotencyConflict, CodeConflict,
		CodeWorkspaceIdempotencyConflict, CodeOrganizationIdempotencyConflict,
		CodeBoardRevisionConflict, CodeBoardIdempotencyConflict,
		CodeUploadStateConflict, CodeUploadIdempotencyConflict,
		CodeMediaNotReady, CodeMediaIdempotencyConflict, CodeBillingCheckoutConflict,
		CodeLibraryIdempotencyConflict, CodeSearchIdempotencyConflict,
		CodeTaskStateConflict, CodeEventSubscriptionConflict:
		return http.StatusConflict
	case CodeValidationFailed, CodeUploadValidationFailed, CodeMediaValidationFailed:
		return http.StatusUnprocessableEntity
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeServiceUnavailable, CodeBillingQuoteUnavailable,
		CodeSearchProviderUnavailable, CodeSearchVisionNotConfigured:
		return http.StatusServiceUnavailable
	case CodeContractNotImplemented:
		return http.StatusNotImplemented
	case CodeSearchVisionProviderError:
		return http.StatusBadGateway
	case CodeTaskInsufficientCredits:
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}
