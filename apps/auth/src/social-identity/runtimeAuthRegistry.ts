import { betterAuth, type BetterAuthOptions } from "better-auth";
import type { RuntimeSocialProviderConfig } from "./socialProviderSettings.js";

export type BetterAuthInstance = ReturnType<typeof betterAuth>;
export type BetterAuthFactory = (options: BetterAuthOptions) => BetterAuthInstance;

export function createRuntimeAuthRegistry(
  factory: BetterAuthFactory,
  options: { baseOptions: BetterAuthOptions },
) {
  let current = factory(withRuntimeProvider(options.baseOptions, null));
  return {
    get current() {
      return current;
    },
    apply(config: RuntimeSocialProviderConfig | null) {
      current = factory(withRuntimeProvider(options.baseOptions, config));
    },
  };
}

function withRuntimeProvider(
  baseOptions: BetterAuthOptions,
  config: RuntimeSocialProviderConfig | null,
): BetterAuthOptions {
  const baseAccount = baseOptions.account;
  const baseLinking = baseAccount?.accountLinking;
  return {
    ...baseOptions,
    ...(config
      ? {
          socialProviders: {
            google: {
              clientId: config.clientId,
              clientSecret: config.clientSecret,
              accessType: "online",
              disableDefaultScope: true,
              disableImplicitSignUp: true,
              scope: [...config.scopes],
              mapProfileToUser: (profile: {
                email: string;
                email_verified: boolean;
                picture?: string;
              }) => ({
                email: profile.email_verified === true ? profile.email : null,
                image: profile.picture,
              }),
            },
          },
        }
      : { socialProviders: undefined }),
    account: {
      ...baseAccount,
      encryptOAuthTokens: true,
      accountLinking: {
        ...baseLinking,
        enabled: true,
        disableImplicitLinking: false,
        requireLocalEmailVerified: true,
        trustedProviders: config ? [config.provider] : [],
        allowDifferentEmails: false,
        updateUserInfoOnLink: false,
      },
    },
  };
}
