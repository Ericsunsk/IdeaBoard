// This file is auto-generated from contracts/auth-internal.openapi.json. Do not edit.

export const authInternalRoutes = {
  "validateSession": {
    method: "GET",
    openapiPath: "/internal/session",
    fastifyPath: "/internal/session",
    responseStatuses: [200,401,500],
  },
  "ensureUser": {
    method: "POST",
    openapiPath: "/internal/users/ensure",
    fastifyPath: "/internal/users/ensure",
    responseStatuses: [200,400,401,404,409,500],
  },
  "getUserEmails": {
    method: "POST",
    openapiPath: "/internal/users/emails",
    fastifyPath: "/internal/users/emails",
    responseStatuses: [200,400,401,500],
  },
  "deleteUser": {
    method: "DELETE",
    openapiPath: "/internal/users/{id}",
    fastifyPath: "/internal/users/:id",
    responseStatuses: [200,400,401,404,409,500],
  },
  "setUserRole": {
    method: "PUT",
    openapiPath: "/internal/users/{id}/role",
    fastifyPath: "/internal/users/:id/role",
    responseStatuses: [200,400,401,404,409,500],
  },
  "banUser": {
    method: "POST",
    openapiPath: "/internal/users/ban",
    fastifyPath: "/internal/users/ban",
    responseStatuses: [200,400,401,404,409,500],
  },
  "unbanUser": {
    method: "POST",
    openapiPath: "/internal/users/unban",
    fastifyPath: "/internal/users/unban",
    responseStatuses: [200,400,401,404,409,500],
  },
  "revokeUserSessions": {
    method: "POST",
    openapiPath: "/internal/users/revoke-sessions",
    fastifyPath: "/internal/users/revoke-sessions",
    responseStatuses: [200,400,401,404,409,500],
  },
  "getMailConfig": {
    method: "GET",
    openapiPath: "/internal/mail/config",
    fastifyPath: "/internal/mail/config",
    responseStatuses: [200,401,500],
  },
  "updateMailConfig": {
    method: "PUT",
    openapiPath: "/internal/mail/config",
    fastifyPath: "/internal/mail/config",
    responseStatuses: [200,400,401,409,500,503],
  },
  "clearMailConfig": {
    method: "DELETE",
    openapiPath: "/internal/mail/config",
    fastifyPath: "/internal/mail/config",
    responseStatuses: [200,400,401,409,500],
  },
  "sendMailConfigTest": {
    method: "POST",
    openapiPath: "/internal/mail/config/test",
    fastifyPath: "/internal/mail/config/test",
    responseStatuses: [200,400,401,409,500,503],
  },
  "getSocialProviderCapabilities": {
    method: "GET",
    openapiPath: "/internal/social-providers/capabilities",
    fastifyPath: "/internal/social-providers/capabilities",
    responseStatuses: [200,401,500,503],
  },
  "getSocialProvider": {
    method: "GET",
    openapiPath: "/internal/social-providers/{provider}",
    fastifyPath: "/internal/social-providers/:provider",
    responseStatuses: [200,401,404,500,503],
  },
  "updateSocialProvider": {
    method: "PUT",
    openapiPath: "/internal/social-providers/{provider}",
    fastifyPath: "/internal/social-providers/:provider",
    responseStatuses: [200,400,401,404,409,500,503],
  },
} as const

export type AuthInternalOperationID = keyof typeof authInternalRoutes
