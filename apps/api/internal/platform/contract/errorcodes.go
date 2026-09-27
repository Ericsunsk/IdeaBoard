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
	CodeBoardArchived            Code = "board_archived"
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
	// billing_quote_unavailable (503, transient) and unpriced SKUs
	// render billing_pricing_unavailable (422, deterministic).
	CodeBillingWalletRestricted   Code = "billing_wallet_restricted"
	CodeBillingCheckoutConflict   Code = "billing_checkout_conflict"
	CodeBillingSessionNotFound    Code = "billing_session_not_found"
	CodeBillingQuoteUnavailable   Code = "billing_quote_unavailable"
	CodeBillingPricingUnavailable Code = "billing_pricing_unavailable"

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
	// #804 codes (billing_quote_unavailable 503 transient,
	// billing_pricing_unavailable 422 unpriced SKU,
	// billing_wallet_restricted 403).
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

	// Legacy billing catalog codes (billing/interfaces/http edge).
	// Wire-stable: same freeze rule as the kernel #804 codes. The
	// catalog edge renders through the machine envelope; the v1
	// workspace billing surface reuses the #804 kernel codes above.
	CodeBillingInvalidInput       Code = "billing_invalid_input"
	CodeBillingSKUNotFound        Code = "billing_sku_not_found"
	CodeBillingItemNotArchived    Code = "billing_item_not_archived"
	CodeBillingArchivedItemInUse  Code = "billing_archived_item_in_use"
	CodeBillingQuoteTotalExceeded Code = "billing_quote_total_exceeded"
	CodeBillingServiceUnavailable Code = "billing_service_unavailable"
	CodeBillingOperationFailed    Code = "billing_operation_failed"

	// Rewards codes (rewards/interfaces/http edge). Wire-stable: do not
	// rename without coordinating the frontend caller that branches on
	// them. The v1 workspace redeem surface
	// (runtime/interfaces/http/contract_v1_billing_gaps.go) aliases
	// CodeRewardCodeUnavailable, CodeRewardWalletLocked, and
	// CodeRewardRedeemConflict from here; statuses stay explicit at
	// each render site.
	CodeRewardInvalidInput            Code = "reward_invalid_input"
	CodeRewardCodeIDInvalid           Code = "reward_code_id_invalid"
	CodeRewardRedemptionIDInvalid     Code = "reward_redemption_id_invalid"
	CodeRewardPaginationInvalid       Code = "reward_pagination_invalid"
	CodeRewardCodeInvalid             Code = "reward_code_invalid"
	CodeRewardCodeConflict            Code = "reward_code_conflict"
	CodeRewardCodeIdempotencyConflict Code = "reward_code_idempotency_conflict"
	CodeRewardCodeNotFound            Code = "reward_code_not_found"
	CodeRewardCodeCapInvalid          Code = "reward_code_cap_invalid"
	CodeRewardCodeRedeemed            Code = "reward_code_redeemed"
	CodeRewardRedemptionNotFound      Code = "reward_redemption_not_found"
	CodeRewardCodeUnavailable         Code = "reward_code_unavailable"
	CodeRewardRedeemConflict          Code = "reward_redeem_conflict"
	CodeRewardWalletLocked            Code = "reward_wallet_locked"
	CodeRewardRateLimited             Code = "reward_rate_limited"
	CodeRewardServiceUnavailable      Code = "reward_service_unavailable"
	CodeRewardOperationFailed         Code = "reward_operation_failed"
	CodeRewardRedeemFailed            Code = "reward_redeem_failed"

	// Legacy board canvas codes (boards edge). Wire-stable. The v1
	// workspace board surface reuses the #802 kernel codes
	// (board_not_found, board_revision_conflict, board_member_not_found,
	// board_share_token_not_found, board_share_token_invalid); the rows
	// below are the canvas-edge-only outcomes with the same freeze rule.
	CodeBoardInvalidInput      Code = "board_invalid_input"
	CodeBoardNameConflict      Code = "board_name_conflict"
	CodeBoardIDConflict        Code = "board_id_conflict"
	CodeBoardPermissionDenied  Code = "board_permission_denied"
	CodeBoardMemberRoleInvalid Code = "board_member_role_invalid"
	CodeBoardMemberSelfInvalid Code = "board_member_self_invalid"
	CodeBoardShareTokenRevoked Code = "board_share_token_revoked"
	CodeBoardShareTokenExpired Code = "board_share_token_expired"
	CodeBoardOperationFailed   Code = "board_operation_failed"

	// Payments codes (payments/interfaces/http edge). Wire-stable: do
	// not rename without coordinating the frontend. Table defaults that
	// predate the machine-code contract keep the legacy numeric
	// envelope via errorrender.RenderNumeric; every mapped row renders
	// the machine envelope with the code below.
	CodeStripeConfigurationInvalidInput  Code = "stripe_configuration_invalid_input"
	CodeStripePermissionDenied           Code = "stripe_permission_denied"
	CodeStripeConfigurationUnprocessable Code = "stripe_configuration_unprocessable"
	CodeStripeProviderUnavailable        Code = "stripe_provider_unavailable"
	CodeStripeConfigurationChanged       Code = "stripe_configuration_changed"
	CodeCheckoutInvalidInput             Code = "checkout_invalid_input"
	CodeCheckoutConflict                 Code = "checkout_conflict"
	CodePaymentAlreadyCompleted          Code = "payment_already_completed"
	CodeCheckoutSessionExpired           Code = "checkout_session_expired"
	CodeWalletNotRechargeable            Code = "wallet_not_rechargeable"
	CodeCheckoutRequestRejected          Code = "checkout_request_rejected"
	CodePaymentServiceUnavailable        Code = "payment_service_unavailable"
	CodePurchasesUnavailable             Code = "purchases_unavailable"
	CodeRechargeHistoryLimitInvalid      Code = "recharge_history_limit_invalid"
	CodeRechargeHistoryCursorInvalid     Code = "recharge_history_cursor_invalid"
	CodeWebhookSignatureInvalid          Code = "webhook_signature_invalid"
	CodeWebhookConfigurationMissing      Code = "webhook_configuration_missing"
	CodeAdminPaymentOrderFilterInvalid   Code = "admin_payment_order_filter_invalid"
	CodeAdminPaymentOrderCursorInvalid   Code = "admin_payment_order_cursor_invalid"

	// Identity codes (identity/users/interfaces/http edge).
	// Wire-stable: do not rename without coordinating frontend callers.
	// CodeIdentityUpstreamRejected renders with the upstream 401/403
	// status kept explicit at the impersonation edge, never via
	// StatusOf.
	CodeIdentityRequestInvalid      Code = "identity_request_invalid"
	CodeIdentityUnauthorized        Code = "identity_unauthorized"
	CodeIdentityUserPublicIDInvalid Code = "identity_user_public_id_invalid"
	CodeIdentityUserNotFound        Code = "identity_user_not_found"
	CodeIdentityForbidden           Code = "identity_forbidden"
	CodeIdentityConflict            Code = "identity_conflict"
	CodeIdentityUpstreamRejected    Code = "identity_upstream_rejected"
	CodeIdentityUpstreamUnavailable Code = "identity_upstream_unavailable"
	CodeIdentityOperationFailed     Code = "identity_operation_failed"
	CodeIdentityServiceUnavailable  Code = "identity_service_unavailable"

	// Storage codes (media/storage/interfaces/http edge). Wire-stable:
	// do not rename without coordinating the frontend caller that
	// branches on them. The storage_profile_invalid outcome preserves
	// the curated provider UserMessage copy.
	CodeStorageInvalidInput       Code = "storage_invalid_input"
	CodeStorageServiceUnavailable Code = "storage_service_unavailable"
	CodeStorageProfileInvalid     Code = "storage_profile_invalid"
	CodeStorageOperationFailed    Code = "storage_operation_failed"

	// Search provider admin codes (searchcatalog/interfaces/http edge).
	// Wire-stable. CodeSearchProviderUnavailable is the #806 kernel
	// code (503); the provider edge keeps explicit statuses at each
	// render site (nil-dependency paths render 500 with the same code
	// and must not route via StatusOf).
	CodeSearchProviderNotFound       Code = "search_provider_not_found"
	CodeSearchRequestInvalid         Code = "search_request_invalid"
	CodeSearchProviderTestRequired   Code = "search_provider_test_required"
	CodeSearchProviderAPIKeyInvalid  Code = "search_provider_api_key_invalid"
	CodeSearchProviderAPIKeyRequired Code = "search_provider_api_key_required"
	CodeSearchProviderTestFailed     Code = "search_provider_test_failed"

	// Web search v1 codes (searchcatalog search_handler edge).
	// Wire-stable.
	CodeSearchUnavailable   Code = "search_unavailable"
	CodeSearchLimitInvalid  Code = "search_limit_invalid"
	CodeSearchQueryInvalid  Code = "search_query_invalid"
	CodeSearchCursorInvalid Code = "search_cursor_invalid"

	// Vision search codes (searchcatalog vision_input edge).
	// Wire-stable. These are distinct from the #806 kernel vision
	// codes (search_vision_provider_error, search_vision_not_configured):
	// the vision_* strings below are the edge contract pinned by
	// TestWriteVisionErrorUsesStableMachineCodes and mirrored in the
	// vision settlement details; do not merge the two families.
	CodeVisionSearchProviderError    Code = "vision_search_provider_error"
	CodeVisionSearchInputInvalid     Code = "vision_search_input_invalid"
	CodeVisionSearchImageInvalid     Code = "vision_search_image_invalid"
	CodeVisionSearchImageUnavailable Code = "vision_search_image_unavailable"
	CodeVisionSearchNotConfigured    Code = "vision_search_not_configured"
	CodeVisionSearchUnavailable      Code = "vision_search_unavailable"
	CodeVisionSearchSchemaError      Code = "vision_search_schema_error"
	CodeVisionSearchCredentials      Code = "vision_search_credentials"
	CodeVisionSearchQuota            Code = "vision_search_quota"
	CodeVisionSearchTimeout          Code = "vision_search_timeout"
	CodeVisionSearchNetwork          Code = "vision_search_network"
)

