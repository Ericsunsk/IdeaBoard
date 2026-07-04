import { useRouter } from 'vue-router'

type LogoutRedirectDependencies = {
  logout: () => Promise<boolean> | boolean
}

export function useLogoutRedirect(deps: LogoutRedirectDependencies) {
  const router = useRouter()

  async function logoutAndRedirect(path: string, options: { replace?: boolean } = {}) {
    if (!(await deps.logout())) {
      return false
    }
    if (options.replace) {
      await router.replace(path)
    } else {
      await router.push(path)
    }
    return true
  }

  return {
    logoutAndRedirect,
  }
}
