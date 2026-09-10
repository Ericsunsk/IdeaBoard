import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

type SourceValue<T> = {
  label: string
  file: string
  value: T
}

type SourceValues<T> = {
  label: string
  file: string
  values: readonly T[]
}

type BooleanCheck = {
  label: string
  file: string
  ok: boolean
  detail: string
}

type Violation = {
  check: string
  message: string
  details?: readonly string[]
}

type ContractSources = {
  frontendDomain: string
  frontendSchema: string
  backendState: string
  openApi: string
  generatedEnvelope: string
  generatedSchemaVersion: string
  generatedGroup: string
}

type GeneratedSchemaVersionEntry = {
  key: number
  value: number
}

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const repoRoot = path.resolve(webRoot, '../..')

const sourceFiles = {
  frontendDomain: path.join(webRoot, 'src/features/board/domain/document/nodes.ts'),
  frontendSchema: path.join(webRoot, 'src/features/board/domain/schema/stateSchema.ts'),
  backendState: path.join(repoRoot, 'apps/api/internal/boards/state.go'),
  openApi: path.join(repoRoot, 'apps/api/openapi.yaml'),
  generatedEnvelope: path.join(
    webRoot,
    'src/shared/api/generated-query/model/boardStateEnvelope.ts',
  ),
  generatedSchemaVersion: path.join(
    webRoot,
    'src/shared/api/generated-query/model/boardStateEnvelopeSchemaVersion.ts',
  ),
  generatedGroup: path.join(webRoot, 'src/shared/api/generated-query/model/boardStateGroup.ts'),
} as const

const requiredGroupFields = ['id', 'nodeIds', 'x', 'y', 'width', 'height'] as const
const positiveGeometryFields = ['width', 'height'] as const
const expectedGroupIdPattern = '^group-[0-9]+$'
const expectedGroupNodeIdPattern = '^(image|prompt|fusion)-[0-9]+(?:\\.[0-9]+)?$'

function repoPath(filePath: string): string {
  return path.relative(repoRoot, filePath).split(path.sep).join('/')
}

async function readContractSources(): Promise<ContractSources> {
  const [
    frontendDomain,
    frontendSchema,
    backendState,
    openApi,
    generatedEnvelope,
    generatedSchemaVersion,
    generatedGroup,
  ] = await Promise.all([
    readFile(sourceFiles.frontendDomain, 'utf8'),
    readFile(sourceFiles.frontendSchema, 'utf8'),
    readFile(sourceFiles.backendState, 'utf8'),
    readFile(sourceFiles.openApi, 'utf8'),
    readFile(sourceFiles.generatedEnvelope, 'utf8'),
    readFile(sourceFiles.generatedSchemaVersion, 'utf8'),
    readFile(sourceFiles.generatedGroup, 'utf8'),
  ])

  return {
    frontendDomain,
    frontendSchema,
    backendState,
    openApi,
    generatedEnvelope,
    generatedSchemaVersion,
    generatedGroup,
  }
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function extractCapture(source: string, pattern: RegExp, label: string, file: string): string {
  const match = pattern.exec(source)
  const value = match?.[1]
  if (!value) {
    throw new Error(`Could not extract ${label} from ${repoPath(file)}.`)
  }
  return value
}

function extractNumberConst(source: string, name: string, file: string): number {
  const value = extractCapture(
    source,
    new RegExp(`\\b${escapeRegExp(name)}\\s*=\\s*(\\d+)`),
    name,
    file,
  )
  return Number(value)
}

function extractYamlSchemaSection(source: string, schemaName: string, file: string): string {
  const marker = `    ${schemaName}:\n`
  const start = source.indexOf(marker)
  if (start < 0) {
    throw new Error(`Could not find OpenAPI schema ${schemaName} in ${repoPath(file)}.`)
  }

  const afterMarker = start + marker.length
  const rest = source.slice(afterMarker)
  const nextSchemaIndex = rest.search(/\n {4}[A-Za-z0-9]+:\n/)
  const end = nextSchemaIndex < 0 ? source.length : afterMarker + nextSchemaIndex

  return source.slice(start, end)
}

function parseInlineList(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function parseNumberList(value: string, label: string, file: string): number[] {
  return parseInlineList(value).map((item) => {
    const numberValue = Number(item)
    if (!Number.isInteger(numberValue)) {
      throw new Error(`Expected ${label} in ${repoPath(file)} to contain integers, got ${item}.`)
    }
    return numberValue
  })
}

function extractOpenApiRequiredFields(section: string, file: string): string[] {
  return parseInlineList(
    extractCapture(section, /required:\s*\[([^\]]+)\]/, 'BoardStateGroup.required', file),
  )
}

function extractOpenApiSchemaVersionEnum(section: string, file: string): number[] {
  const list = extractCapture(
    section,
    /schemaVersion:[\s\S]*?enum:\s*\[([^\]]+)\]/,
    'BoardStateEnvelope.schemaVersion enum',
    file,
  )
  return parseNumberList(list, 'BoardStateEnvelope.schemaVersion enum', file)
}

