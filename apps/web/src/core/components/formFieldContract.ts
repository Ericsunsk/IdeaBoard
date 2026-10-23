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

export interface FormFieldStateOptions {
  fieldId?: string | undefined
  fallbackId: string
  error?: string | undefined
  hint?: string | undefined
}

/**
 * Single derivation for every FieldShell adapter (core `IbFormField`,
 * Admin `AdminFormField`, auth `AuthFieldShell` via `IbFormField`).
 * Adapters only supply their surface tokens/markup plus the id prefix;
 * ids, `invalid` and the `describedby` chain always come from here so
 * `describedbyId`/`invalid` stay identical across surfaces.
 */
export function resolveFormFieldState(
  options: FormFieldStateOptions,
): Omit<FormFieldContractSlotProps, 'required'> {
  const fieldId = options.fieldId || options.fallbackId
  const invalid = Boolean(options.error)
  const errorId = invalid ? `${fieldId}-error` : undefined
  const hintId = options.hint ? `${fieldId}-hint` : undefined
  return {
    fieldId,
    invalid,
    ariaInvalid: invalid ? true : undefined,
    errorId,
    hintId,
    describedbyId: errorId ?? hintId,
  }
}
