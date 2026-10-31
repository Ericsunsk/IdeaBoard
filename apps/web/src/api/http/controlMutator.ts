import { orvalMutator } from './orvalMutator'

// Control Plane transport entry for orval codegen (issue 808).
//
// The Control OpenAPI contract is canonical with `servers: /api/control/v1`
// plus relative paths, so orval generates bare paths (`/users`, `/credits/...`).
// The shared `api.request` mounts only `/api`; calling the generated Control
// functions through `orvalMutator` would therefore hit `/api/users` (a legacy
// route) instead of `/api/control/v1/users`. This mutator prefixes every
// generated Control path so transport stays correct while generated code
// remains untouched. Feature code must still call generated Control
// functions through its own resource/adapter boundary, never this module
// directly. The prefix literal is intentionally module-private with this
// module as its single owner: Admin feature helpers under
// `features/admin/shared/adapters/` (headers, DTO mapping) do not duplicate
// this prefix, and `shared` stays free of Admin-only control helpers (#863).
const CONTROL_API_PREFIX = '/control/v1'

export function controlMutator<T>(
  url: string,
  options?: Parameters<typeof orvalMutator>[1],
): Promise<T> {
  const normalized = url.startsWith('/') ? `${CONTROL_API_PREFIX}${url}` : url
  return orvalMutator<T>(normalized, options)
}