function extractInterfaceRequiredFields(
  source: string,
  interfaceName: string,
  file: string,
): string[] {
  const body = extractCapture(
    source,
    new RegExp(`export interface ${escapeRegExp(interfaceName)} \\{([\\s\\S]*?)\\n\\}`),
    `${interfaceName} interface`,
    file,
  )

  return [...body.matchAll(/^\s*([A-Za-z][A-Za-z0-9]*)\??:/gm)]
    .filter((match) => !match[0].includes('?:'))
    .map((match) => match[1])
    .filter((field): field is string => Boolean(field))
}

function extractStrictObjectBlock(source: string, constName: string, file: string): string {
  return extractCapture(
    source,
    new RegExp(`const ${escapeRegExp(constName)} = v\\.strictObject\\(\\{([\\s\\S]*?)\\n\\}\\)`),
    constName,
    file,
  )
}

function extractStrictObjectFields(source: string, constName: string, file: string): string[] {
  const block = extractStrictObjectBlock(source, constName, file)
  return [...block.matchAll(/^\s*([A-Za-z][A-Za-z0-9]*):/gm)]
    .map((match) => match[1])
    .filter((field): field is string => Boolean(field))
}

function extractStrictObjectFieldExpression(
  source: string,
  constName: string,
  fieldName: string,
  file: string,
): string {
  const block = extractStrictObjectBlock(source, constName, file)
  return extractCapture(
    block,
    new RegExp(`^\\s*${escapeRegExp(fieldName)}:\\s*([^,\\n]+),?`, 'm'),
    `${constName}.${fieldName}`,
    file,
  ).trim()
}

function extractValibotRegex(source: string, constName: string, file: string): string {
  return extractCapture(
    source,
    new RegExp(`const ${escapeRegExp(constName)} =[\\s\\S]*?v\\.regex\\(/(.+?)/\\)`),
    `${constName} regex`,
    file,
  )
}

function extractGoFunctionBody(source: string, functionName: string, file: string): string {
  const start = source.indexOf(`func ${functionName}(`)
  if (start < 0) {
    throw new Error(`Could not find Go function ${functionName} in ${repoPath(file)}.`)
  }

  const rest = source.slice(start)
  const endMatch = /^}/m.exec(rest)
  if (!endMatch) {
    throw new Error(`Could not find end of Go function ${functionName} in ${repoPath(file)}.`)
  }

  return rest.slice(0, endMatch.index)
}

