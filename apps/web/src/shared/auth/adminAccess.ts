export type AdminAccessUser = {
  role?: string | null
}

export function canAccessAdmin(user: AdminAccessUser | null | undefined): boolean {
  return user?.role === 'admin'
}
