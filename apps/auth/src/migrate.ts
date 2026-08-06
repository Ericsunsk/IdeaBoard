import dotenv from "dotenv";
import { runProductionAuthMigrations } from "./application/authMigrationProduction.js";

dotenv.config({ quiet: true });

runProductionAuthMigrations(process.env).catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
});
