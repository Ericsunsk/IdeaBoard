// @vitest-environment node
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/**
 * 表格分层契约：ui/table 是原子，AdminTable(+AdminColumn) 是唯一数据封装，
 * 业务页禁直拼 <table> 或 ui/table 原子。
 *
 * Allowlist（实现层 + 已评审例外）：
 * - `components/ui/table/*`：原子实现。
 * - `core/components/DataTableFrame.vue`：用户面数据表实现（Admin 不用）。
 * - `features/admin/shared/ui/table/*`：AdminTable 数据封装实现。
 * - `features/account/ui/usage/UsagePane.vue`：sr-only 无障碍数据表，无视觉表格。
 *
 * 说明：`AdminModelAddDialog.vue` 曾作为本地多选列表示例列入例外，现已无
 * 原生 <table>（仅 provider select + model input），故不再列入 allowlist；
 * 若将来对话框需要本地多选列表，必须改用 AdminTable，不得仿效手写表。
 */
const SRC_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', '..', '..')
const ALLOWLIST = new Set([
  'components/ui/table/Table.vue',
  'components/ui/table/TableBody.vue',
  'components/ui/table/TableCaption.vue',
  'components/ui/table/TableCell.vue',
  'components/ui/table/TableEmpty.vue',
  'components/ui/table/TableFooter.vue',
  'components/ui/table/TableHead.vue',
  'components/ui/table/TableHeader.vue',
  'components/ui/table/TableRow.vue',
  'core/components/DataTableFrame.vue',
  'features/account/ui/usage/UsagePane.vue',
])

function listVueFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) return listVueFiles(full)
    return full.endsWith('.vue') && !full.endsWith('.spec.ts') ? [full] : []
  })
}

function relativeToSrc(file: string): string {
  return file.slice(SRC_ROOT.length + 1).replace(/\\/g, '/')
}

describe('table layering contract', () => {
  it('keeps raw <table> inside the atomic/data-table implementations and reviewed exceptions', () => {
    const offenders: string[] = []
    for (const file of listVueFiles(join(SRC_ROOT))) {
      const rel = relativeToSrc(file)
      if (rel.startsWith('features/admin/shared/ui/table/')) continue
      if (ALLOWLIST.has(rel)) continue
      const source = readFileSync(file, 'utf8')
      if (/<table[\s>]/.test(source)) offenders.push(rel)
    }
    expect(offenders).toEqual([])
  })
})
