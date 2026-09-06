import { useUserStore } from '../../auth/session/authSession'
import { impersonateAdminUser } from './adminUserApi'

export type AdminUserRedirect = {
  schedule: (url: string, delayMs: number) => void
}

export type AdminUserSessionCommands = {
  startImpersonation: (publicId: string) => Promise<void>
}

function createBrowserRedirect(): AdminUserRedirect {
  return {
    schedule(url, delayMs) {
      setTimeout(() => {
        window.location.href = url
      }, delayMs)
    },
  }
}

export function createAdminUserSessionCommands(
  redirect: AdminUserRedirect = createBrowserRedirect(),
): AdminUserSessionCommands {
  const userStore = useUserStore()

  return {
    async startImpersonation(publicId: string) {
      await impersonateAdminUser(publicId)

      userStore.clearAuth()
      redirect.schedule('/', 1000)
    },
  }
}
