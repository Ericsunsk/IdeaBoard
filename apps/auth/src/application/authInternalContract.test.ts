import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import Fastify from "fastify";
import type {
  AuthSessionAPI,
  AuthInternalCommands,
} from "../trusted-identity/authInternalCommands.js";
import { registerInternalRoutes } from "../trusted-identity/internalRoutes.js";

const internalKey = "contract-test-internal-key";
const authUserID = "auth-contract-user";
const maxProcessOutputBytes = 16 * 1024;

test(
  "production Go ServiceClient satisfies the live Fastify internal contract",
  { timeout: 75_000 },
  async (t) => {
    const app = Fastify();
    registerInternalRoutes(app, {
      internalKey,
      authApi: createContractAuthAPI(),
      commands: createContractCommands(),
    });

    const baseURL = await app.listen({ host: "127.0.0.1", port: 0 });
    t.after(async () => {
      await app.close();
    });

    await runGoContractTest(baseURL);
  },
);

function createContractAuthAPI(): AuthSessionAPI {
  return {
    getSession: async ({ headers }) => {
      assert.equal(
        headers.get("cookie"),
        "better-auth.session_token=contract-session",
      );
      return {
        user: {
          id: authUserID,
          name: "Alice",
          email: "alice@example.com",
          emailVerified: true,
          image: "https://lh3.googleusercontent.com/avatar",
        },
      };
    },
  };
}

function createContractCommands(): AuthInternalCommands {
  return {
    getUserEmails: async (input) => {
      assert.deepEqual(input, { userIDs: [authUserID] });
      return {
        ok: true,
        data: {
          users: [{ auth_user_id: authUserID, email: "alice@example.com" }],
        },
      };
    },
    ensureIdentity: async (input) => {
      assert.deepEqual(input, {
        email: "alice@example.com",
        displayName: "Alice",
        password: "secret1234",
        allowExisting: false,
        role: "admin",
      });
      return {
        ok: true,
        data: {
          user: {
            auth_user_id: authUserID,
            display_name: "Alice",
            email: "alice@example.com",
            claim_identity: "alice@example.com",
            claim_identity_kind: "canonical_email",
            claim_identity_verified: true,
            role: "admin",
            existing: false,
          },
        },
      };
    },
    deleteIdentity: async (input) => {
      if (input.authUserID === "missing-auth-user") {
        return { ok: false, kind: "not_found", message: "auth user not found" };
      }
      assert.equal(input.authUserID, authUserID);
      return { ok: true, data: { auth_user_id: authUserID, deleted: true } };
    },
    setRole: async (input) => {
      assert.deepEqual(input, { authUserID, role: "admin" });
      return { ok: true, data: { auth_user_id: authUserID, role: "admin" } };
    },
    banIdentity: async (input) => {
      assert.deepEqual(input, {
        authUserID,
        banReason: "policy violation",
        banExpiresIn: 3600,
      });
      return { ok: true, data: { auth_user_id: authUserID, banned: true } };
    },
    unbanIdentity: async (input) => {
      assert.deepEqual(input, { authUserID });
      return { ok: true, data: { auth_user_id: authUserID, banned: false } };
    },
    revokeSessions: async (input) => {
      assert.deepEqual(input, { authUserID });
      return { ok: true, data: { auth_user_id: authUserID, revoked: true } };
    },
  };
}

async function runGoContractTest(baseURL: string): Promise<void> {
  const apiRoot = fileURLToPath(new URL("../../../api/", import.meta.url));
  const child = spawn(
    "go",
    [
      "test",
      "./internal/identity/auth",
      "-run",
      "^TestServiceClientAgainstFastify$",
      "-count=1",
    ],
    {
      cwd: apiRoot,
      env: {
        ...process.env,
        AUTH_FASTIFY_CONTRACT_URL: baseURL,
        AUTH_FASTIFY_CONTRACT_INTERNAL_KEY: internalKey,
      },
      stdio: ["ignore", "pipe", "pipe"],
    },
  );

  let stdout = "";
  let stderr = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk: string) => {
    stdout = appendProcessOutput(stdout, chunk);
  });
  child.stderr.on("data", (chunk: string) => {
    stderr = appendProcessOutput(stderr, chunk);
  });

  await new Promise<void>((resolve, reject) => {
    let settled = false;
    let timedOut = false;
    let forceKillTimer: NodeJS.Timeout | undefined;
    const timeout = setTimeout(() => {
      timedOut = true;
      child.kill("SIGTERM");
      forceKillTimer = setTimeout(() => child.kill("SIGKILL"), 2_000);
      forceKillTimer.unref();
    }, 60_000);
    timeout.unref();

    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      if (forceKillTimer) clearTimeout(forceKillTimer);
      if (error) reject(error);
      else resolve();
    };

    child.once("error", (error) => {
      finish(new Error(`failed to start Go contract test: ${error.message}`));
    });
    child.once("close", (code, signal) => {
      if (timedOut) {
        finish(
          new Error(
            `Go contract test timed out after 60s${formatProcessOutput(stdout, stderr)}`,
          ),
        );
        return;
      }
      if (code !== 0) {
        finish(
          new Error(
            `Go contract test exited with code ${String(code)}${signal ? ` (${signal})` : ""}${formatProcessOutput(stdout, stderr)}`,
          ),
        );
        return;
      }
      finish();
    });
  });
}

function appendProcessOutput(current: string, chunk: string): string {
  return `${current}${chunk}`.slice(-maxProcessOutputBytes);
}

function formatProcessOutput(stdout: string, stderr: string): string {
  const sections = [];
  if (stderr.trim()) sections.push(`stderr:\n${stderr.trim()}`);
  if (stdout.trim()) sections.push(`stdout:\n${stdout.trim()}`);
  return sections.length > 0 ? `\n${sections.join("\n")}` : "";
}
