import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { graphEdges, loadSeamInventory, type CruiseModule } from './seamInventory.ts'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const ledgerPath = path.resolve(webRoot, '../../docs/architecture/knip-issue-ledger.md')
const inventory = loadSeamInventory()
const cruise = spawnSync(
  process.execPath,
  [
    path.join(webRoot, 'node_modules/dependency-cruiser/bin/dependency-cruise.mjs'),
    '--config',
    '.dependency-cruiser.cjs',
    '--output-type',
    'json',
    'src',
  ],
  { cwd: webRoot, encoding: 'utf8', maxBuffer: 40 * 1024 * 1024 },
)
if (cruise.error || cruise.status !== 0)
  throw new Error('Dependency graph failed; ledger was not changed.')
const modules = (JSON.parse(cruise.stdout) as { modules: CruiseModule[] }).modules
const edges = graphEdges(modules)
const isTest = (file: string) => /\.(?:spec|test)\.|^src\/test\//.test(file)
const reachable = new Set<string>()
const queue = ['src/app/main.ts', 'src/app/router/index.ts', 'src/app/App.vue']
while (queue.length) {
  const file = queue.pop()!
  if (reachable.has(file)) continue
  reachable.add(file)
  queue.push(
    ...edges
      .filter((edge) => edge.from === file && edge.kind === 'runtime' && !isTest(edge.to))
      .map((edge) => edge.to),
  )
}

type KnipSymbol = { name: string; line?: number }
type KnipReport = {
  issues: Array<{ file: string; [category: string]: string | Array<KnipSymbol | KnipSymbol[]> }>
}
type Row = {
  mode: string
  category: string
  file: string
  symbol: string
  line: string
  disposition: string
  owner: string
  evidence: string
}
const rows: Row[] = []
const summaries: Array<{ mode: string; exit: number | null; counts: Record<string, number> }> = []
const owner = (file: string) =>
  file.startsWith('src/features/admin/')
    ? 'p4-lane'
    : file.startsWith('src/features/canvas/tools/image-tools/')
      ? 'p6-lane'
      : file.startsWith('src/features/canvas/')
        ? 'p5-lane'
        : /^src\/features\/(account|board|search)\//.test(file)
          ? 'p3-lane'
          : /^src\/features\/(legal|auth|library|workspace)\//.test(file)
            ? 'p2-lane'
            : file.startsWith('src/components/')
              ? 'design-system'
              : 'p1-lane'
const fileDecisions: Record<string, [string, string]> = {
  'src/shared/assets/eric-studio-logo.svg.d.ts': [
    'delete-candidate',
    '与 shared/types/asset-modules.d.ts SVG 声明比较，P1 确认后删除',
  ],
  'src/shared/theme/composables/themeKeyboard.ts': [
    'delete-candidate',
    'production 图不可达；P1 复核 keyboard 场景后决定',
  ],
  'src/shared/api/workspaceTasks.ts': [
    'compatibility-review',
    'stable port barrel；未伪造 shim TTL，P1 复核公共导出',
  ],
  'src/shared/i18n/locales/en/index.ts': [
    'tool-review',
    'parity bundle entry；i18n 选型完成前保留',
  ],
  'src/features/legal/index.ts': [
    'retain-public-entry',
    'canonical inventory + surface；P2 收敛 route chunk direct import',
  ],
}

