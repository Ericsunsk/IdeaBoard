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
  prompt_unavailable: 'This prompt is unavailable right now. Please try again later.',
  wallet_restricted: 'Ideas are temporarily unavailable for this account.',
  vision_search_not_configured: 'Visual Search is unavailable right now.',
  vision_search_unavailable: 'Visual Search is unavailable right now. Please try again later.',
  vision_search_input_invalid: 'Check your search and try again.',
  vision_search_image_invalid: 'This image cannot be used for Visual Search.',
  vision_search_image_unavailable: 'This image is unavailable. Try another image.',
  vision_search_timeout: 'Visual Search took too long. Please try again.',
  vision_search_credentials: 'Visual Search is unavailable right now.',
  vision_search_quota: 'Visual Search is temporarily unavailable. Please try again later.',
  vision_search_network: 'Could not reach Visual Search. Please try again.',
  vision_search_provider_error: 'Visual Search failed. Please try again.',
  vision_search_schema_error: 'Visual Search returned an invalid result. Please try again.',
  vision_search_idempotency_key_required: 'Visual Search could not start. Please try again.',
  vision_search_idempotency_conflict: 'This Visual Search is already running.',
  vision_search_pricing_unavailable: 'Visual Search pricing is temporarily unavailable.',
  checkout_request_rejected: 'Payment could not be started. Please try again.',
  checkout_session_expired: 'This payment session has expired. Start a new payment.',
  payment_already_completed: 'This payment has already been completed.',
} as const
