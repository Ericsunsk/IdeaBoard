// Session user view type (issue #809).
//
// This type lives in its own leaf module (zero imports) on purpose: the
// dependency-cruiser `no-circular` rule counts even type-only edges, and
// `shared/api/userSession` (runtime: generated `getMe` transport) sits
// downstream of `shared/api/client`. Importing this type from `userSession`
// would close a client → utils/auth → userSession → orvalMutator → client
// cycle. Feature code keeps importing this `User` view type (never generated
// code directly).
export type User = {
  public_id: string
  display_name?: string
  avatar?: string
  role?: string | null
  email?: string
  personal_workspace_id?: string
  storage_namespace?: string
  status?: number
  created_at?: string
  updated_at?: string
}
