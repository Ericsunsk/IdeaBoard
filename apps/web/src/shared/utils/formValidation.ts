const REQUIRED_SENTINEL = '__required__:'

/**
 * Mark an error message as a "required-field" error.
 * Required errors are rendered inline as placeholder text instead of
 * as a separate Message element below the field.
 */
export function markRequiredError(message: string): string {
  return `${REQUIRED_SENTINEL}${message}`
}

/** Check whether an error was created by `markRequiredError`. */
export function isRequiredErrorMessage(error: string): boolean {
  return error.startsWith(REQUIRED_SENTINEL)
}

/** Strip the required sentinel so the message is safe to display. */
export function displayErrorMessage(error: string): string {
  return error.startsWith(REQUIRED_SENTINEL) ? error.slice(REQUIRED_SENTINEL.length) : error
}
