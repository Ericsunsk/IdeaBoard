import { readdirSync, readFileSync } from "node:fs";
import { dirname, extname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const authRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = resolve(authRoot, "src");
const contextDirectories = new Set([
  "admission",
  "social-identity",
  "transactional-mail",
  "trusted-identity",
]);
const allowedRootEntries = new Set([
  "admission",
  "application",
  "generated",
  "index.ts",
  "infrastructure",
  "migrate.ts",
  "social-identity",
  "transactional-mail",
  "trusted-identity",
]);
const violations = [];

for (const entry of readdirSync(sourceRoot, { withFileTypes: true })) {
  if (!allowedRootEntries.has(entry.name)) {
    violations.push(`src/${entry.name}: root entry is not allowlisted`);
  }
}

for (const sourcePath of listTypeScriptFiles(sourceRoot)) {
  const sourceRelativePath = toPosix(relative(sourceRoot, sourcePath));
  const sourceArea = sourceRelativePath.split("/")[0];
  const sourceFile = ts.createSourceFile(
    sourcePath,
    readFileSync(sourcePath, "utf8"),
    ts.ScriptTarget.Latest,
    true,
  );

  for (const specifier of moduleSpecifiers(sourceFile)) {
    if (!specifier.startsWith(".")) continue;
    const targetRelativePath = toPosix(
      relative(sourceRoot, resolve(dirname(sourcePath), specifier)),
    );
    const targetArea = targetRelativePath.split("/")[0];

    if (
      contextDirectories.has(sourceArea) &&
      (targetArea === "application" ||
        targetRelativePath === "index.js" ||
        targetRelativePath === "migrate.js")
    ) {
      violations.push(
        `${sourceRelativePath}: Context must not import composition module ${specifier}`,
      );
    }
    if (
      sourceArea === "infrastructure" &&
      (targetArea === "application" || contextDirectories.has(targetArea))
    ) {
      violations.push(
        `${sourceRelativePath}: Infrastructure must not import ${targetArea} module ${specifier}`,
      );
    }
  }
}

if (violations.length > 0) {
  console.error(`Auth architecture violations:\n${violations.join("\n")}`);
  process.exitCode = 1;
}

function listTypeScriptFiles(root) {
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(root, entry.name);
    if (entry.isDirectory()) return listTypeScriptFiles(path);
    return extname(entry.name) === ".ts" ? [path] : [];
  });
}

function moduleSpecifiers(sourceFile) {
  const specifiers = [];
  sourceFile.forEachChild((node) => {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      specifiers.push(node.moduleSpecifier.text);
    }
  });
  return specifiers;
}

function toPosix(value) {
  return value.split(sep).join("/");
}
