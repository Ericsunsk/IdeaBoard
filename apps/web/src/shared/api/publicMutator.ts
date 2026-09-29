import { orvalMutator } from './orvalMutator'

// Public Data Plane transport prefix (issue #809).
//
// The Public OpenAPI contract is canonical with `servers: /api/v1` plus
// relative paths, so orval generates bare paths (`/workspaces`, `/boards`
// roots). The shared `api.request` mounts only `/api`; calling the
// generated Public functions through `orvalMutator` would therefore hit
// `/api/workspaces` (no such route) instead of `/api/v1/workspaces`. This
// mutator prefixes every generated Public path with `/v1` so transport
// stays correct while generated code remains untouched. Feature code must
// still call generated Public functions through its own resource/adapter
// boundary, never this module directly.
export const PUBLIC_API_PREFIX = '/v1'

export function publicMutator<T>(
  url: string,
  options?: Parameters<typeof orvalMutator>[1],
): Promise<T> {
  const normalized = url.startsWith('/') ? `${PUBLIC_API_PREFIX}${url}` : url
  return orvalMutator<T>(normalized, options)
}
