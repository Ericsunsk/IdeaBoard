// Central Code -> status registry (single source for HTTP status mapping).
//
// Domains only throw sentinels (errors.New values, never status or copy).
// Each HTTP edge maps sentinel -> Code via an errorrender.Mapping table
// (first-win, errors.Is). Status is never stored in the edge table as
// authoritative truth: RenderMapped and contract.StatusOf both resolve
// status here so Code -> status stays unified.
// Unknown codes fail closed to 500: an unregistered Code is a server bug,
// never a caller mistake.
//
// Code strings are wire-stable (Baseline 5.3/5.4). Do not rename without
// coordinating the OpenAPI contract and callers. This registry mirrors
// platform/contract errorcodes exactly; contract.StatusOf delegates here
// so the mapping lives once. errorrender cannot import contract (import
// cycle: contract -> errorrender), so the cases use string literals that
// must stay byte-identical to the contract.Code values.
package errorrender

import "net/http"

// StatusForCode maps a stable machine code to its HTTP status. Unknown
// codes fail closed to 500.
func StatusForCode(code Code) int {
	switch string(code) {
	case "pagination_cursor_invalid", "idempotency_key_required", "bad_request",
		"billing_invalid_input",
		"credit_invalid_input",
		"reward_invalid_input", "reward_code_id_invalid", "reward_redemption_id_invalid",
		"reward_pagination_invalid", "reward_code_invalid",
		"board_invalid_input", "board_member_role_invalid", "board_member_self_invalid",
		"identity_request_invalid", "identity_user_public_id_invalid",
		"storage_invalid_input", "storage_profile_invalid",
		"stripe_configuration_invalid_input", "checkout_invalid_input",
		"recharge_history_limit_invalid", "recharge_history_cursor_invalid",
		"webhook_signature_invalid",
		"admin_payment_order_filter_invalid", "admin_payment_order_cursor_invalid",
		"search_request_invalid", "search_provider_api_key_invalid",
		"search_limit_invalid", "search_query_invalid", "search_cursor_invalid",
		"search_provider_test_failed",
		"vision_search_input_invalid", "vision_search_image_invalid",
		"vision_search_image_unavailable",
		"task_statistics_request_invalid":
		return http.StatusBadRequest
	case "unauthorized", "identity_unauthorized":
		return http.StatusUnauthorized
	case "forbidden", "workspace_forbidden", "organization_forbidden", "membership_transition_forbidden",
		"workspace_default_protected",
		"board_forbidden", "billing_wallet_restricted", "task_forbidden", "event_forbidden",
		"board_permission_denied", "stripe_permission_denied", "identity_forbidden",
		"billing_account_forbidden",
		"member_quota_exhausted",
		"identity_upstream_rejected":
		return http.StatusForbidden
	case "not_found", "workspace_not_found", "organization_not_found", "workspace_member_not_found",
		"organization_member_not_found",
		"board_not_found", "board_member_not_found",
		"upload_not_found", "media_not_found", "billing_session_not_found",
		"billing_account_not_found",
		"library_folder_not_found", "library_asset_not_found", "library_materialization_not_found",
		"task_not_found", "event_not_found", "event_subscription_not_found", "control_not_found",
		"billing_sku_not_found",
		"credit_wallet_not_found", "credit_charge_not_found", "credit_reconciliation_not_found",
		"reward_code_not_found", "reward_redemption_not_found",
		"identity_user_not_found", "search_provider_not_found":
		return http.StatusNotFound
	case "upload_expired", "media_archived":
		return http.StatusGone
	case "task_idempotency_conflict", "idempotency_conflict", "conflict",
		"workspace_idempotency_conflict", "organization_idempotency_conflict",
		"workspace_member_limit_reached", "organization_limit_reached", "workspace_default_conflict", "workspace_active_tasks",
		"organization_closed", "organization_closure_blocked",
		"board_revision_conflict", "board_idempotency_conflict", "board_archived",
		"upload_state_conflict", "upload_idempotency_conflict",
		"media_not_ready", "media_idempotency_conflict", "billing_checkout_conflict",
		"library_idempotency_conflict", "search_idempotency_conflict",
		"task_state_conflict", "event_subscription_conflict",
		"billing_item_not_archived", "billing_archived_item_in_use",
		"reward_code_conflict", "reward_code_idempotency_conflict",
		"reward_code_cap_invalid", "reward_code_redeemed", "reward_redeem_conflict",
		"reward_wallet_locked",
		"credit_idempotency_conflict",
		"board_name_conflict", "board_id_conflict",
		"identity_conflict", "stripe_configuration_changed", "checkout_conflict",
		"wallet_not_rechargeable", "billing_account_conflict",
		"search_provider_test_required", "search_provider_api_key_required":
		return http.StatusConflict
	case "validation_failed", "upload_validation_failed", "media_validation_failed",
		"service_resolution_unavailable",
		"billing_pricing_unavailable",
		"billing_quote_total_exceeded", "reward_code_unavailable",
		"credit_insufficient_credits", "credit_refund_exceeds_charge",
		"stripe_configuration_unprocessable",
		"payment_already_completed", "checkout_session_expired", "checkout_request_rejected":
		return http.StatusUnprocessableEntity
	case "rate_limited", "reward_rate_limited",
		"task_monitor_concurrency_limited":
		return http.StatusTooManyRequests
	case "service_unavailable", "billing_quote_unavailable",
		"search_provider_unavailable", "search_vision_not_configured",
		"billing_service_unavailable", "reward_service_unavailable",
		"credit_reconciliation_unavailable",
		"storage_service_unavailable", "identity_service_unavailable",
		"stripe_provider_unavailable", "payment_service_unavailable",
		"purchases_unavailable", "webhook_configuration_missing",
		"search_unavailable",
		"vision_search_unavailable", "vision_search_not_configured":
		return http.StatusServiceUnavailable
	case "contract_not_implemented":
		return http.StatusNotImplemented
	case "search_vision_provider_error",
		"identity_upstream_unavailable",
		"vision_search_provider_error", "vision_search_schema_error",
		"vision_search_credentials", "vision_search_quota", "vision_search_network":
		return http.StatusBadGateway
	case "vision_search_timeout":
		return http.StatusGatewayTimeout
	case "media_unsupported_type":
		return http.StatusUnsupportedMediaType
	case "task_insufficient_credits":
		return http.StatusPaymentRequired
	case "board_precondition_required":
		return http.StatusPreconditionRequired
	case "board_access_conflict":
		return http.StatusPreconditionFailed
	case "billing_operation_failed", "reward_operation_failed", "reward_redeem_failed",
		"credit_operation_failed",
		"board_operation_failed", "storage_operation_failed", "identity_operation_failed":
		return http.StatusInternalServerError
	case "credit_wallet_frozen", "credit_wallet_restricted":
		return http.StatusLocked
	default:
		return http.StatusInternalServerError
	}
}
