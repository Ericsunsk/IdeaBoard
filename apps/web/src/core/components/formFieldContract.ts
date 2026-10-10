/**
 * Neutral contract shared by user-shell and Admin form-field adapters.
 *
 * Keep this contract free of feature tokens and validation implementations.
 * Adapters own their surface markup while controls consume the same semantic
 * ids and accessibility state.
 */
export interface FormFieldContractProps {
  label: string
  hint?: string | undefined
  error?: string | undefined
  required?: boolean | undefined
  fieldId?: string | undefined
}

export interface FormFieldContractSlotProps {
  fieldId: string
  required: boolean
  invalid: boolean
  ariaInvalid: true | undefined
  describedbyId?: string | undefined
  hintId?: string | undefined
  errorId?: string | undefined
}
