import {
  authClient,
  extractBetterAuthErrorMessage,
  normalizeCanonicalEmail,
} from '../../../shared/auth/client'
import { resolveAuthUrl } from '../../../shared/config/runtimeUrls'

async function requestAuthEndpoint(
  path: string,
  init: RequestInit,
  fallback: string,
): Promise<unknown> {
  const response = await fetch(resolveAuthUrl(path), init)
  const payload: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    throw new Error(extractBetterAuthErrorMessage(payload, fallback))
  }
  return payload
}

export const authTransport = {
  async login(identifier: string, password: string): Promise<void> {
    const normalizedIdentifier = identifier.trim()
    const result = normalizedIdentifier.includes('@')
      ? await authClient.signIn.email({
          email: normalizeCanonicalEmail(normalizedIdentifier),
          password,
        })
      : await authClient.signIn.username({
          username: normalizedIdentifier.toLowerCase(),
          password,
        })
    if (result?.error) {
      throw new Error(extractBetterAuthErrorMessage(result.error, '登录失败'))
    }
  },

  async register(email: string, username: string, password: string): Promise<void> {
    const canonicalEmail = normalizeCanonicalEmail(email)
    const normalizedUsername = username.trim()
    const result = await authClient.signUp.email({
      email: canonicalEmail,
      name: normalizedUsername,
      username: normalizedUsername,
      password,
    })
    if (result?.error) {
      throw new Error(extractBetterAuthErrorMessage(result.error, '注册失败'))
    }
    if (!result?.data?.user) {
      throw new Error('注册成功，但服务端未返回用户身份')
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
      '验证码无效或已过期',
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
      '无法发送验证码，请稍后重试',
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
      '验证码无效或密码更新失败',
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
      '当前密码错误',
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
      '无法发送 Email 验证码',
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
      'Email 验证码无效或已过期',
    )
    await requestAuthEndpoint(
      '/api/auth/revoke-other-sessions',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
      '其他 Session 撤销失败',
    )
  },

  async logout(): Promise<boolean> {
    try {
      const result = await authClient.signOut()
      if (result?.error) {
        console.warn(
          '[auth] signOut failed',
          extractBetterAuthErrorMessage(result.error, '退出登录失败，请稍后重试'),
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
      '密码更新失败',
    )
  },

  useSession() {
    return authClient.useSession()
  },

  async startImpersonation(authUserId: string): Promise<void> {
    const { error } = await authClient.admin.impersonateUser({ userId: authUserId })
    if (error) {
      throw new Error(extractBetterAuthErrorMessage(error, '伪装失败'))
    }
  },

  async stopImpersonating(): Promise<void> {
    const { error } = await authClient.admin.stopImpersonating()
    if (error) {
      throw new Error(extractBetterAuthErrorMessage(error, '退出伪装失败'))
    }
  },
}
