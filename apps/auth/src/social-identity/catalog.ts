import type { BetterAuthOptions } from "better-auth";

export type SupportedSocialProvider = "google" | "discord";

export type RuntimeSocialProviderConfig = {
  provider: SupportedSocialProvider;
  clientId: string;
  clientSecret: string;
  scopes: string[];
};

export type RuntimeSocialProviderConfigs = Partial<
  Record<SupportedSocialProvider, RuntimeSocialProviderConfig>
>;

export type SocialProviderCapability = {
  provider: SupportedSocialProvider;
  label: string;
  available: boolean;
};

type SocialProviderDefinition = {
  code: SupportedSocialProvider;
  label: string;
  scopes: readonly string[];
  validateClientId(value: string): boolean;
  buildRuntimeOptions(
    config: RuntimeSocialProviderConfig,
  ): Record<string, unknown>;
};

const catalog: Record<SupportedSocialProvider, SocialProviderDefinition> = {
  google: {
    code: "google",
    label: "Google",
    scopes: ["openid", "email", "profile"],
    validateClientId(value) {
      return (
        value.length <= 512 &&
        /^[A-Za-z0-9._-]+\.apps\.googleusercontent\.com$/.test(value)
      );
    },
    buildRuntimeOptions(config) {
      return {
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
      };
    },
  },
  discord: {
    code: "discord",
    label: "Discord",
    scopes: ["identify", "email"],
    validateClientId(value) {
      return value.length <= 32 && /^\d{1,32}$/.test(value);
    },
    buildRuntimeOptions(config) {
      return {
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
      };
    },
  },
};

export const SOCIAL_PROVIDER_CATALOG: Readonly<
  Record<SupportedSocialProvider, SocialProviderDefinition>
> = catalog;

export const supportedSocialProviders: readonly SupportedSocialProvider[] =
  Object.keys(catalog) as SupportedSocialProvider[];

export function isSupportedSocialProvider(
  value: string,
): value is SupportedSocialProvider {
  return Object.prototype.hasOwnProperty.call(catalog, value);
}

export function requireSocialProvider(value: string): SupportedSocialProvider {
  const provider = value.trim().toLowerCase();
  if (!isSupportedSocialProvider(provider)) {
    throw new Error("unsupported social provider");
  }
  return provider;
}

export function socialProviderDefinition(
  provider: SupportedSocialProvider,
): SocialProviderDefinition {
  return catalog[provider];
}

export function socialProviderLabel(provider: SupportedSocialProvider): string {
  return catalog[provider].label;
}

export function socialProviderScopes(
  provider: SupportedSocialProvider,
): string[] {
  return [...catalog[provider].scopes];
}

export function socialProviderCapabilities(
  availability: Partial<Record<SupportedSocialProvider, boolean>>,
): SocialProviderCapability[] {
  return supportedSocialProviders.map((provider) => ({
    provider,
    label: catalog[provider].label,
    available: availability[provider] === true,
  }));
}

export function buildSocialProviders(
  configs: RuntimeSocialProviderConfigs,
): NonNullable<BetterAuthOptions["socialProviders"]> {
  const socialProviders: Record<string, unknown> = {};
  for (const provider of supportedSocialProviders) {
    const config = configs[provider];
    if (config) {
      socialProviders[provider] = catalog[provider].buildRuntimeOptions(config);
    }
  }
  return socialProviders as NonNullable<BetterAuthOptions["socialProviders"]>;
}
