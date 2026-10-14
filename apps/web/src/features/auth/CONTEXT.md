# Auth Context

Auth owns the Better Auth session facts, login/register/OTP/OAuth/logout and impersonation exit, auth overlay chrome, and post-auth redirect intent.
It caches the server session as a 5-minute client snapshot via a single store.
It does NOT own workspace layout, account management UI, or per-page login modals.
Shape: flat feature (`components/` + `session/` + `pages/`, no `domain/`/`application/`/`infrastructure` split) — session transport plus panels need no anti-corruption layering.

## Domain Language

| Term           | Meaning                                                                                                                                                                                                                                                                               |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| User Session   | The frontend view of the authenticated user, backed by the Auth-owned server session; not a second identity source.                                                                                                                                                                   |
| Auth Transport | Sole Better Auth adapter (`session/transport.ts`): signIn/signUp/signOut/getSession plus `/api/auth/*` fetch.                                                                                                                                                                         |
| Session Cache  | In-memory session snapshot with 5-minute TTL (`session/freshness.ts` owns the trust window and freshness predicate; `browserSessionTransition.ensureSession` and the router admin gate both consume it); `ensureSession` returns cache when fresh, else `hasSession ? fetch : clear`. |
| Post-Auth Path | Post-login redirect intent; stored only in `sessionStorage` (`ideaboard:auth:post-auth-path`), consume-once, must pass internal-path validation.                                                                                                                                      |
| Auth Overlay   | Global login/register modal (`mode: login\|register\|null`); installed in `WorkspaceLayout` via `createAuthOverlay` / `provideAuthOverlay`, opened per surface.                                                                                                                       |
| OTP            | A short-lived email ownership proof used for registration or credential recovery; not an authenticated session by itself.                                                                                                                                                             |
| Impersonation  | Admin impersonation banner state: `impersonatedBy` from Better Auth `useSession()` read only in `WorkspaceLayout`; exit via `stopImpersonating`.                                                                                                                                      |

## Public Seams

- `useUserStore()` — session store singleton in `session`.
- `authTransport` — signIn/signUp/signOut/getSession adapter in `session/transport.ts`; sole owner of the Better Auth client instance.
- `UserAuthModals` — modal router mounted by route path or overlay mode in `components`.
- `Login` / `Register` panels — submit panels with local pending/locked and field validation in `components`.
- Auth fields kit — `EmailField` / `PasswordField` / `OtpField` plus `useAuthForm` (valibot-backed validators, required-sentinel wiring, submit pending) in `components/auth-fields`; re-exported by the barrel for cross-feature use (account `SecurityPane`).
- `AuthModalHost` — modal shell in `components`.
- `useAuthPanelControls` — shared panel controls and form utilities in `components`.
- `UserAuthCallbackPage` — sole route page for OAuth callback (`loading` / `failed`); lazy-loaded by `workspaceRoutes` in `pages`.
- `useAuthFeedback` — stateless login/register/stopImpersonating toast groups over `useAppToast` plus errorCatalog plus i18n.

## Invariants

- `/user/login` is the authentication entry for every account. Protected Admin destinations retain their safe redirect intent; the router enforces the Admin role after authentication.

- Panels and pages never call the generated client or Better Auth directly; chain is `Panel --runAsyncAction--> useUserStore --> authTransport --> authClient/fetch`.
- Identity changes (login, identity switch, logout, 401/403) must update refs, write/clear the `localStorage` snapshot, and clear all auth-scoped Query cache (`clearAuthScopedQueryCache`).
- `?redirect=` and post-auth path must pass `isSafeInternalRedirectPath` / `normalizeSafeInternalRedirectPath`; illegal values fall back to `USER_BOARDS_PATH`.
- Avatar is a server-persisted profile projection: upload writes the user record and `/user` reads it. Client snapshots are temporary; successful refetch replaces them, including clearing a missing avatar.
- `localStorage` holds only session snapshots and scope-restore keys; redirect intent lives only in `sessionStorage`; `File`/`blob` previews are never persisted.
- Session: `unauthenticated` to `restoring` (memory restore) to `authenticated` (login/verifyOTP plus `fetchUserInfo(force, { retryTransient })`) to `session-expired` (401/403 clears refs, storage, Query; network/5xx keeps stale refs and retries next nav); `ensureSession` returns directly when cache is under 5 minutes.
- OAuth callback: `loading` to `ensureSession(fresh)` plus `replace(redirect)`; provider error or session failure to `logout/clear` plus `replace` to param-free callback to `failed` (shows back-to-login button).
- Overlay: `closed` to `open(mode, postAuthPath?)` to `login|register` to `close` / `overlay-success` (consumes post-auth path and `router.push`); overlay close also clears pending search auth action.
- Panel submit: `idle` to submitting (`runAsyncAction.pending`, submit disabled, mask/esc cannot close) to `success` (emit) / `error` (toast); register adds `form` to `otpPending` to `success`.
- Email/password/OTP validation converges on the auth-fields kit over `shared/auth/passwordPolicy`; panels carry no hand-rolled regex and no English-only validation strings (copy resolves through locale keys at call time).
- Callback failure must clear the session and return to the param-free callback page; never leave provider error params behind.
- Copy keys converge on errorCatalog; no scattered copy.
- `localStorage['user']` goes only through `core/utils/auth.ts`.

## Non-goals

- No `useQuery`-cached session; no duplicated user refs.
- No direct `authClient` calls; no scattered auth fetch.
- No perpetual cache; no fetch on every render.
- No `localStorage` redirect; no open redirect.
- No per-page login modal; no route-only auth gate.
- No single-form auth; no silent OTP failure.
- No impersonation flags in the session store; no hidden impersonation.