for (const [mode, args] of [
  ['normal', ['--config', 'knip.jsonc']],
  ['production', ['--config', 'knip.production.jsonc', '--production', '--strict']],
] as const) {
  const result = spawnSync(
    process.execPath,
    [
      path.join(webRoot, 'node_modules/knip/bin/knip.js'),
      ...args,
      '--reporter',
      'json',
      '--no-progress',
    ],
    {
      cwd: webRoot,
      encoding: 'utf8',
      maxBuffer: 30 * 1024 * 1024,
    },
  )
  if (result.error || (result.status !== 0 && result.status !== 1))
    throw new Error(`${mode} Knip execution failed; ledger was not changed.`)
  const report = JSON.parse(result.stdout) as KnipReport
  if (!Array.isArray(report.issues)) throw new Error(`${mode} Knip report has no issues array`)
  const counts: Record<string, number> = {}
  for (const record of report.issues) {
    const incoming = edges.filter((edge) => edge.to === record.file)
    const productionConsumers = [
      ...new Set(incoming.filter((edge) => !isTest(edge.from)).map((edge) => edge.from)),
    ].sort()
    const testConsumers = [
      ...new Set(incoming.filter((edge) => isTest(edge.from)).map((edge) => edge.from)),
    ].sort()
    const graphEvidence = `source module runtime-reachable=${reachable.has(record.file)}；production/test direct consumers=${productionConsumers.length}/${testConsumers.length}；runtime/type edges=${incoming.filter((edge) => edge.kind === 'runtime').length}/${incoming.filter((edge) => edge.kind === 'type-only').length}；examples=${[...productionConsumers.slice(0, 1), ...testConsumers.slice(0, 1)].join(', ') || 'none'}`
    for (const [category, issues] of Object.entries(record)) {
      if (!Array.isArray(issues)) continue
      for (const issue of issues) {
        counts[category] = (counts[category] ?? 0) + 1
        const symbol = Array.isArray(issue)
          ? issue
              .map((entry) => entry.name)
              .sort()
              .join(' = ')
          : issue.name
        const line = Array.isArray(issue)
          ? issue.map((entry) => entry.line ?? '').join(',')
          : String(issue.line ?? '')
        const seam = inventory.seams.find(
          (entry) =>
            entry.status === 'active' &&
            entry.path === record.file &&
            entry.publicExports.includes(symbol),
        )
        let disposition = 'retain-until-review'
        let reason = reachable.has(record.file)
          ? `${mode}/${category} 尚未裁决此 symbol；所在 runtime module 迁移时复核 named consumers 与 scenario 后决定`
          : testConsumers.length
            ? `${mode}/${category} 尚未裁决此 symbol；保留 test/tool 场景至 owner 复核后决定`
            : `${mode}/${category} 尚未裁决此 symbol；无 runtime reachability 证明，owner 复核 config/dynamic entry 后决定`
        if (seam) {
          disposition = 'retain-public-export'
          reason = `inventory ${seam.path} 同一 symbol + surface baseline；只保留该已声明公共导出`
        }
        if (record.file.startsWith('src/shared/api/generated-query/')) {
          disposition = 'generator-review'
          reason = 'Orval 生成 symbol；保留至 owner 审查 generator/client 消费，禁止手改'
        }
        if (category === 'files' && fileDecisions[record.file])
          [disposition, reason] = fileDecisions[record.file]!
        if (category === 'duplicates') {
          disposition = 'compatibility-review'
          reason = 'named api 与 default 指向同一实例；P1 复核 default 公共契约后决定'
        }
        rows.push({
          mode,
          category,
          file: record.file,
          symbol,
          line,
          disposition,
          owner: owner(record.file),
          evidence: `${reason}；${graphEvidence}`,
        })
      }
    }
  }
  summaries.push({ mode, exit: result.status, counts })
}
rows.sort((a, b) =>
  `${a.mode}|${a.category}|${a.file}|${a.symbol}` < `${b.mode}|${b.category}|${b.file}|${b.symbol}`
    ? -1
    : 1,
)
const escapeCell = (value: string) => value.replaceAll('|', '\\|').replaceAll('\n', ' ')
const summary = summaries
  .map(
    (entry) =>
      `| ${entry.mode} | ${entry.exit} | ${Object.entries(entry.counts)
        .map(([category, count]) => `${category}=${count}`)
        .join(', ')} |`,
  )
  .join('\n')
const table = rows
  .map(
    (row, index) =>
      `| K${String(index + 1).padStart(4, '0')} | ${row.mode}/${row.category} | \`${row.file}${row.line ? ':' + row.line : ''}\` | \`${escapeCell(row.symbol)}\` | ${row.disposition} | ${row.owner} | ${escapeCell(row.evidence)} |`,
  )
  .join('\n')
let source = fs.readFileSync(ledgerPath, 'utf8')
if (!source.includes('<!-- KNIP_ISSUES_END -->'))
  source = source.trimEnd() + '\n<!-- KNIP_ISSUES_END -->\n'
if (
  !source.includes('<!-- KNIP_SUMMARY_START -->') ||
  !source.includes('<!-- KNIP_ISSUES_START -->')
)
  throw new Error('Knip ledger regeneration markers missing')
source = source.replace(
  /<!-- KNIP_SUMMARY_START -->[\s\S]*?<!-- KNIP_SUMMARY_END -->/,
  `<!-- KNIP_SUMMARY_START -->\n\n| Mode | Exit | Issues |\n|---|---|---|\n${summary}\n\n<!-- KNIP_SUMMARY_END -->`,
)
source = source.replace(/总计 \d+ 个当前 issues/, `总计 ${rows.length} 个当前 issues`)
source = source.replace(
  /<!-- KNIP_ISSUES_START -->[\s\S]*?<!-- KNIP_ISSUES_END -->/,
  `<!-- KNIP_ISSUES_START -->\n\n| ID | Mode / Category | File : Line | Symbol | Disposition | Owner | Evidence / Next Check |\n|---|---|---|---|---|---|---|\n${table}\n\n<!-- KNIP_ISSUES_END -->`,
)
if (process.argv.includes('--update')) fs.writeFileSync(ledgerPath, source)
else if (normalizeLedger(source) !== normalizeLedger(fs.readFileSync(ledgerPath, 'utf8'))) {
  console.error(
    'Knip ledger drift; review and regenerate with npm run audit:knip-ledger -- --update.',
  )
  process.exit(1)
}
function normalizeLedger(markdown: string): string {
  return markdown
    .split('\n')
    .filter((line) => line.trim() && !/^\|[\s:|-]+\|$/.test(line))
    .map((line) => (line.startsWith('|') ? line.replace(/\s*\|\s*/g, '|') : line.trimEnd()))
    .join('\n')
}
console.log(
  JSON.stringify({ summaries, issues: rows.length, updated: process.argv.includes('--update') }),
)
