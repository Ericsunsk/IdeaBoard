import dotenv from "dotenv";
import { runAuthMigrations } from "./authMigration.js";

dotenv.config({ quiet: true });

runAuthMigrations(process.env).catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
});