function extractBackendRequiredGroupFields(functionBody: string): string[] {
  return uniqueStrings(
    [...functionBody.matchAll(/required(?:String|StringArray|Number)Field\(raw, "([^"]+)"\)/g)]
      .map((match) => match[1])
      .filter((field): field is string => Boolean(field)),
  )
}

function extractBackendAllowedGroupFields(functionBody: string): string[] {
  return uniqueStrings(
    [...functionBody.matchAll(/key != "([^"]+)"/g)]
      .map((match) => match[1])
      .filter((field): field is string => Boolean(field)),
  )
}

function extractGoRegexp(source: string, name: string, file: string): string {
  return extractCapture(
    source,
    new RegExp(`${escapeRegExp(name)}\\s*=\\s*regexp\\.MustCompile\\(` + '`([^`]+)`' + `\\)`),
    name,
    file,
  )
}

function extractGeneratedSchemaVersions(
  source: string,
  file: string,
): GeneratedSchemaVersionEntry[] {
  const entries = [...source.matchAll(/NUMBER_(\d+):\s*(\d+)/g)].map((match) => {
    const key = match[1]
    const value = match[2]
    if (!key || !value) {
      throw new Error(`Could not extract generated schemaVersion enum entry in ${repoPath(file)}.`)
    }
    return {
      key: Number(key),
      value: Number(value),
    }
  })

  if (entries.length === 0) {
    throw new Error(`Could not extract generated schemaVersion enum from ${repoPath(file)}.`)
  }

  return entries
}

function extractGeneratedPattern(source: string, field: string, tag: string, file: string): string {
  return extractCapture(
    source,
    new RegExp(`/\\*\\* @${escapeRegExp(tag)} ([^*]+?) \\*/\\s*\\n\\s*${escapeRegExp(field)}:`),
    `generated ${field} ${tag}`,
    file,
  ).trim()
}

function normalizePattern(pattern: string): string {
  let normalized = pattern.trim()
  while (normalized.includes('\\\\')) {
    normalized = normalized.replace(/\\\\/g, '\\')
  }

  return normalized
    .replace(/\\d/g, '[0-9]')
    .replace(/\(\?:image\|prompt\|fusion\)/g, '(image|prompt|fusion)')
}

function uniqueStrings(values: readonly string[]): string[] {
  return [...new Set(values)]
}

function sameMembers(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((item) => right.includes(item))
}

function list(values: readonly (number | string)[]): string {
  return `[${values.join(', ')}]`
}

function addSchemaVersionChecks(sources: ContractSources, violations: Violation[]) {
  const openApiEnvelopeSection = extractYamlSchemaSection(
    sources.openApi,
    'BoardStateEnvelope',
    sourceFiles.openApi,
  )
  const generatedEntries = extractGeneratedSchemaVersions(
    sources.generatedSchemaVersion,
    sourceFiles.generatedSchemaVersion,
  )
  const sourceValues: SourceValues<number>[] = [
    {
      label: 'frontend domain BOARD_STATE_SCHEMA_VERSION',
      file: repoPath(sourceFiles.frontendDomain),
      values: [
        extractNumberConst(
          sources.frontendDomain,
          'BOARD_STATE_SCHEMA_VERSION',
          sourceFiles.frontendDomain,
        ),
      ],
    },
    {
      label: 'backend CurrentBoardStateSchemaVersion',
      file: repoPath(sourceFiles.backendState),
      values: [
        extractNumberConst(
          sources.backendState,
          'CurrentBoardStateSchemaVersion',
          sourceFiles.backendState,
        ),
      ],
    },
    {
      label: 'OpenAPI BoardStateEnvelope.schemaVersion enum',
      file: repoPath(sourceFiles.openApi),
      values: extractOpenApiSchemaVersionEnum(openApiEnvelopeSection, sourceFiles.openApi),
    },
    {
      label: 'generated BoardStateEnvelopeSchemaVersion',
      file: repoPath(sourceFiles.generatedSchemaVersion),
      values: generatedEntries.map((entry) => entry.value),
    },
  ]

  const nonSingleValueSources = sourceValues.filter((source) => source.values.length !== 1)
  if (nonSingleValueSources.length > 0) {
    violations.push({
      check: 'schemaVersion alignment',
      message: 'Each schemaVersion source must expose exactly one value.',
      details: nonSingleValueSources.map(
        (source) => `${source.label} (${source.file}) has ${list(source.values)}`,
      ),
    })
  }

  const values = sourceValues.flatMap((source) => source.values)
  if (new Set(values).size > 1) {
    violations.push({
      check: 'schemaVersion alignment',
      message: 'Board state schemaVersion values drifted.',
      details: sourceValues.map(
        (source) => `${source.label} (${source.file}): ${list(source.values)}`,
      ),
    })
  }

  const keyValueMismatches = generatedEntries.filter((entry) => entry.key !== entry.value)
  if (keyValueMismatches.length > 0) {
    violations.push({
      check: 'generated schemaVersion enum',
      message: 'Generated BoardStateEnvelopeSchemaVersion NUMBER_* keys must match values.',
      details: keyValueMismatches.map((entry) => `NUMBER_${entry.key}: ${entry.value}`),
    })
  }

  if (!/schemaVersion:\s*BoardStateEnvelopeSchemaVersion\b/.test(sources.generatedEnvelope)) {
    violations.push({
      check: 'generated schemaVersion enum',
      message:
        'Generated BoardStateEnvelope must type schemaVersion as BoardStateEnvelopeSchemaVersion.',
      details: [repoPath(sourceFiles.generatedEnvelope)],
    })
  }

  const frontendSchemaVersionLiteralCount = [
    ...sources.frontendSchema.matchAll(
      /schemaVersion:\s*v\.literal\(BOARD_STATE_SCHEMA_VERSION\)/g,
    ),
  ].length
  if (frontendSchemaVersionLiteralCount < 2) {
    violations.push({
      check: 'frontend schemaVersion literal',
      message:
        'Frontend snapshot and state schemas must use BOARD_STATE_SCHEMA_VERSION literals.',
      details: [
        `${repoPath(sourceFiles.frontendSchema)} has ${frontendSchemaVersionLiteralCount} literal reference(s).`,
      ],
    })
  }
}

function addFieldSetChecks(sources: ContractSources, violations: Violation[]) {
  const openApiGroupSection = extractYamlSchemaSection(
    sources.openApi,
    'BoardStateGroup',
    sourceFiles.openApi,
  )
  const backendGroupFunction = extractGoFunctionBody(
    sources.backendState,
    'validateBoardStateGroup',
    sourceFiles.backendState,
  )
  const fieldSources: SourceValues<string>[] = [
    {
      label: 'frontend CanvasGroup domain interface',
      file: repoPath(sourceFiles.frontendDomain),
      values: extractInterfaceRequiredFields(
        sources.frontendDomain,
        'CanvasGroup',
        sourceFiles.frontendDomain,
      ),
    },
    {
      label: 'frontend canvasGroupSchema',
      file: repoPath(sourceFiles.frontendSchema),
      values: extractStrictObjectFields(
        sources.frontendSchema,
        'canvasGroupSchema',
        sourceFiles.frontendSchema,
      ),
    },
    {
      label: 'backend validateBoardStateGroup required fields',
      file: repoPath(sourceFiles.backendState),
      values: extractBackendRequiredGroupFields(backendGroupFunction),
    },
    {
      label: 'backend validateBoardStateGroup allowed fields',
      file: repoPath(sourceFiles.backendState),
      values: extractBackendAllowedGroupFields(backendGroupFunction),
    },
    {
      label: 'OpenAPI BoardStateGroup.required',
      file: repoPath(sourceFiles.openApi),
      values: extractOpenApiRequiredFields(openApiGroupSection, sourceFiles.openApi),
    },
    {
      label: 'generated BoardStateGroup interface',
      file: repoPath(sourceFiles.generatedGroup),
      values: extractInterfaceRequiredFields(
        sources.generatedGroup,
        'BoardStateGroup',
        sourceFiles.generatedGroup,
      ),
    },
  ]

  const driftedSources = fieldSources.filter(
    (source) => !sameMembers(source.values, requiredGroupFields),
  )
  if (driftedSources.length > 0) {
    violations.push({
      check: 'BoardStateGroup required fields',
      message: `BoardStateGroup fields must remain ${list(requiredGroupFields)}.`,
      details: driftedSources.map(
        (source) => `${source.label} (${source.file}): ${list(source.values)}`,
      ),
    })
  }
}

function addPatternChecks(sources: ContractSources, violations: Violation[]) {
  const openApiGroupSection = extractYamlSchemaSection(
    sources.openApi,
    'BoardStateGroup',
    sourceFiles.openApi,
  )
  const groupIdPatternSources: SourceValue<string>[] = [
    {
      label: 'frontend canvasGroupIdSchema',
      file: repoPath(sourceFiles.frontendSchema),
      value: extractValibotRegex(
        sources.frontendSchema,
        'canvasGroupIdSchema',
        sourceFiles.frontendSchema,
      ),
    },
    {
      label: 'backend boardStateGroupIDPattern',
      file: repoPath(sourceFiles.backendState),
      value: extractGoRegexp(
        sources.backendState,
        'boardStateGroupIDPattern',
        sourceFiles.backendState,
      ),
    },
    {
      label: 'OpenAPI BoardStateGroup.id.pattern',
      file: repoPath(sourceFiles.openApi),
      value: extractCapture(
        openApiGroupSection,
        /\n {8}id:\n\s+type: string\n\s+pattern:\s*'([^']+)'/,
        'BoardStateGroup.id.pattern',
        sourceFiles.openApi,
      ),
    },
    {
      label: 'generated BoardStateGroup.id JSDoc',
      file: repoPath(sourceFiles.generatedGroup),
      value: extractGeneratedPattern(
        sources.generatedGroup,
        'id',
        'pattern',
        sourceFiles.generatedGroup,
      ),
    },
  ]
  const groupNodeIdPatternSources: SourceValue<string>[] = [
    {
      label: 'frontend canvasGroupNodeIdSchema',
      file: repoPath(sourceFiles.frontendSchema),
      value: extractValibotRegex(
        sources.frontendSchema,
        'canvasGroupNodeIdSchema',
        sourceFiles.frontendSchema,
      ),
    },
    {
      label: 'backend boardStateGroupNodeIDPattern',
      file: repoPath(sourceFiles.backendState),
      value: extractGoRegexp(
        sources.backendState,
        'boardStateGroupNodeIDPattern',
        sourceFiles.backendState,
      ),
    },
    {
      label: 'OpenAPI BoardStateGroup.nodeIds.items.pattern',
      file: repoPath(sourceFiles.openApi),
      value: extractCapture(
        openApiGroupSection,
        /nodeIds:[\s\S]*?items:[\s\S]*?pattern:\s*'([^']+)'/,
        'BoardStateGroup.nodeIds.items.pattern',
        sourceFiles.openApi,
      ),
    },
    {
      label: 'generated BoardStateGroup.nodeIds JSDoc',
      file: repoPath(sourceFiles.generatedGroup),
      value: extractGeneratedPattern(
        sources.generatedGroup,
        'nodeIds',
        'items.pattern',
        sourceFiles.generatedGroup,
      ),
    },
  ]

  addPatternDriftViolation(
    violations,
    'BoardStateGroup id pattern',
    expectedGroupIdPattern,
    groupIdPatternSources,
  )
  addPatternDriftViolation(
    violations,
    'BoardStateGroup node id pattern',
    expectedGroupNodeIdPattern,
    groupNodeIdPatternSources,
  )
}

