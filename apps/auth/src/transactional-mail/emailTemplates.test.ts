import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  mailConfigurationTest,
  passwordResetOtp,
  paymentReceipt,
  securityAlertLogin,
  teamInvite,
  verificationOtp,
  welcomeOnboarding,
  type RenderedEmail,
} from './emailTemplates/index.js'

function assertRenderedEmail(email: RenderedEmail): void {
  assert.ok(email.subject.trim())
  assert.match(email.html, /<!DOCTYPE html>/i)
  assert.ok(email.text.trim())
  assert.doesNotMatch(email.html, /\{\{[A-Z0-9_]+\}\}/)
  assert.doesNotMatch(`${email.subject}\n${email.html}\n${email.text}`, /\p{Script=Han}/u)
}

describe('emailTemplates', () => {
  it('renders verification and password reset OTPs with typed expiry variables', () => {
    const verification = verificationOtp({
      otp: '849201',
      expiresInMinutes: 10,
    })
    const passwordReset = passwordResetOtp({
      otp: '192048',
      expiresInMinutes: 10,
    })

    assertRenderedEmail(verification)
    assertRenderedEmail(passwordReset)
    assert.match(verification.html, /849201/)
    assert.match(verification.html, /10 minutes/)
    assert.match(verification.text, /849201/)
    assert.match(verification.text, /expires in 10 minutes/)
    assert.match(passwordReset.html, /192048/)
    assert.match(passwordReset.text, /expires in 10 minutes/)
    assert.doesNotMatch(passwordReset.html, /RESET_URL/)
  })

  it('renders custom appName and brandLogoUrl when provided', () => {
    const verification = verificationOtp({
      otp: '998877',
      expiresInMinutes: 10,
      appName: 'Design Studio',
      brandLogoUrl: 'https://example.com/custom-logo.png',
    })
    const passwordReset = passwordResetOtp({
      otp: '112233',
      expiresInMinutes: 5,
      appName: 'Design Studio',
      brandLogoUrl: 'https://example.com/custom-logo.png',
    })

    assertRenderedEmail(verification)
    assertRenderedEmail(passwordReset)
    assert.match(verification.subject, /Design Studio/)
    assert.match(verification.html, /https:\/\/example\.com\/custom-logo\.png/)
    assert.match(verification.html, /Design Studio/)
    assert.match(verification.text, /Design Studio/)

    assert.match(passwordReset.subject, /Design Studio/)
    assert.match(passwordReset.html, /https:\/\/example\.com\/custom-logo\.png/)
    assert.match(passwordReset.html, /Design Studio/)
    assert.match(passwordReset.text, /Design Studio/)
  })

  it('resolves relative and remote branding logo URLs accurately', async () => {
    const { resolveBrandingLogoUrl } = await import('./emailTemplates/index.js')

    assert.equal(
      resolveBrandingLogoUrl('', 'https://app.example.com'),
      'https://app.example.com/favicon.svg',
    )
    assert.equal(
      resolveBrandingLogoUrl('/uploads/site/assets/logo.png', 'https://app.example.com'),
      'https://app.example.com/uploads/site/assets/logo.png',
    )
    assert.equal(
      resolveBrandingLogoUrl('/uploads/site/assets/logo.png', 'http://localhost:5173'),
      'http://localhost:5173/uploads/site/assets/logo.png',
    )
    assert.equal(
      resolveBrandingLogoUrl('https://cdn.example.com/logo.png', 'http://localhost:5173'),
      'https://cdn.example.com/logo.png',
    )
    assert.equal(
      resolveBrandingLogoUrl('javascript:alert(1)', 'http://localhost:5173'),
      'http://localhost:5173/favicon.svg',
    )
  })

  it('uses the runtime public origin for default branding and footer links', () => {
    const email = verificationOtp({
      otp: '123456',
      expiresInMinutes: 10,
      publicSiteOrigin: 'https://studio.example.com/',
    })

    assert.match(email.html, /https:\/\/studio\.example\.com\/favicon\.svg/)
    assert.match(email.html, /href="https:\/\/studio\.example\.com"/)
    assert.match(email.html, /🌐 https:\/\/studio\.example\.com/)
  })

  it('escapes text variables and validates URL variables', () => {
    const invitation = teamInvite({
      inviterName: '<Admin & "Owner">',
      boardTitle: 'Roadmap <Q4>',
      roleName: 'Editor & Reviewer',
      acceptInviteUrl: 'https://ideaboard.art/invite?a=1&b=2',
    })

    assertRenderedEmail(invitation)
    assert.match(invitation.html, /&lt;Admin &amp; &quot;Owner&quot;&gt;/)
    assert.match(invitation.html, /Roadmap &lt;Q4&gt;/)
    assert.match(invitation.html, /invite\?a=1&amp;b=2/)
    assert.doesNotMatch(invitation.html, /<Admin/)
    assert.match(invitation.text, /<Admin & "Owner">/)

    assert.throws(
      () =>
        teamInvite({
          inviterName: 'Admin',
          boardTitle: 'Roadmap',
          roleName: 'Editor',
          acceptInviteUrl: 'javascript:alert(1)',
        }),
      /http\(s\) URL/,
    )
  })

  it('renders every transactional template without unresolved variables', () => {
    const rendered = [
      welcomeOnboarding({
        userName: 'Alice',
        canvasUrl: 'https://ideaboard.art/canvas',
      }),
      paymentReceipt({
        amount: '99.00',
        orderId: 'ORD-1',
        planName: 'Pro',
        credits: '1,000',
        totalCredits: '2,500',
        paymentTime: '2026-08-06 17:58:00',
        paymentMethod: 'Stripe',
        accountBillingUrl: 'https://ideaboard.art/account/billing',
      }),
      securityAlertLogin({
        userName: 'Alice',
        loginTime: '2026-08-06 17:58:00',
        deviceInfo: 'macOS / Chrome',
        ipLocation: '127.0.0.1 (Local)',
        secureAccountUrl: 'https://ideaboard.art/account/security',
      }),
      mailConfigurationTest(),
    ]

    rendered.forEach(assertRenderedEmail)
  })

  it('rejects invalid OTP and expiry values', () => {
    assert.throws(() => verificationOtp({ otp: '12345', expiresInMinutes: 10 }), /6 位数字/)
    assert.throws(() => verificationOtp({ otp: '123456', expiresInMinutes: 0 }), /有效期/)
  })

  it('never reads the public origin from process.env', () => {
    process.env.PUBLIC_ORIGIN = 'https://env-bypass.example.com'
    try {
      const withoutOrigin = verificationOtp({
        otp: '123456',
        expiresInMinutes: 10,
      })
      assert.doesNotMatch(withoutOrigin.html, /env-bypass\.example\.com/)
      assert.match(withoutOrigin.html, /http:\/\/localhost:5173\/favicon\.svg/)

      const withOrigin = verificationOtp({
        otp: '123456',
        expiresInMinutes: 10,
        publicSiteOrigin: 'https://studio.example.com/',
      })
      assert.match(withOrigin.html, /https:\/\/studio\.example\.com\/favicon\.svg/)
      assert.doesNotMatch(withOrigin.html, /env-bypass\.example\.com/)
    } finally {
      delete process.env.PUBLIC_ORIGIN
    }
  })
})
