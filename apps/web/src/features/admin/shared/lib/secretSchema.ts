import * as v from 'valibot'

// Conditionally-required secret field shared by Admin credential forms
// (mail / Stripe / OAuth providers). When not required, blank input means
// "keep existing" and skips validation; otherwise a trimmed, non-empty value
// is required. The required message stays at the call site because copy is
// domain-specific.
export function secretField(isRequired: boolean, requiredMessage: string) {
  return isRequired ? v.pipe(v.string(), v.trim(), v.minLength(1, requiredMessage)) : v.string()
}
