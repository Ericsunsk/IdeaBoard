import {
  authClient,
  resolveBetterAuthErrorMessage,
  normalizeCanonicalEmail,
} from '../../../shared/auth/client'
import { resolveAuthUrl } from '../../../shared/config/runtimeUrls'
import { USER_AUTH_CALLBACK_PATH } from '../../../shared/constants/routePaths'
import { isSafeInternalRedirectPath } from '../../../shared/utils/auth'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'
import { i18n } from '../../../shared/i18n'

async function requestAuthEndpoint(
  path: string,
  init: RequestInit,
  fallback: string,
): Promise<unknown> {
  const response = await fetch(resolveAuthUrl(path), init)
  const payload: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    throw new Error(resolveBetterAuthErrorMessage(payload, fallback))
  }
  return payload
}

export const authTransport = {
  async login(email: string, password: string): Promise<void> {
    const result = await authClient.signIn.email({
      email: normalizeCanonicalEmail(email),
      password,
    })
    if (result?.error) {
      throw new Error(
        resolveBetterAuthErrorMessage(result.error, i18n.global.t('auth.errors.signInFailed')),
      )
    }
  },

  async register(email: string, displayName: string, password: string): Promise<void> {
    const canonicalEmail = normalizeCanonicalEmail(email)
    const normalizedDisplayName = displayName.trim()
    const result = await authClient.signUp.email({
      email: canonicalEmail,
      name: normalizedDisplayName,
      password,
    })
    if (result?.error) {
      throw new Error(
        resolveBetterAuthErrorMessage(result.error, i18n.global.t('auth.errors.signUpFailed')),
      )
    }
    if (!result?.data?.user) {
      throw new Error(i18n.global.t('auth.errors.signUpIdentityMissing'))
    }
  },

  async signInWithOAuth(provider: OAuthProviderCode, redirectPath?: string): Promise<void> {
    const currentURL = new URL(window.location.href)
    const callbackURL = new URL(USER_AUTH_CALLBACK_PATH, window.location.origin)
    const requestedRedirect = redirectPath ?? currentURL.searchParams.get('redirect')?.trim() ?? ''
    if (isSafeInternalRedirectPath(requestedRedirect)) {
      callbackURL.searchParams.set('redirect', requestedRedirect)
    }
    const result = await authClient.signIn.social({
      provider,
      callbackURL: callbackURL.toString(),
      errorCallbackURL: callbackURL.toString(),
    })
    if (result?.error) {
      throw new Error(
        resolveBetterAuthErrorMessage(
          result.error,
          i18n.global.t('auth.errors.oauthSignInFailed', {
            provider: getOAuthProviderLabel(provider),
          }),
        ),
      )
    }
  },

  async getCapabilities(): Promise<{
    social: Record<OAuthProviderCode, boolean>
    providers?:
      Array<{ provider: OAuthProviderCode; label: string; available: boolean }> | undefined
  }> {
    try {
      const response = await fetch(resolveAuthUrl('/api/auth/capabilities'), {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!response.ok) return { social: {} }
      const payload = (await response.json().catch(() => null)) as {
        data?: {
          social?: Record<string, unknown>
          providers?: Array<{ provider?: unknown; label?: unknown; available?: unknown }>
        }
      } | null
      const social = payload?.data?.social
      return {
        social: Object.fromEntries(
          Object.entries(social ?? {}).map(([provider, available]) => [
            provider,
            available === true,
          ]),
        ),
        providers: payload?.data?.providers?.flatMap((provider) =>
          typeof provider.provider === 'string' && typeof provider.label === 'string'
            ? [
                {
                  provider: provider.provider,
                  label: provider.label,
                  available: provider.available === true,
                },
              ]
            : [],
        ),
      }
    } catch {
      return { social: {} }
    }
  },

  async verifyRegistrationOTP(email: string, otp: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/email-otp/verify-email',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: normalizeCanonicalEmail(email), otp: otp.trim() }),
      },
      i18n.global.t('auth.errors.otpInvalidOrExpired'),
    )
  },

  async requestPasswordResetOTP(email: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/email-otp/request-password-reset',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: normalizeCanonicalEmail(email) }),
      },
      i18n.global.t('auth.errors.otpSendFailed'),
    )
  },

  async resetPasswordWithOTP(email: string, otp: string, password: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/email-otp/reset-password',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: normalizeCanonicalEmail(email),
          otp: otp.trim(),
          password,
        }),
      },
      i18n.global.t('auth.errors.passwordResetFailed'),
    )
  },

  async verifyCurrentPassword(password: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/verify-password',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      },
      i18n.global.t('auth.errors.currentPasswordIncorrect'),
    )
  },

  async requestEmailChangeOTP(newEmail: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/email-otp/request-email-change',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newEmail: normalizeCanonicalEmail(newEmail) }),
      },
      i18n.global.t('auth.errors.emailCodeSendFailed'),
    )
  },

  async changeEmailWithOTP(newEmail: string, otp: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/email-otp/change-email',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newEmail: normalizeCanonicalEmail(newEmail), otp: otp.trim() }),
      },
      i18n.global.t('auth.errors.otpInvalidOrExpired'),
    )
    await requestAuthEndpoint(
      '/api/auth/revoke-other-sessions',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
      i18n.global.t('auth.errors.otherSessionsRevokeFailed'),
    )
  },

  async logout(): Promise<boolean> {
    try {
      const result = await authClient.signOut()
      if (result?.error) {
        console.warn(
          '[auth] signOut failed',
          resolveBetterAuthErrorMessage(result.error, i18n.global.t('auth.errors.signOutFailed')),
        )
        return false
      }
    } catch (error) {
      console.warn('[auth] signOut failed', error)
      return false
    }
    return true
  },

  async hasSession(): Promise<boolean> {
    try {
      const result = await authClient.getSession()
      return Boolean(result?.data?.session)
    } catch {
      return false
    }
  },

  async updatePassword(oldPassword: string, newPassword: string): Promise<void> {
    await requestAuthEndpoint(
      '/api/auth/change-password',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          currentPassword: oldPassword,
          newPassword,
          revokeOtherSessions: true,
        }),
      },
      i18n.global.t('auth.errors.passwordUpdateFailed'),
    )
  },

  async getSessionUser(): Promise<{ email?: string } | null> {
    try {
      const result = await authClient.getSession()
      if (!result?.data?.user) return null
      return {
        email: result.data.user.email,
      }
    } catch {
      return null
    }
  },

  useSession() {
    return authClient.useSession()
  },

  async stopImpersonating(): Promise<void> {
    const { error } = await authClient.admin.stopImpersonating()
    if (error) {
      throw new Error(
        resolveBetterAuthErrorMessage(error, i18n.global.t('auth.errors.stopImpersonatingFailed')),
      )
    }
  },
}
