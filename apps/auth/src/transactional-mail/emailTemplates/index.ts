import { readFileSync } from "node:fs";

export type RenderedEmail = Readonly<{
  subject: string;
  html: string;
  text: string;
}>;

type EmailBrandingVariables = {
  appName?: string;
  brandLogoUrl?: string;
  publicSiteOrigin?: string;
};

export type VerificationOtpVariables = Readonly<{
  otp: string;
  expiresInMinutes: number;
} & EmailBrandingVariables>;

export type PasswordResetOtpVariables = VerificationOtpVariables;

export type WelcomeOnboardingVariables = Readonly<{
  userName: string;
  canvasUrl: string;
} & EmailBrandingVariables>;

export type PaymentReceiptVariables = Readonly<{
  amount: string;
  orderId: string;
  planName: string;
  credits: string;
  totalCredits: string;
  paymentTime: string;
  paymentMethod: string;
  accountBillingUrl: string;
} & EmailBrandingVariables>;

export type TeamInviteVariables = Readonly<{
  inviterName: string;
  boardTitle: string;
  roleName: string;
  acceptInviteUrl: string;
} & EmailBrandingVariables>;

export type SecurityAlertLoginVariables = Readonly<{
  userName: string;
  loginTime: string;
  deviceInfo: string;
  ipLocation: string;
  secureAccountUrl: string;
} & EmailBrandingVariables>;

export type MailConfigurationTestVariables = Readonly<EmailBrandingVariables>;

type HTMLVariable = Readonly<{
  value: string;
  kind: "text" | "url";
}>;

const APP_NAME = "IdeaBoard";
const DEFAULT_PUBLIC_SITE_ORIGIN = "http://localhost:5173";
const unresolvedVariablePattern = /\{\{[A-Z0-9_]+\}\}/g;
const templateCache = new Map<string, string>();

export class EmailTemplateError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "EmailTemplateError";
  }
}

export function resolveBrandingLogoUrl(
  logoUrl?: string,
  baseUrl?: string,
): string {
  const fallback = `${resolvePublicSiteOrigin(baseUrl)}/favicon.svg`;
  const trimmed = logoUrl?.trim() ?? "";
  if (!trimmed) {
    return fallback;
  }
  if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) {
    try {
      return validURL(trimmed, "brandLogoUrl");
    } catch {
      return fallback;
    }
  }
  if (trimmed.startsWith("/") && baseUrl?.trim()) {
    try {
      const base = new URL(baseUrl.trim());
      const resolved = new URL(trimmed, base.origin);
      return validURL(resolved.toString(), "brandLogoUrl");
    } catch {
      return fallback;
    }
  }
  return fallback;
}

