import { buildContractRequestHeaders, newRequestId } from '@/api/http/contractKernel'

/** Common options carried by every Admin Control mutation. */
export type ControlMutationOptions = {
  idempotencyKey?: string | undefined
}

/** Build Control headers while preserving caller-owned retry keys. */
export function buildControlMutationHeaders(
  options: ControlMutationOptions = {},
  defaultIdempotencyKey?: string,
): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? defaultIdempotencyKey ?? newRequestId(),
  })
}
