import { readFileSync } from "node:fs";

export type RenderedEmail = Readonly<{
  subject: string;
  html: string;
  text: string;
}>;

export type VerificationOtpVariables = Readonly<{
  otp: string;
  expiresInMinutes: number;
}>;

export type PasswordResetOtpVariables = VerificationOtpVariables;

export type WelcomeOnboardingVariables = Readonly<{
  userName: string;
  canvasUrl: string;
}>;

export type PaymentReceiptVariables = Readonly<{
  amount: string;
  orderId: string;
  planName: string;
  credits: string;
  totalCredits: string;
  paymentTime: string;
  paymentMethod: string;
  accountBillingUrl: string;
}>;

export type TeamInviteVariables = Readonly<{
  inviterName: string;
  boardTitle: string;
  roleName: string;
  acceptInviteUrl: string;
}>;

export type SecurityAlertLoginVariables = Readonly<{
  userName: string;
  loginTime: string;
  deviceInfo: string;
  ipLocation: string;
  secureAccountUrl: string;
}>;

type HTMLVariable = Readonly<{
  value: string;
  kind: "text" | "url";
}>;

const APP_NAME = "IdeaBoard";
const BRAND_LOGO_URL = "https://ideaboard.art/favicon.svg";
const unresolvedVariablePattern = /\{\{[A-Z0-9_]+\}\}/g;
const templateCache = new Map<string, string>();

export class EmailTemplateError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "EmailTemplateError";
  }
}

export function verificationOtp(
  variables: VerificationOtpVariables,
): RenderedEmail {
  const otp = validOtp(variables.otp);
  const expiresInMinutes = validExpiry(variables.expiresInMinutes);
  return {
    subject: `你的 ${APP_NAME} Email 验证码`,
    html: renderHTML("verification-otp.html", {
      OTP_CODE: textVariable(otp),
      EXPIRES_IN_MINUTES: textVariable(String(expiresInMinutes)),
    }),
    text: `${otp} 是你的 ${APP_NAME} Email 验证码，${expiresInMinutes} 分钟内有效。如非本人操作，请忽略此邮件。`,
  };
}

export function passwordResetOtp(
  variables: PasswordResetOtpVariables,
): RenderedEmail {
  const otp = validOtp(variables.otp);
  const expiresInMinutes = validExpiry(variables.expiresInMinutes);
  return {
    subject: `重置你的 ${APP_NAME} Password`,
    html: renderHTML("password-reset-otp.html", {
      OTP_CODE: textVariable(otp),
      EXPIRES_IN_MINUTES: textVariable(String(expiresInMinutes)),
    }),
    text: `${otp} 是你的 ${APP_NAME} Password 重置验证码，${expiresInMinutes} 分钟内有效。如非本人操作，请忽略此邮件。`,
  };
}

export function welcomeOnboarding(
  variables: WelcomeOnboardingVariables,
): RenderedEmail {
  const userName = requiredText(variables.userName, "userName");
  const canvasUrl = validURL(variables.canvasUrl, "canvasUrl");
  return {
    subject: `欢迎加入 ${APP_NAME}`,
    html: renderHTML("welcome-onboarding.html", {
      USER_NAME: textVariable(userName),
      CANVAS_URL: urlVariable(canvasUrl),
    }),
    text: `Hi ${userName}，欢迎加入 ${APP_NAME}！打开你的画布：${canvasUrl}`,
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
  return {
    subject: `${APP_NAME} 付款收据 ${orderId}`,
    html: renderHTML("payment-receipt.html", {
      AMOUNT: textVariable(amount),
      ORDER_ID: textVariable(orderId),
      PLAN_NAME: textVariable(planName),
      CREDITS: textVariable(credits),
      TOTAL_CREDITS: textVariable(totalCredits),
      PAYMENT_TIME: textVariable(paymentTime),
      PAYMENT_METHOD: textVariable(paymentMethod),
      ACCOUNT_BILLING_URL: urlVariable(accountBillingUrl),
    }),
    text: [
      `${APP_NAME} 付款成功`,
      `金额：¥${amount}`,
      `订单号：${orderId}`,
      `套餐：${planName}（${credits} Ideas）`,
      `支付时间：${paymentTime}`,
      `支付渠道：${paymentMethod}`,
      `当前可用 Ideas：${totalCredits}`,
      `账单管理：${accountBillingUrl}`,
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
  return {
    subject: `${inviterName} 邀请你加入 ${APP_NAME} 画板`,
    html: renderHTML("team-invite.html", {
      INVITER_NAME: textVariable(inviterName),
      BOARD_TITLE: textVariable(boardTitle),
      ROLE_NAME: textVariable(roleName),
      ACCEPT_INVITE_URL: urlVariable(acceptInviteUrl),
    }),
    text: `${inviterName} 邀请你以“${roleName}”身份加入画板“${boardTitle}”。接受邀请：${acceptInviteUrl}`,
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
  return {
    subject: `${APP_NAME} 新设备登录安全提醒`,
    html: renderHTML("login-security-alert.html", {
      USER_NAME: textVariable(userName),
      LOGIN_TIME: textVariable(loginTime),
      DEVICE_INFO: textVariable(deviceInfo),
      IP_LOCATION: textVariable(ipLocation),
      SECURE_ACCOUNT_URL: urlVariable(secureAccountUrl),
    }),
    text: [
      `Hi ${userName}，你的 ${APP_NAME} 账号刚才在一台新设备上完成了登录。`,
      `登录时间：${loginTime}`,
      `设备与浏览器：${deviceInfo}`,
      `IP 地址 / 位置：${ipLocation}`,
      `如果这不是你本人操作，请立即保护账号：${secureAccountUrl}`,
    ].join("\n"),
  };
}

export function mailConfigurationTest(): RenderedEmail {
  return {
    subject: `${APP_NAME} 邮件服务测试`,
    html: renderHTML("mail-configuration-test.html", {}),
    text: `${APP_NAME} 邮件服务配置测试成功。`,
  };
}

function renderHTML(
  templateName: string,
  variables: Readonly<Record<string, HTMLVariable>>,
): string {
  const template = loadTemplate(templateName);
  const allVariables: Record<string, HTMLVariable> = {
    APP_NAME: textVariable(APP_NAME),
    BRAND_LOGO_URL: urlVariable(BRAND_LOGO_URL),
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