// StatusOf maps a kernel code to its HTTP status (Baseline 5.4). Unknown
// codes fail closed to 500: an unregistered code is a server bug, never a
// caller mistake.
func StatusOf(code Code) int {
	switch code {
	case CodePaginationCursorInvalid, CodeIdempotencyKeyRequired, CodeBadRequest, CodeBoardShareTokenInvalid,
		CodeControlReasonRequired,
		CodeBillingInvalidInput,
		CodeRewardInvalidInput, CodeRewardCodeIDInvalid, CodeRewardRedemptionIDInvalid,
		CodeRewardPaginationInvalid, CodeRewardCodeInvalid,
		CodeBoardInvalidInput, CodeBoardMemberRoleInvalid, CodeBoardMemberSelfInvalid,
		CodeBoardShareTokenRevoked, CodeBoardShareTokenExpired,
		CodeIdentityRequestInvalid, CodeIdentityUserPublicIDInvalid,
		CodeStorageInvalidInput, CodeStorageProfileInvalid,
		CodeStripeConfigurationInvalidInput, CodeCheckoutInvalidInput,
		CodeRechargeHistoryLimitInvalid, CodeRechargeHistoryCursorInvalid,
		CodeWebhookSignatureInvalid,
		CodeAdminPaymentOrderFilterInvalid, CodeAdminPaymentOrderCursorInvalid,
		CodeSearchRequestInvalid, CodeSearchProviderAPIKeyInvalid,
		CodeSearchLimitInvalid, CodeSearchQueryInvalid, CodeSearchCursorInvalid,
		CodeSearchProviderTestFailed,
		CodeVisionSearchInputInvalid, CodeVisionSearchImageInvalid,
		CodeVisionSearchImageUnavailable:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeIdentityUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, CodeWorkspaceForbidden, CodeOrganizationForbidden, CodeMembershipTransitionForbidden,
		CodeBoardForbidden, CodeBillingWalletRestricted, CodeTaskForbidden, CodeEventForbidden,
		CodeBoardPermissionDenied, CodeStripePermissionDenied, CodeIdentityForbidden,
		// CodeIdentityUpstreamRejected renders with the upstream 401/403
		// status kept explicit at the impersonation edge; the registry
		// pins 403 as the representative denial.
		CodeIdentityUpstreamRejected:
		return http.StatusForbidden
	case CodeNotFound, CodeWorkspaceNotFound, CodeOrganizationNotFound, CodeWorkspaceMemberNotFound,
		CodeBoardNotFound, CodeBoardMemberNotFound, CodeBoardShareTokenNotFound,
		CodeUploadNotFound, CodeMediaNotFound, CodeBillingSessionNotFound,
		CodeLibraryFolderNotFound, CodeLibraryAssetNotFound, CodeLibraryMaterializationNotFound,
		CodeTaskNotFound, CodeEventNotFound, CodeEventSubscriptionNotFound, CodeControlNotFound,
		CodeBillingSKUNotFound,
		CodeRewardCodeNotFound, CodeRewardRedemptionNotFound,
		CodeIdentityUserNotFound, CodeSearchProviderNotFound:
		return http.StatusNotFound
	case CodeUploadExpired, CodeMediaArchived:
		return http.StatusGone
	case CodeTaskIdempotencyConflict, CodeIdempotencyConflict, CodeConflict,
		CodeWorkspaceIdempotencyConflict, CodeOrganizationIdempotencyConflict,
		CodeBoardRevisionConflict, CodeBoardIdempotencyConflict, CodeBoardArchived,
		CodeUploadStateConflict, CodeUploadIdempotencyConflict,
		CodeMediaNotReady, CodeMediaIdempotencyConflict, CodeBillingCheckoutConflict,
		CodeLibraryIdempotencyConflict, CodeSearchIdempotencyConflict,
		CodeTaskStateConflict, CodeEventSubscriptionConflict,
		CodeBillingItemNotArchived, CodeBillingArchivedItemInUse,
		CodeRewardCodeConflict, CodeRewardCodeIdempotencyConflict,
		CodeRewardCodeCapInvalid, CodeRewardCodeRedeemed, CodeRewardRedeemConflict,
		CodeRewardWalletLocked,
		CodeBoardNameConflict, CodeBoardIDConflict,
		CodeIdentityConflict, CodeStripeConfigurationChanged, CodeCheckoutConflict,
		CodeWalletNotRechargeable,
		CodeSearchProviderTestRequired, CodeSearchProviderAPIKeyRequired:
		return http.StatusConflict
	case CodeValidationFailed, CodeUploadValidationFailed, CodeMediaValidationFailed,
		CodeBillingPricingUnavailable,
		CodeBillingQuoteTotalExceeded, CodeRewardCodeUnavailable,
		CodeStripeConfigurationUnprocessable,
		CodePaymentAlreadyCompleted, CodeCheckoutSessionExpired, CodeCheckoutRequestRejected:
		return http.StatusUnprocessableEntity
	case CodeRateLimited, CodeRewardRateLimited:
		return http.StatusTooManyRequests
	case CodeServiceUnavailable, CodeBillingQuoteUnavailable,
		CodeSearchProviderUnavailable, CodeSearchVisionNotConfigured,
		CodeBillingServiceUnavailable, CodeRewardServiceUnavailable,
		CodeStorageServiceUnavailable, CodeIdentityServiceUnavailable,
		CodeStripeProviderUnavailable, CodePaymentServiceUnavailable,
		CodePurchasesUnavailable, CodeWebhookConfigurationMissing,
		CodeSearchUnavailable,
		CodeVisionSearchUnavailable, CodeVisionSearchNotConfigured:
		return http.StatusServiceUnavailable
	case CodeContractNotImplemented:
		return http.StatusNotImplemented
	case CodeSearchVisionProviderError,
		CodeIdentityUpstreamUnavailable,
		CodeVisionSearchProviderError, CodeVisionSearchSchemaError,
		CodeVisionSearchCredentials, CodeVisionSearchQuota, CodeVisionSearchNetwork:
		return http.StatusBadGateway
	case CodeVisionSearchTimeout:
		return http.StatusGatewayTimeout
	case CodeTaskInsufficientCredits:
		return http.StatusPaymentRequired
	case CodeBillingOperationFailed, CodeRewardOperationFailed, CodeRewardRedeemFailed,
		CodeBoardOperationFailed, CodeStorageOperationFailed, CodeIdentityOperationFailed:
		// Explicit 500 pins: same as the fail-closed default, kept so
		// the registry documents every promoted code's exact status.
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
