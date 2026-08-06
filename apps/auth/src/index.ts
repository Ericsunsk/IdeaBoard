import dotenv from "dotenv";
import { createProductionAuthRuntime } from "./application/authRuntimeProduction.js";
import { loadEnv } from "./infrastructure/env.js";

dotenv.config({ quiet: true });

const environment = loadEnv(process.env);
const runtime = createProductionAuthRuntime(environment);
let shutdownPromise: Promise<void> | undefined;

process.once("SIGTERM", () => {
  void gracefulShutdown("SIGTERM");
});
process.once("SIGINT", () => {
  void gracefulShutdown("SIGINT");
});

try {
  await runtime.start();
  console.log(
    `[auth] Better Auth service listening on http://${environment.auth.host}:${environment.auth.port}`,
  );
} catch (error) {
  console.error("[auth] Failed to start auth service", error);
  process.exitCode = 1;
}

function gracefulShutdown(signal: string): Promise<void> {
  shutdownPromise ??= shutdown(signal);
  return shutdownPromise;
}

async function shutdown(signal: string): Promise<void> {
  console.log(`[auth] ${signal} received, shutting down gracefully...`);
  const forceExitTimer = setTimeout(() => {
    console.error("[auth] Graceful shutdown timed out, forcing exit");
    process.exit(1);
  }, 10_000);
  forceExitTimer.unref();

  try {
    await runtime.close();
    clearTimeout(forceExitTimer);
    console.log("[auth] Auth Runtime closed");
    process.exit(0);
  } catch (error) {
    clearTimeout(forceExitTimer);
    console.error("[auth] Error closing auth service", error);
    process.exit(1);
  }
}
