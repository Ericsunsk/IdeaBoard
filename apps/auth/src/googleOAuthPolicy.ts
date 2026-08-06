type RegistrationPolicyReader = () => Promise<boolean>;

type AuthHookContext = {
  path: string;
  body?: unknown;
};

export async function applyGoogleOAuthRequestPolicy(
  context: AuthHookContext,
  getRegistrationEnabled: RegistrationPolicyReader,
): Promise<void> {
  if (!context.body || typeof context.body !== "object") return;
  const body = context.body as Record<string, unknown>;
  if (body.provider !== "google") return;
  if (context.path !== "/sign-in/social" && context.path !== "/link-social") return;

  // Better Auth appends request scopes to the provider defaults. Removing the
  // caller value keeps the server-side provider definition as the allowlist.
  delete body.scopes;
  if (context.path === "/sign-in/social") {
    body.requestSignUp = await readRegistrationPolicyFailClosed(
      getRegistrationEnabled,
    );
  }
}

export function createVerifiedOAuthUserCreationGuard(
  getRegistrationEnabled: RegistrationPolicyReader,
) {
  return async (user: { emailVerified?: unknown }): Promise<boolean | void> => {
    // Email/password users are created unverified in this application. A
    // verified user at this boundary is therefore a social-provider signup.
    if (user.emailVerified !== true) return;
    return (await readRegistrationPolicyFailClosed(getRegistrationEnabled))
      ? undefined
      : false;
  };
}

async function readRegistrationPolicyFailClosed(
  getRegistrationEnabled: RegistrationPolicyReader,
): Promise<boolean> {
  try {
    return (await getRegistrationEnabled()) === true;
  } catch {
    return false;
  }
}
