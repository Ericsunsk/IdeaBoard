/** Cancellation is a fact (code/name), never message prose. */
export const PROCESSING_CANCELLED_NAME = 'ImageProcessingCancelled'

export function processingCancelledError(): Error {
  const error = new Error('Image processing cancelled')
  error.name = PROCESSING_CANCELLED_NAME
  return error
}

export function isProcessingCancelled(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  return (
    error.name === PROCESSING_CANCELLED_NAME || (error as { code?: unknown }).code === 'cancelled'
  )
}
