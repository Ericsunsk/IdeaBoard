// Session view: /user provides principal fields, the persisted avatar URL,
// and profile facts (storage namespace, status, timestamps).
// Wire `id` maps to `public_id`; optional workspace ID comes from provisioning.
// Email is hydrated separately from Better Auth by the session store.
// Browser snapshots cache these projections and never override a fresh server read.
//
// Keep this type in a leaf module: importing it from the runtime userSession
// transport would close the client → utils/auth → transport import cycle.
export type User = {
  public_id: string
  display_name?: string
  avatar?: string
  role?: string | null
  personal_workspace_id?: string
  storage_namespace?: string
  status?: number
  created_at?: string
  updated_at?: string
}