export function verificationOtp(
  variables: VerificationOtpVariables,
): RenderedEmail {
  const otp = validOtp(variables.otp);
  const expiresInMinutes = validExpiry(variables.expiresInMinutes);
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `Your ${appName} verification code`,
    html: renderHTML(
      "verification-otp.html",
      {
        OTP_CODE: textVariable(otp),
        EXPIRES_IN_MINUTES: textVariable(String(expiresInMinutes)),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: `${otp} is your ${appName} verification code. It expires in ${expiresInMinutes} minutes. If you did not request it, you can safely ignore this email.`,
  };
}

export function passwordResetOtp(
  variables: PasswordResetOtpVariables,
): RenderedEmail {
  const otp = validOtp(variables.otp);
  const expiresInMinutes = validExpiry(variables.expiresInMinutes);
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `Reset your ${appName} password`,
    html: renderHTML(
      "password-reset-otp.html",
      {
        OTP_CODE: textVariable(otp),
        EXPIRES_IN_MINUTES: textVariable(String(expiresInMinutes)),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: `${otp} is your ${appName} password reset code. It expires in ${expiresInMinutes} minutes. If you did not request it, you can safely ignore this email.`,
  };
}

export function welcomeOnboarding(
  variables: WelcomeOnboardingVariables,
): RenderedEmail {
  const userName = requiredText(variables.userName, "userName");
  const canvasUrl = validURL(variables.canvasUrl, "canvasUrl");
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `Welcome to ${appName}`,
    html: renderHTML(
      "welcome-onboarding.html",
      {
        USER_NAME: textVariable(userName),
        CANVAS_URL: urlVariable(canvasUrl),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: `Hi ${userName}, welcome to ${appName}. Open your canvas: ${canvasUrl}`,
  };
}

export function paymentReceipt(
  variables: PaymentReceiptVariables,
): RenderedEmail {
  const amount = requiredText(variables.amount, "amount");
  const orderId = requiredText(variables.orderId, "orderId");
  const planName = requiredText(variables.planName, "planName");
  const credits = requiredText(variables.credits, "credits");
  const totalCredits = requiredText(variables.totalCredits, "totalCredits");
  const paymentTime = requiredText(variables.paymentTime, "paymentTime");
  const paymentMethod = requiredText(variables.paymentMethod, "paymentMethod");
  const accountBillingUrl = validURL(
    variables.accountBillingUrl,
    "accountBillingUrl",
  );
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `${appName} receipt · ${orderId}`,
    html: renderHTML(
      "payment-receipt.html",
      {
        AMOUNT: textVariable(amount),
        ORDER_ID: textVariable(orderId),
        PLAN_NAME: textVariable(planName),
        CREDITS: textVariable(credits),
        TOTAL_CREDITS: textVariable(totalCredits),
        PAYMENT_TIME: textVariable(paymentTime),
        PAYMENT_METHOD: textVariable(paymentMethod),
        ACCOUNT_BILLING_URL: urlVariable(accountBillingUrl),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: [
      `Payment received by ${appName}`,
      `Amount: ¥${amount}`,
      `Order: ${orderId}`,
      `Plan: ${planName} (${credits} Ideas)`,
      `Paid: ${paymentTime}`,
      `Method: ${paymentMethod}`,
      `Available Ideas: ${totalCredits}`,
      `Manage billing: ${accountBillingUrl}`,
    ].join("\n"),
  };
}

export function teamInvite(variables: TeamInviteVariables): RenderedEmail {
  const inviterName = requiredText(variables.inviterName, "inviterName");
  const boardTitle = requiredText(variables.boardTitle, "boardTitle");
  const roleName = requiredText(variables.roleName, "roleName");
  const acceptInviteUrl = validURL(
    variables.acceptInviteUrl,
    "acceptInviteUrl",
  );
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `${inviterName} invited you to ${appName}`,
    html: renderHTML(
      "team-invite.html",
      {
        INVITER_NAME: textVariable(inviterName),
        BOARD_TITLE: textVariable(boardTitle),
        ROLE_NAME: textVariable(roleName),
        ACCEPT_INVITE_URL: urlVariable(acceptInviteUrl),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: `${inviterName} invited you to join the “${boardTitle}” board as ${roleName}. Accept the invitation: ${acceptInviteUrl}`,
  };
}

export function securityAlertLogin(
  variables: SecurityAlertLoginVariables,
): RenderedEmail {
  const userName = requiredText(variables.userName, "userName");
  const loginTime = requiredText(variables.loginTime, "loginTime");
  const deviceInfo = requiredText(variables.deviceInfo, "deviceInfo");
  const ipLocation = requiredText(variables.ipLocation, "ipLocation");
  const secureAccountUrl = validURL(
    variables.secureAccountUrl,
    "secureAccountUrl",
  );
  const appName = variables.appName?.trim() || APP_NAME;
  const brandLogoUrl = variables.brandLogoUrl?.trim() || `${resolvePublicSiteOrigin(variables.publicSiteOrigin)}/favicon.svg`;
  return {
    subject: `New sign-in to your ${appName} account`,
    html: renderHTML(
      "login-security-alert.html",
      {
        USER_NAME: textVariable(userName),
        LOGIN_TIME: textVariable(loginTime),
        DEVICE_INFO: textVariable(deviceInfo),
        IP_LOCATION: textVariable(ipLocation),
        SECURE_ACCOUNT_URL: urlVariable(secureAccountUrl),
      },
      { appName, brandLogoUrl, publicSiteOrigin: variables.publicSiteOrigin },
    ),
    text: [
      `Hi ${userName}, a new device just signed in to your ${appName} account.`,
      `Time: ${loginTime}`,
      `Device and browser: ${deviceInfo}`,
      `IP address and location: ${ipLocation}`,
      `If this was not you, secure your account now: ${secureAccountUrl}`,
    ].join("\n"),
  };
}

export function mailConfigurationTest(
  variables?: MailConfigurationTestVariables,
): RenderedEmail {
  const appName = variables?.appName?.trim() || APP_NAME;
  const publicSiteOrigin = resolvePublicSiteOrigin(variables?.publicSiteOrigin);
  const brandLogoUrl = variables?.brandLogoUrl?.trim() || `${publicSiteOrigin}/favicon.svg`;
  return {
    subject: `${appName} email service test`,
    html: renderHTML("mail-configuration-test.html", {}, { appName, brandLogoUrl, publicSiteOrigin }),
    text: `${appName} email service configuration test succeeded.`,
  };
}

function renderHTML(
  templateName: string,
  variables: Readonly<Record<string, HTMLVariable>>,
  options?: { appName?: string; brandLogoUrl?: string; publicSiteOrigin?: string },
): string {
  const template = loadTemplate(templateName);
  const appName = options?.appName?.trim() || APP_NAME;
  const publicSiteOrigin = resolvePublicSiteOrigin(options?.publicSiteOrigin);
  const brandLogoUrl = options?.brandLogoUrl?.trim()
    ? validURL(options.brandLogoUrl.trim(), "brandLogoUrl")
    : `${publicSiteOrigin}/favicon.svg`;
  const allVariables: Record<string, HTMLVariable> = {
    APP_NAME: textVariable(appName),
    BRAND_LOGO_URL: urlVariable(brandLogoUrl),
    // The origin has already been validated and normalized; keep its canonical
    // serialization without URL.toString() adding a trailing slash.
    PUBLIC_SITE_ORIGIN: textVariable(publicSiteOrigin),
    BRAND_LOGO_SVG: textVariable(""),
    ...variables,
  };
  let html = template;
  for (const [name, variable] of Object.entries(allVariables)) {
    const safeValue =
      variable.kind === "url"
        ? escapeHTML(validURL(variable.value, name))
        : escapeHTML(variable.value);
    html = html.replaceAll(`{{${name}}}`, safeValue);
  }
  const unresolved = [...new Set(html.match(unresolvedVariablePattern) ?? [])];
  if (unresolved.length > 0) {
    throw new EmailTemplateError(
      `模板 ${templateName} 存在未赋值变量：${unresolved.join(", ")}`,
    );
  }
  return html;
}

function resolvePublicSiteOrigin(value?: string): string {
  const candidate = value?.trim() || process.env.PUBLIC_ORIGIN?.trim() || process.env.BETTER_AUTH_URL?.trim() || DEFAULT_PUBLIC_SITE_ORIGIN;
  try {
    const url = new URL(candidate);
    if ((url.protocol !== "http:" && url.protocol !== "https:") || url.username || url.password) {
      throw new Error("unsupported URL");
    }
    return url.origin;
  } catch {
    return DEFAULT_PUBLIC_SITE_ORIGIN;
  }
}

function loadTemplate(templateName: string): string {
  const cached = templateCache.get(templateName);
  if (cached !== undefined) return cached;
  try {
    const template = readFileSync(
      new URL(`./templates/${templateName}`, import.meta.url),
      "utf8",
    );
    templateCache.set(templateName, template);
    return template;
  } catch (error) {
    throw new EmailTemplateError(
      `无法读取 Transactional Email 模板 ${templateName}: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
}

function textVariable(value: string): HTMLVariable {
  return { kind: "text", value };
}

function urlVariable(value: string): HTMLVariable {
  return { kind: "url", value };
}

function requiredText(value: string, name: string): string {
  const normalized = value.trim();
  if (!normalized || /[\u0000-\u001f\u007f]/.test(normalized)) {
    throw new EmailTemplateError(`${name} 必须是非空且不含控制字符的文本`);
  }
  return normalized;
}

function validOtp(value: string): string {
  const otp = value.trim();
  if (!/^\d{6}$/.test(otp)) {
    throw new EmailTemplateError("otp 必须是 6 位数字");
  }
  return otp;
}

function validExpiry(value: number): number {
  if (!Number.isInteger(value) || value < 1 || value > 24 * 60) {
    throw new EmailTemplateError("有效期必须是 1 到 1440 分钟的整数");
  }
  return value;
}

function validURL(value: string, name: string): string {
  try {
    const url = new URL(value.trim());
    if (
      (url.protocol !== "http:" && url.protocol !== "https:") ||
      url.username ||
      url.password
    ) {
      throw new Error("unsupported URL");
    }
    return url.toString();
  } catch {
    throw new EmailTemplateError(`${name} 必须是无凭据的有效 http(s) URL`);
  }
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