function addPatternDriftViolation(
  violations: Violation[],
  check: string,
  expectedPattern: string,
  sources: readonly SourceValue<string>[],
) {
  const driftedSources = sources.filter(
    (source) => normalizePattern(source.value) !== expectedPattern,
  )
  if (driftedSources.length === 0) return

  violations.push({
    check,
    message: `Expected normalized pattern ${expectedPattern}.`,
    details: driftedSources.map(
      (source) =>
        `${source.label} (${source.file}): ${source.value} -> ${normalizePattern(source.value)}`,
    ),
  })
}

function frontendPositiveNumberSchemaIsExclusive(source: string): boolean {
  return /const positiveFiniteNumberSchema = v\.pipe\(\s*finiteNumberSchema,\s*v\.minValue\(Number\.MIN_VALUE\),?\s*\)/.test(
    source,
  )
}

function backendHasPositiveNumberGuard(functionBody: string, field: string): boolean {
  return new RegExp(
    `${escapeRegExp(field)}\\s*,\\s*err := requiredNumberField\\(raw, "${escapeRegExp(
      field,
    )}"\\)[\\s\\S]*?${escapeRegExp(field)}\\s*<=\\s*0`,
  ).test(functionBody)
}

function openApiHasExclusivePositiveNumber(section: string, field: string): boolean {
  return new RegExp(
    `${escapeRegExp(
      field,
    )}:\\s*\\{\\s*type:\\s*number,\\s*minimum:\\s*0,\\s*exclusiveMinimum:\\s*true\\s*\\}`,
  ).test(section)
}

