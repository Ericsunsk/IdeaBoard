import { betterAuth, type BetterAuthOptions } from "better-auth";
import type {
  RuntimeSocialProviderConfig,
  RuntimeSocialProviderConfigs,
  SupportedSocialProvider,
} from "./socialProviderSettings.js";

export type BetterAuthInstance = ReturnType<typeof betterAuth>;
export type BetterAuthFactory = (options: BetterAuthOptions) => BetterAuthInstance;

export function createRuntimeAuthRegistry(
  factory: BetterAuthFactory,
  options: { baseOptions: BetterAuthOptions },
) {
  let current = factory(withRuntimeProviders(options.baseOptions, {}));
  return {
    get current() {
      return current;
    },
    apply(configs: RuntimeSocialProviderConfigs) {
      current = factory(withRuntimeProviders(options.baseOptions, configs));
    },
  };
}

function withRuntimeProviders(
  baseOptions: BetterAuthOptions,
  configs: RuntimeSocialProviderConfigs,
): BetterAuthOptions {
  const baseAccount = baseOptions.account;
  const baseLinking = baseAccount?.accountLinking;
  return {
    ...baseOptions,
    ...(Object.keys(configs).length > 0
      ? {
          socialProviders: createSocialProviders(configs),
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
        trustedProviders: Object.keys(configs),
        allowDifferentEmails: false,
        updateUserInfoOnLink: false,
      },
    },
  };
}

function createSocialProviders(
  configs: RuntimeSocialProviderConfigs,
): NonNullable<BetterAuthOptions["socialProviders"]> {
  const socialProviders: Record<string, unknown> = {};
  for (const [provider, config] of Object.entries(configs) as Array<[
    SupportedSocialProvider,
    RuntimeSocialProviderConfig | undefined,
  ]>) {
    if (config) socialProviders[provider] = socialProviderBuilders[provider](config);
  }
  return socialProviders as NonNullable<BetterAuthOptions["socialProviders"]>;
}

const socialProviderBuilders: Record<
  SupportedSocialProvider,
  (config: RuntimeSocialProviderConfig) => Record<string, unknown>
> = {
  google: (config) => ({
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
      emailVerified: profile.email_verified === true,
      image: profile.picture,
    }),
  }),
  discord: (config) => ({
    clientId: config.clientId,
    clientSecret: config.clientSecret,
    disableDefaultScope: true,
    disableImplicitSignUp: true,
    scope: [...config.scopes],
    mapProfileToUser: (profile: {
      email?: string | null;
      verified?: boolean;
      image_url?: string;
      global_name?: string | null;
      username?: string;
    }) => ({
      email: profile.verified === true ? profile.email ?? null : null,
      emailVerified: profile.verified === true,
      image: profile.image_url,
      name: profile.global_name || profile.username,
    }),
  }),
};
