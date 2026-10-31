import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import ts from 'typescript'
import postcss from 'postcss'
import { loadSeamInventory } from './seamInventory.ts'
import { vueAutoImportCompiler } from '../node_modules/knip/dist/plugins/_vue/auto-import.js'

export type KnipGovernanceManifest = {
  version: number
  symbols: Record<string, Record<string, { kind: string; evidence: string[] }>>
  generated: Record<string, { sha256: string; exports: string[] }>
  files: Record<string, { kind: string; evidence: string[] }>
}

const REQUIRED_KNIP_VERSION = '6.32.3'

/** Follow CSS imports and scoped package asset URLs in Knip's analysis input. */
function cssDependencyCompiler(source: string): string {
  const imports = new Set<string>()
  const stylesheet = postcss.parse(source)
  stylesheet.walkAtRules('import', (rule) => {
    const specifier = rule.params.match(/^(?:url\(\s*)?(['"])(.*?)\1/)?.[2]
    if (specifier && !/^(?:[a-z][\w+.-]*:|\/\/|\/|#)/i.test(specifier)) imports.add(specifier)
  })
  stylesheet.walkDecls((declaration) => {
    for (const match of declaration.value.matchAll(
      /"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|url\(\s*(?:"([^"]*)"|'([^']*)'|([^\s)]+))\s*\)/gi,
    )) {
      const specifier = match[1] ?? match[2] ?? match[3]
      if (specifier && /^@[^/\s]+\/[^/\s]+\//.test(specifier))
        imports.add(specifier.split(/[?#]/)[0]!)
    }
  })
  return [...imports].map((specifier) => `import ${JSON.stringify(specifier)};`).join('\n')
}

/** Annotation exists only in Knip's analysis input; source bytes remain untouched. */
function annotateKnipPublic(
  source: string,
  file: string,
  manifest: KnipGovernanceManifest,
  root: string,
): string {
  const relative = path.relative(root, file).split(path.sep).join('/')
  const generated = manifest.generated[relative]
  const approved = generated
    ? Object.fromEntries(
        generated.exports.map((name) => [name, { evidence: ['frozen Orval capability contract'] }]),
      )
    : manifest.symbols[relative]
  if (!approved) return source
  const parsed = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true)
  const positions: number[] = []
  const replacements: Array<{ start: number; end: number; text: string }> = []
  for (const statement of parsed.statements) {
    if (
      ts.isExportAssignment(statement) &&
      !statement.isExportEquals &&
      approved.default?.evidence.length
    ) {
      positions.push(statement.getStart(parsed))
      continue
    }
    if (
      ts.isExportDeclaration(statement) &&
      statement.exportClause &&
      ts.isNamedExports(statement.exportClause)
    ) {
      const names = statement.exportClause.elements.map((element) => element.name.text)
      if (names.length && names.every((name) => approved[name]?.evidence.length))
        positions.push(statement.getStart(parsed))
      else if (names.some((name) => approved[name]?.evidence.length)) {
        const suffix = statement.moduleSpecifier
          ? ` from ${statement.moduleSpecifier.getText(parsed)}`
          : ''
        const text = statement.exportClause.elements
          .map(
            (element) =>
              `${approved[element.name.text]?.evidence.length ? '/** @public */\n' : ''}export ${statement.isTypeOnly ? 'type ' : ''}{ ${element.getText(parsed)} }${suffix};`,
          )
          .join('\n')
        replacements.push({ start: statement.getStart(parsed), end: statement.end, text })
      }
      continue
    }
    if (
      !ts.canHaveModifiers(statement) ||
      !ts.getModifiers(statement)?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword)
    )
      continue
    let names: string[] = []
    if (ts.isVariableStatement(statement))
      names = statement.declarationList.declarations.map((declaration) =>
        ts.isIdentifier(declaration.name) ? declaration.name.text : '',
      )
    else if ('name' in statement && statement.name && ts.isIdentifier(statement.name as ts.Node))
      names = [(statement.name as ts.Identifier).text]
    if (names.length && names.every((name) => approved[name]?.evidence.length))
      positions.push(statement.getStart(parsed))
  }
  replacements.push(
    ...positions.map((position) => ({ start: position, end: position, text: '/** @public */\n' })),
  )
  for (const replacement of replacements.sort((a, b) => b.start - a.start))
    source = `${source.slice(0, replacement.start)}${replacement.text}${source.slice(replacement.end)}`
  return source
}

function annotateVue(
  source: string,
  file: string,
  manifest: KnipGovernanceManifest,
  root: string,
): string {
  return annotateKnipPublic(vueAutoImportCompiler(source, file), file, manifest, root)
}

function resolveLocalModule(root: string, file: string, module: string): string | undefined {
  const absolute = module.startsWith('@/')
    ? path.join(root, 'src', module.slice(2))
    : path.resolve(root, path.dirname(file), module)
  return ['', '.ts', '.vue', '/index.ts']
    .map((suffix) => absolute + suffix)
    .find((candidate) => fs.existsSync(candidate) && fs.statSync(candidate).isFile())
}

function symbolReaches(
  root: string,
  file: string,
  symbol: string,
  target: string,
  name: string,
  seen = new Set<string>(),
): boolean {
  if (file === target && symbol === name) return true
  const key = `${file}#${symbol}`
  if (seen.has(key)) return false
  seen.add(key)
  const source = ts.createSourceFile(
    file,
    fs.readFileSync(path.join(root, file), 'utf8'),
    ts.ScriptTarget.Latest,
    true,
  )
  return source.statements.some((statement) => {
    if (
      !ts.isExportDeclaration(statement) ||
      !statement.moduleSpecifier ||
      !ts.isStringLiteral(statement.moduleSpecifier)
    )
      return false
    const resolved = resolveLocalModule(root, file, statement.moduleSpecifier.text)
    if (!resolved) return false
    const relative = path.relative(root, resolved).split(path.sep).join('/')
    if (statement.exportClause && ts.isNamedExports(statement.exportClause))
      return statement.exportClause.elements.some(
        (element) =>
          element.name.text === symbol &&
          symbolReaches(
            root,
            relative,
            (element.propertyName ?? element.name).text,
            target,
            name,
            seen,
          ),
      )
    return !statement.exportClause && symbolReaches(root, relative, symbol, target, name, seen)
  })
}

function namedConsumer(root: string, consumer: string, target: string, name: string): boolean {
  let text = fs.readFileSync(path.join(root, consumer), 'utf8')
  if (consumer.endsWith('.vue'))
    text = [...text.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)]
      .map((match) => match[1])
      .join('\n')
  const source = ts.createSourceFile(consumer, text, ts.ScriptTarget.Latest, true)
  const namespaces = new Map<string, string>()
  let dynamicNamedMatch = false
  function collect(node: ts.Node): void {
    if (
      ts.isImportDeclaration(node) &&
      node.importClause?.namedBindings &&
      ts.isNamespaceImport(node.importClause.namedBindings) &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      const resolved = resolveLocalModule(root, consumer, node.moduleSpecifier.text)
      if (resolved)
        namespaces.set(
          node.importClause.namedBindings.name.text,
          path.relative(root, resolved).split(path.sep).join('/'),
        )
    }
    if (ts.isVariableDeclaration(node) && node.initializer) {
      const value = ts.isAwaitExpression(node.initializer)
        ? node.initializer.expression
        : node.initializer
      if (
        ts.isCallExpression(value) &&
        value.expression.kind === ts.SyntaxKind.ImportKeyword &&
        value.arguments[0] &&
        ts.isStringLiteral(value.arguments[0])
      ) {
        const resolved = resolveLocalModule(root, consumer, value.arguments[0].text)
        if (resolved && ts.isIdentifier(node.name))
          namespaces.set(node.name.text, path.relative(root, resolved).split(path.sep).join('/'))
        if (resolved && ts.isObjectBindingPattern(node.name))
          for (const element of node.name.elements) {
            if (
              ts.isIdentifier(element.name) &&
              symbolReaches(
                root,
                path.relative(root, resolved).split(path.sep).join('/'),
                element.propertyName?.getText(source) ?? element.name.text,
                target,
                name,
              )
            )
              dynamicNamedMatch = true
          }
      }
    }
    ts.forEachChild(node, collect)
  }
  collect(source)
  let namespaceMatch = false
  function inspect(node: ts.Node): void {
    if (ts.isPropertyAccessExpression(node) && ts.isIdentifier(node.expression)) {
      const file = namespaces.get(node.expression.text)
      if (file && symbolReaches(root, file, node.name.text, target, name)) namespaceMatch = true
    }
    ts.forEachChild(node, inspect)
  }
  inspect(source)
  return (
    namespaceMatch ||
    dynamicNamedMatch ||
    source.statements.some((statement) => {
      if (!ts.isImportDeclaration(statement) || !ts.isStringLiteral(statement.moduleSpecifier))
        return false
      const resolved = resolveLocalModule(root, consumer, statement.moduleSpecifier.text)
      if (!resolved) return false
      const file = path.relative(root, resolved).split(path.sep).join('/')
      const clause = statement.importClause
      if (clause?.name && symbolReaches(root, file, 'default', target, name)) return true
      return (
        clause?.namedBindings &&
        ts.isNamedImports(clause.namedBindings) &&
        clause.namedBindings.elements.some((element) =>
          symbolReaches(root, file, (element.propertyName ?? element.name).text, target, name),
        )
      )
    }) === true
  )
}

export function createGovernanceConfig(
  configFile: string,
  root = process.cwd(),
  suppliedManifest?: KnipGovernanceManifest,
) {
  root = fs.realpathSync(root)
  const installedKnip = JSON.parse(
    fs.readFileSync(new URL('../node_modules/knip/package.json', import.meta.url), 'utf8'),
  ) as { version?: string }
  if (installedKnip.version !== REQUIRED_KNIP_VERSION)
    throw new Error(
      `Knip governance adapter requires ${REQUIRED_KNIP_VERSION}; found ${installedKnip.version ?? 'unknown'}.`,
    )
  const parsed = ts.parseConfigFileTextToJson(
    configFile,
    fs.readFileSync(path.resolve(root, configFile), 'utf8'),
  )
  if (parsed.error) throw new Error(ts.flattenDiagnosticMessageText(parsed.error.messageText, '\n'))
  const manifest: KnipGovernanceManifest =
    suppliedManifest ??
    JSON.parse(fs.readFileSync(new URL('./knipGovernanceManifest.json', import.meta.url), 'utf8'))
  let activeSeams: ReturnType<typeof loadSeamInventory>['seams'] | undefined
  const getActiveSeams = () => (activeSeams ??= loadSeamInventory().seams)
  if (
    !suppliedManifest ||
    Object.keys(manifest.symbols).length > 0 ||
    Object.keys(manifest.files).length > 0
  ) {
    const publicSymbols = new Set(
      getActiveSeams().flatMap((seam) => seam.publicExports.map((name) => `${seam.path}#${name}`)),
    )
    for (const [file, symbols] of Object.entries(manifest.symbols)) {
      for (const [name, contract] of Object.entries(symbols)) {
        if (
          !contract.evidence.length ||
          !['canonical-public', 'canonical-origin', 'test-contract', 'generator-support'].includes(
            contract.kind,
          )
        )
          throw new Error(`Invalid Knip symbol disposition: ${file}#${name}`)
        if (contract.kind === 'canonical-public' && !publicSymbols.has(`${file}#${name}`))
          throw new Error(`Stale public Knip disposition: ${file}#${name}`)
        if (
          contract.kind === 'canonical-origin' &&
          contract.evidence.some((evidence) => {
            const split = evidence.lastIndexOf('#')
            return (
              !publicSymbols.has(evidence) ||
              !symbolReaches(root, evidence.slice(0, split), evidence.slice(split + 1), file, name)
            )
          })
        )
          throw new Error(`Stale public origin disposition: ${file}#${name}`)
        if (
          contract.kind === 'test-contract' &&
          contract.evidence.some(
            (evidence) =>
              !fs.existsSync(path.resolve(root, evidence)) ||
              !namedConsumer(root, evidence, file, name),
          )
        )
          throw new Error(`Missing test Knip evidence: ${file}#${name}`)
        if (contract.kind === 'generator-support') {
          const allowed: Record<string, string[]> = {
            'src/api/http/orvalMutator.ts': ['orvalMutator', 'ErrorType', 'BodyType'],
            'src/api/http/publicMutator.ts': ['publicMutator'],
            'src/api/http/controlMutator.ts': ['controlMutator'],
          }
          if (!allowed[file]?.includes(name))
            throw new Error(`Unknown generator support contract: ${file}#${name}`)
          if (file === 'src/api/http/orvalMutator.ts') {
            if (
              !namedConsumer(root, 'src/api/http/publicMutator.ts', file, 'orvalMutator') ||
              !namedConsumer(root, 'src/api/http/controlMutator.ts', file, 'orvalMutator')
            )
              throw new Error(
                'Orval transport protocol no longer has its configured wrapper consumers.',
              )
            const source = ts.createSourceFile(
              file,
              fs.readFileSync(path.resolve(root, file), 'utf8'),
              ts.ScriptTarget.Latest,
              true,
            )
            if (
              !source.statements.some(
                (statement) =>
                  ts.canHaveModifiers(statement) &&
                  ts
                    .getModifiers(statement)
                    ?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) &&
                  (('name' in statement &&
                    (statement.name as ts.Identifier | undefined)?.text === name) ||
                    (ts.isVariableStatement(statement) &&
                      statement.declarationList.declarations.some(
                        (declaration) =>
                          ts.isIdentifier(declaration.name) && declaration.name.text === name,
                      ))),
              )
            )
              throw new Error(`Orval generator protocol declaration missing: ${name}`)
          }
        }
      }
    }
    const generatedRoot = path.join(root, 'src/api/generated-query')
    const files: string[] = []
    function walk(directory: string): void {
      for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
        const file = path.join(directory, entry.name)
        if (entry.isDirectory()) walk(file)
        else files.push(path.relative(root, file).split(path.sep).join('/'))
      }
    }
    if (!suppliedManifest) walk(generatedRoot)
    if (
      !suppliedManifest &&
      JSON.stringify(files.sort()) !== JSON.stringify(Object.keys(manifest.generated).sort())
    )
      throw new Error('Generated Knip contract file set changed.')
  }
  // Frozen generated contracts fail before exemptions can conceal changed output.
  for (const [file, contract] of Object.entries(manifest.generated)) {
    const bytes = fs.readFileSync(path.resolve(root, file))
    if (crypto.createHash('sha256').update(bytes).digest('hex') !== contract.sha256)
      throw new Error(
        `Generated Knip contract changed: ${file}; regenerate and review its exact exports.`,
      )
  }
  return {
    ...parsed.config,
    ignoreIssues: {
      ...parsed.config.ignoreIssues,
      ...Object.fromEntries(
        Object.entries(manifest.files).map(([file, contract]) => {
          if (!contract.evidence.length) throw new Error(`Missing retained-file evidence: ${file}`)
          {
            const canonical = getActiveSeams().some((seam) => seam.path === file)
            const imported = contract.evidence.some((consumer) => {
              if (!fs.existsSync(path.resolve(root, consumer))) return false
              const text = fs.readFileSync(path.resolve(root, consumer), 'utf8')
              const source = ts.createSourceFile(consumer, text, ts.ScriptTarget.Latest, true)
              let found = false
              function visit(node: ts.Node): void {
                if (
                  (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
                  node.moduleSpecifier &&
                  ts.isStringLiteral(node.moduleSpecifier)
                ) {
                  const target = resolveLocalModule(root, consumer, node.moduleSpecifier.text)
                  if (target === path.resolve(root, file)) found = true
                }
                if (
                  ts.isCallExpression(node) &&
                  node.expression.kind === ts.SyntaxKind.ImportKeyword &&
                  node.arguments[0] &&
                  ts.isStringLiteral(node.arguments[0])
                ) {
                  const target = resolveLocalModule(root, consumer, node.arguments[0].text)
                  if (target === path.resolve(root, file)) found = true
                }
                ts.forEachChild(node, visit)
              }
              visit(source)
              return found
            })
            const setupConfig =
              file === 'src/test/browserSetup.ts'
                ? 'vitest.browser.config.ts'
                : file === 'src/test/setup.ts'
                  ? 'vite.config.ts'
                  : undefined
            const setup = setupConfig
              ? ts
                  .createSourceFile(
                    setupConfig,
                    fs.readFileSync(path.resolve(root, setupConfig), 'utf8'),
                    ts.ScriptTarget.Latest,
                    true,
                  )
                  .statements.some((statement) => {
                    let found = false
                    function visit(node: ts.Node): void {
                      if (
                        ts.isPropertyAssignment(node) &&
                        node.name.getText() === 'setupFiles' &&
                        ts.isArrayLiteralExpression(node.initializer)
                      )
                        found = node.initializer.elements.some(
                          (element) => ts.isStringLiteral(element) && element.text === file,
                        )
                      ts.forEachChild(node, visit)
                    }
                    visit(statement)
                    return found
                  })
              : false
            const cssContract =
              contract.kind === 'motion-style-contract' &&
              (file === 'src/core/motion/dialog-motion.css' ||
                file === 'src/core/motion/dialog-motion.css.d.ts') &&
              ts
                .createSourceFile(
                  'dialogMotion.ts',
                  fs.readFileSync(path.resolve(root, 'src/core/motion/dialogMotion.ts'), 'utf8'),
                  ts.ScriptTarget.Latest,
                  true,
                )
                .statements.some(
                  (statement) =>
                    ts.isImportDeclaration(statement) &&
                    ts.isStringLiteral(statement.moduleSpecifier) &&
                    statement.moduleSpecifier.text === './dialog-motion.css',
                )
            if (!manifest.generated[file] && !canonical && !imported && !setup && !cssContract)
              throw new Error(`Stale retained-file evidence: ${file}`)
          }
          return [file, ['files']]
        }),
      ),
    },
    compilers: {
      ...parsed.config.compilers,
      css: cssDependencyCompiler,
      ts: (source: string, file: string) => annotateKnipPublic(source, file, manifest, root),
      '.vue': (source: string, file: string) => annotateVue(source, file, manifest, root),
    },
  }
}
