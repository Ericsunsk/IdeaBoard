import { betterAuth, type BetterAuthOptions } from "better-auth";
import type {
  RuntimeSocialProviderConfigs,
} from "./socialProviderSettings.js";
import { buildSocialProviders } from "./catalog.js";

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
          socialProviders: buildSocialProviders(configs),
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