function generatedHasExclusiveMinimumComment(source: string, field: string): boolean {
  return new RegExp(
    `/\\*\\* @exclusiveMinimum 0 \\*/\\s*\\n\\s*${escapeRegExp(field)}: number`,
  ).test(source)
}

function addPositiveGeometryChecks(sources: ContractSources, violations: Violation[]) {
  const openApiGroupSection = extractYamlSchemaSection(
    sources.openApi,
    'BoardStateGroup',
    sourceFiles.openApi,
  )
  const backendGroupFunction = extractGoFunctionBody(
    sources.backendState,
    'validateBoardStateGroup',
    sourceFiles.backendState,
  )
  const positiveSchemaIsExclusive = frontendPositiveNumberSchemaIsExclusive(sources.frontendSchema)
  const checks: BooleanCheck[] = positiveGeometryFields.flatMap((field) => {
    const frontendExpression = extractStrictObjectFieldExpression(
      sources.frontendSchema,
      'canvasGroupSchema',
      field,
      sourceFiles.frontendSchema,
    )

    return [
      {
        label: `frontend canvasGroupSchema.${field}`,
        file: repoPath(sourceFiles.frontendSchema),
        ok: positiveSchemaIsExclusive && frontendExpression === 'positiveFiniteNumberSchema',
        detail: `${field} must use positiveFiniteNumberSchema backed by v.minValue(Number.MIN_VALUE).`,
      },
      {
        label: `backend validateBoardStateGroup.${field}`,
        file: repoPath(sourceFiles.backendState),
        ok: backendHasPositiveNumberGuard(backendGroupFunction, field),
        detail: `${field} must reject values <= 0 after requiredNumberField.`,
      },
      {
        label: `OpenAPI BoardStateGroup.${field}`,
        file: repoPath(sourceFiles.openApi),
        ok: openApiHasExclusivePositiveNumber(openApiGroupSection, field),
        detail: `${field} must use minimum: 0 with exclusiveMinimum: true.`,
      },
      {
        label: `generated BoardStateGroup.${field}`,
        file: repoPath(sourceFiles.generatedGroup),
        ok: generatedHasExclusiveMinimumComment(sources.generatedGroup, field),
        detail: `${field} must carry generated @exclusiveMinimum 0 metadata.`,
      },
    ]
  })

  const failedChecks = checks.filter((check) => !check.ok)
  if (failedChecks.length > 0) {
    violations.push({
      check: 'BoardStateGroup positive geometry',
      message: 'BoardStateGroup width and height must stay positive across all contract sources.',
      details: failedChecks.map((check) => `${check.label} (${check.file}): ${check.detail}`),
    })
  }
}

function printViolations(violations: readonly Violation[]) {
  console.error('BoardStateGroup contract drift check failed.')
  console.error('')

  for (const violation of violations) {
    console.error(`- ${violation.check}`)
    console.error(`  ${violation.message}`)
    for (const detail of violation.details ?? []) {
      console.error(`  - ${detail}`)
    }
  }

  console.error('')
  console.error(
    'Fix the drift in the frontend schema/domain, Go validator, OpenAPI, or regenerate the API model with `npm run api:generate` if OpenAPI changed.',
  )
}

async function main() {
  const sources = await readContractSources()
  const violations: Violation[] = []

  addSchemaVersionChecks(sources, violations)
  addFieldSetChecks(sources, violations)
  addPatternChecks(sources, violations)
  addPositiveGeometryChecks(sources, violations)

  if (violations.length > 0) {
    printViolations(violations)
    process.exitCode = 1
    return
  }

  console.log('BoardStateGroup contract drift check passed.')
}

void main().catch((error: unknown) => {
  console.error('BoardStateGroup contract drift check could not inspect all contract sources.')
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
