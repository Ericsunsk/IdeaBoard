import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  mailConfigurationTest,
  passwordResetOtp,
  paymentReceipt,
  securityAlertLogin,
  teamInvite,
  verificationOtp,
  welcomeOnboarding,
  type RenderedEmail,
} from "./emailTemplates/index.js";

function assertRenderedEmail(email: RenderedEmail): void {
  assert.ok(email.subject.trim());
  assert.match(email.html, /<!DOCTYPE html>/i);
  assert.ok(email.text.trim());
  assert.doesNotMatch(email.html, /\{\{[A-Z0-9_]+\}\}/);
}

describe("emailTemplates", () => {
  it("renders verification and password reset OTPs with typed expiry variables", () => {
    const verification = verificationOtp({
      otp: "849201",
      expiresInMinutes: 10,
    });
    const passwordReset = passwordResetOtp({
      otp: "192048",
      expiresInMinutes: 10,
    });

    assertRenderedEmail(verification);
    assertRenderedEmail(passwordReset);
    assert.match(verification.html, /849201/);
    assert.match(verification.html, /10 分钟/);
    assert.match(verification.text, /849201/);
    assert.match(passwordReset.html, /192048/);
    assert.match(passwordReset.text, /10 分钟/);
    assert.doesNotMatch(passwordReset.html, /RESET_URL|重置链接/);
  });

  it("escapes text variables and validates URL variables", () => {
    const invitation = teamInvite({
      inviterName: '<Admin & "Owner">',
      boardTitle: "Roadmap <Q4>",
      roleName: "Editor & Reviewer",
      acceptInviteUrl: "https://ideaboard.art/invite?a=1&b=2",
    });

    assertRenderedEmail(invitation);
    assert.match(invitation.html, /&lt;Admin &amp; &quot;Owner&quot;&gt;/);
    assert.match(invitation.html, /Roadmap &lt;Q4&gt;/);
    assert.match(invitation.html, /invite\?a=1&amp;b=2/);
    assert.doesNotMatch(invitation.html, /<Admin/);
    assert.match(invitation.text, /<Admin & "Owner">/);

    assert.throws(
      () =>
        teamInvite({
          inviterName: "Admin",
          boardTitle: "Roadmap",
          roleName: "Editor",
          acceptInviteUrl: "javascript:alert(1)",
        }),
      /http\(s\) URL/,
    );
  });

  it("renders every transactional template without unresolved variables", () => {
    const rendered = [
      welcomeOnboarding({
        userName: "Alice",
        canvasUrl: "https://ideaboard.art/canvas",
      }),
      paymentReceipt({
        amount: "99.00",
        orderId: "ORD-1",
        planName: "Pro",
        credits: "1,000",
        totalCredits: "2,500",
        paymentTime: "2026-08-06 17:58:00",
        paymentMethod: "Stripe",
        accountBillingUrl: "https://ideaboard.art/account/billing",
      }),
      securityAlertLogin({
        userName: "Alice",
        loginTime: "2026-08-06 17:58:00",
        deviceInfo: "macOS / Chrome",
        ipLocation: "127.0.0.1 (Local)",
        secureAccountUrl: "https://ideaboard.art/account/security",
      }),
      mailConfigurationTest(),
    ];

    rendered.forEach(assertRenderedEmail);
  });

  it("rejects invalid OTP and expiry values", () => {
    assert.throws(
      () => verificationOtp({ otp: "12345", expiresInMinutes: 10 }),
      /6 位数字/,
    );
    assert.throws(
      () => verificationOtp({ otp: "123456", expiresInMinutes: 0 }),
      /有效期/,
    );
  });
});
