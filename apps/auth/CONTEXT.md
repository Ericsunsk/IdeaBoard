# Auth Context

Auth owns login, registration, session, identity-side fields, credentials, transactional mail, and social-provider settings for the Better Auth Fastify service (`apps/auth`, port 6001). It does NOT own business user profiles, roles/status adjudication, Admin orchestration, or business permissions — those belong to the Go API behind `AUTH_INTERNAL_KEY`.

## Domain Language

| Term               | Meaning                                                                                                                                                                                                                                                                                                                                 |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AuthEnvironment    | Sole typed env contract from `infrastructure/env.ts` `loadEnv(source)`; both entrypoints (`src/index.ts` runtime, `src/migrate.ts` migrations) load it and pass it down. Never read `process.env` past the entrypoint.                                                                                                                  |
| Admission          | `src/admission/`: request-shape decisions before any handler runs — origin/CORS allowlist (`authAdmissionPolicy`), registration/OTP/mail/provider readiness gates, per-route internal-key guard (`internalKeyGuard`), social OAuth request policy. Pure decisions, no I/O except injected readers.                                      |
| Infrastructure     | `src/infrastructure/`: I/O and platform adapters — typed env, origin normalization, `{code,msg}` responders, Better Auth runtime lifecycle, repositories, migration runner, privilege verification, secret cipher.                                                                                                                      |
| Trusted Identity   | `src/trusted-identity/`: service-to-service seam the Go API calls with `x-internal-key` — session validation, user ensure/delete/role/ban/session-revoke commands. Machine-facing; wire `msg` values stay English lowercase tokens.                                                                                                     |
| Social Identity    | `src/social-identity/`: OAuth provider settings (stored encrypted, applied to the Better Auth runtime) plus the public capabilities route. Domain errors are user-facing Chinese; the untrusted-origin denial matches that rule.                                                                                                        |
| Transactional Mail | `src/transactional-mail/`: mail provider config (encrypted API key, optimistic-concurrency updates, readiness state machine `not_configured \| not_tested \| ready \| unavailable`) and the template renderer. Branding origin is an explicit `publicSiteOrigin` parameter threaded from `environment.auth.baseURL`, never ambient env. |
| Internal Routes    | `src/generated/auth-internal/`: generated Fastify route/type bindings for the Go↔Auth contract; never hand-edit. `contract:generate` / `contract:check` keep them in sync.                                                                                                                                                              |

## Environment Contract

| Variable                                                                     | Required                                          | Notes                                                                                                                                                                                             |
| ---------------------------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AUTH_INTERNAL_KEY`                                                          | always (≥32 chars, non-placeholder in production) | Shared secret with the Go API; both `x-internal-key` guards and the login-metadata reporter use it.                                                                                               |
| `BETTER_AUTH_SECRET`                                                         | production (≥32 chars, non-placeholder)           | Better Auth session signing; dev-only fallback exists for non-production.                                                                                                                         |
| `PUBLIC_ORIGIN`                                                              | production (valid https URL)                      | Sole public-origin input; `loadEnv` derives `auth.baseURL` and `trustedOrigins` from it.                                                                                                          |
| `API_INTERNAL_URL`                                                           | optional (default `http://127.0.0.1:6000`)        | Container default `http://api:6000`; only the login-metadata reporter calls it. Migrate images omit it.                                                                                           |
| `AUTH_HOST` / `AUTH_PORT` / `AUTH_TRUST_PROXY`                               | optional                                          | Listen surface; production listens on loopback behind the reverse proxy. `AUTH_TRUST_PROXY` defaults to `loopback`.                                                                               |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE` | runtime                                           | Runtime uses the least-privilege role; the migrate entrypoint additionally requires `DB_MIGRATION_USER` ≠ `DB_USER` plus `DB_MIGRATION_PASSWORD`.                                                 |
| `APP_SECRETS_ENCRYPTION_KEY`                                                 | always                                            | Base64 of exactly 32 bytes; encrypts mail API keys and OAuth client secrets.                                                                                                                      |
| `ENV_FILE`                                                                   | optional                                          | Explicit dotenv path loaded before the default `.env` in both entrypoints (mirrors `apps/api` `config.go` `loadEnvFile` precedence); real process env always wins because dotenv never overrides. |
| `NODE_ENV`                                                                   | —                                                 | `production` selects production validation; never gate business logic on it outside `runtimeEnv`.                                                                                                 |

## Public Seams

- `src/index.ts` runtime entrypoint; `src/migrate.ts` migration entrypoint (`migrate` / `migrate:dist`).
- `application/auth.ts` `createAuthComponents` / `createBetterAuthOptions` — composition root for Better Auth options, admission policy, mail/social services.
- `application/authApp.ts` `createAuthApp` — Fastify assembly: auth-entry guard, health, internal routes, mail/social routes.
- `admission/internalKeyGuard.ts` `createInternalKeyPreHandler` / `matchesInternalKey` — shared timing-safe service-to-service guard.
- `infrastructure/httpResponse.ts` `sendSuccess` / `sendError` — sole `{code,msg}` envelope writers.
- `transactional-mail/emailTemplates/index.ts` typed renderers (`verificationOtp`, `passwordResetOtp`, `welcomeOnboarding`, `paymentReceipt`, `teamInvite`, `securityAlertLogin`, `mailConfigurationTest`).

## Invariants

- Errors render through the single envelope: 2xx JSON is `{code: 0, msg: "success", data}`, errors are `{code: <status>, msg}`. Callers branch on `code`/status, never on message text.
- Wire `msg` language is split by audience and documented here: internal/machine surfaces (`trusted-identity` commands, `x-internal-key` guards, `/api/health`, error-handler fallbacks) use English lowercase tokens; user-facing surfaces (admission denials, mail/social domain errors, capabilities guard) use Chinese. Do not mix languages within one route file.
- The template renderer takes no ambient env: every `publicSiteOrigin` arrives as an explicit parameter (production value is `environment.auth.baseURL` via `createAuthComponents` and the mail/social adapters).
- `admission/` never performs I/O except through injected readers (`getSettings`, `isMailReady`, provider readiness); all policy inputs arrive as plain data.
- High-risk identity actions (role changes, user deletion) execute only via `trusted-identity` commands invoked by the Go Admin API, never from browser-facing Better Auth admin calls.
- Social Client ID rotation requires disabling the provider first (`disable_before_client_id_change`); enabling requires both Client ID and Secret.
- Mail sends require `readiness === "ready"` (a successful `sendTest` after every config change); test delivery and readiness persistence fail atomically.
- Runtime and migrate images use distinct DB roles; startup verifies runtime privileges fail-closed.
- Generated code (`src/generated/`) is a boundary, never hand-edited business logic.

## Test Idiom

- Runner: `npm test` = `contract:check` + `architecture:check` + `tsx --test "src/**/*.test.ts"`.
- Style: `node:test` `describe/it` with `node:assert/strict`; colocated `*.test.ts` next to the module under test.
- Env in tests: pass explicit `EnvSource` objects to `loadEnv`; never rely on ambient `process.env` (the email-template suite asserts the renderer ignores it).
- Architecture: `scripts/check-architecture.mjs` enforces the `admission/` vs `infrastructure/` split and the generated-code boundary; consult its allowlist before adding cross-layer imports.

## Non-goals

- No business user profile, role/status, or Admin orchestration logic; no direct Better Auth admin calls from frontend code paths.
- No second env contract: no new `process.env` reads outside the two entrypoints and `loadEnv`.
- No user-visible `Credit` copy in mail templates; user-facing mail copy uses `Idea/Ideas` consistent with the frontend account context.
