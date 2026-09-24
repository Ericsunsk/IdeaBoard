// Session user identity (leaf module, zero imports).
//
// Split from `userSession.ts` so `shared/utils/auth` (imported by the API
// client) can reference the type without forming a type-only import cycle
// back through the generated Public client.
export type User = {
  public_id: string
  display_name: string
  avatar: string
  role: string
}
