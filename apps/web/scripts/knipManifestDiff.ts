import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import ts from 'typescript'
import { fileURLToPath } from 'node:url'
import { loadSeamInventory } from './seamInventory.ts'
import {
  namedConsumer,
  symbolReaches,
  type KnipGovernanceManifest,
} from './knipGovernanceConfig.ts'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const manifestPath = path.join(webRoot, 'scripts', 'knipGovernanceManifest.json')

function collectFileExports(file: string): string[] {
  const source = ts.createSourceFile(
    file,
    fs.readFileSync(path.join(webRoot, file), 'utf8'),
    ts.ScriptTarget.Latest,
    true,
  )
  const names = new Set<string>()
  for (const statement of source.statements) {
    if (
      ts.isExportDeclaration(statement) &&
      statement.exportClause &&
      ts.isNamedExports(statement.exportClause)
    ) {
      for (const element of statement.exportClause.elements) names.add(element.name.text)
      continue
    }
    if (!ts.canHaveModifiers(statement)) continue
    const exported = ts
      .getModifiers(statement)
      ?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword)
    if (!exported) continue
    if (ts.isVariableStatement(statement)) {
      for (const declaration of statement.declarationList.declarations) {
        if (ts.isIdentifier(declaration.name)) names.add(declaration.name.text)
      }
    } else if (
      'name' in statement &&
      statement.name &&
      ts.isIdentifier(statement.name as ts.Node)
    ) {
      names.add((statement.name as ts.Identifier).text)
    }
  }
  return [...names].sort()
}

function collectGeneratedFiles(root: string): string[] {
  const files: string[] = []
  const walk = (directory: string): void => {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      const file = path.join(directory, entry.name)
      if (entry.isDirectory()) walk(file)
      else files.push(path.relative(root, file).split(path.sep).join('/'))
    }
  }
  walk(path.join(root, 'src/api/generated-query'))
  return files.sort()
}

function collectDiffs(manifest: KnipGovernanceManifest): string[] {
  const root = fs.realpathSync(webRoot)
  const diffs: string[] = []
  const seams = loadSeamInventory().seams
  const publicSymbols = new Set(
    seams.flatMap((seam) => seam.publicExports.map((name) => `${seam.path}#${name}`)),
  )
  for (const [file, symbols] of Object.entries(manifest.symbols)) {
    for (const [name, contract] of Object.entries(symbols)) {
      if (
        !contract.evidence.length ||
        !['canonical-public', 'canonical-origin', 'test-contract', 'generator-support'].includes(
          contract.kind,
        )
      ) {
        diffs.push(`invalid disposition: ${file}#${name}`)
        continue
      }
      if (contract.kind === 'canonical-public' && !publicSymbols.has(`${file}#${name}`)) {
        diffs.push(`stale public disposition: ${file}#${name}`)
      }
      if (contract.kind === 'canonical-origin') {
        for (const evidence of contract.evidence) {
          const split = evidence.lastIndexOf('#')
          if (
            !publicSymbols.has(evidence) ||
            !symbolReaches(root, evidence.slice(0, split), evidence.slice(split + 1), file, name)
          ) {
            diffs.push(`stale origin disposition: ${file}#${name} <- ${evidence}`)
          }
        }
      }
      if (contract.kind === 'test-contract') {
        for (const evidence of contract.evidence) {
          if (
            !fs.existsSync(path.resolve(root, evidence)) ||
            !namedConsumer(root, evidence, file, name)
          ) {
            diffs.push(`missing test evidence: ${file}#${name} <- ${evidence}`)
          }
        }
      }
    }
  }
  const files = collectGeneratedFiles(root)
  const actualSet = new Set(files)
  const manifestSet = new Set(Object.keys(manifest.generated).sort())
  for (const file of [...actualSet].filter((file) => !manifestSet.has(file))) {
    diffs.push(`generated file added: ${file}`)
  }
  for (const file of [...manifestSet].filter((file) => !actualSet.has(file))) {
    diffs.push(`generated file removed: ${file}`)
  }
  for (const file of [...actualSet].filter((file) => manifestSet.has(file))) {
    const bytes = fs.readFileSync(path.resolve(root, file))
    if (
      crypto.createHash('sha256').update(bytes).digest('hex') !== manifest.generated[file]?.sha256
    ) {
      diffs.push(`generated file changed: ${file}`)
    }
  }
  return diffs.sort()
}

const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')) as KnipGovernanceManifest
const diffs = collectDiffs(manifest)
if (process.argv.includes('--update')) {
  const root = fs.realpathSync(webRoot)
  const files = collectGeneratedFiles(root)
  const next: KnipGovernanceManifest['generated'] = {}
  for (const file of files) {
    const bytes = fs.readFileSync(path.resolve(root, file))
    next[file] = {
      sha256: crypto.createHash('sha256').update(bytes).digest('hex'),
      exports: collectFileExports(file),
    }
  }
  manifest.generated = next
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`)
  console.log(`manifest:update refreshed ${files.length} generated contract(s).`)
  const remaining = collectDiffs(manifest)
  if (remaining.length) {
    console.log('remaining diff after update:')
    for (const line of remaining) console.log(`  ${line}`)
    process.exit(1)
  }
  console.log('manifest clean after update.')
} else {
  if (!diffs.length) {
    console.log('manifest:diff clean.')
  } else {
    console.log('manifest:diff pending:')
    for (const line of diffs) console.log(`  ${line}`)
    process.exit(1)
  }
}
