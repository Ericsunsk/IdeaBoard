/**
 * English copy keyed by stable backend error codes.
 * Codes come from generated-query models and must stay untouched.
 * Feature-local mappings (billing readiness, search, task events) resolve
 * their own codes to copy in their modules; only cross-cutting codes live here.
 */
export default {
  submission_invalid: 'Check the request and try again.',
  submission_failed: 'Could not submit the request. Please try again.',
  insufficient_credits: 'You need more Ideas to run this action.',
  pricing_unavailable: 'Pricing is temporarily unavailable. Please try again shortly.',
  provider_unavailable: 'This service is unavailable right now. Please try again later.',
  service_resolution_unavailable: 'This resolution is unavailable right now.',
  prompt_unavailable: 'This prompt is unavailable right now. Please try again later.',
  wallet_restricted: 'Ideas are temporarily unavailable for this account.',
  member_quota_exhausted: 'You have used up your personal quota for this period.',
  vision_search_unavailable: 'Visual Search is unavailable right now. Please try again later.',
  vision_search_input_invalid: 'Check your search and try again.',
  vision_search_provider_error: 'Visual Search failed. Please try again.',
  idempotency_key_required: 'The request could not start. Please try again.',
  idempotency_conflict: 'This request is already in progress.',
  checkout_request_rejected: 'Payment could not be started. Please try again.',
  checkout_session_expired: 'This payment session has expired. Start a new payment.',
  payment_already_completed: 'This payment has already been completed.',
  board_forbidden: 'You cannot operate this board.',
  task_forbidden: 'You cannot operate this task.',
  board_archived: 'This board is archived and cannot accept this operation.',
  board_not_found: 'Board not found.',
  board_access_conflict: 'Access changed elsewhere. Refresh and try again.',
  board_precondition_required: 'Access state is outdated. Refresh and try again.',
  board_member_not_found: 'Board member not found.',
  organization_not_found: 'Organization not found.',
  organization_member_not_found: 'Organization member not found.',
  organization_limit_reached: 'Each user may join or create at most one organization.',
  organization_closed: 'This organization is closed.',
  organization_closure_blocked:
    'The organization still has unsettled tasks or funds. Settle them and retry.',
  membership_transition_forbidden: 'The last manager cannot be removed or demoted.',
} as const
