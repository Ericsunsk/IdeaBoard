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
// directly. The prefix literal is intentionally module-private: Admin
// feature URL building owns its own copy at
// `features/admin/shared/core/controlTransport.ts` (#863 keeps Admin-only
// control helpers out of `shared`, and `shared` cannot import that module
// without violating the shared-not-to-features boundary).
const CONTROL_API_PREFIX = '/control/v1'

export function controlMutator<T>(
  url: string,
  options?: Parameters<typeof orvalMutator>[1],
): Promise<T> {
  const normalized = url.startsWith('/') ? `${CONTROL_API_PREFIX}${url}` : url
  return orvalMutator<T>(normalized, options)
}
