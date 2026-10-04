<template>
  <div class="min-w-0 overflow-hidden" :class="frameClass">
    <div
      :class="scrollable ? 'app-scrollbar overflow-x-auto' : 'min-w-0'"
      :role="scrollRegionRole"
      :aria-label="scrollAriaLabel || undefined"
      :tabindex="scrollRegionRole ? 0 : undefined"
    >
      <table
        :class="cn(tableClass, props.class)"
        :style="tableStyle"
        :aria-label="tableAriaLabel || undefined"
        :aria-busy="loading || undefined"
      >
        <template v-if="isDataDriven">
          <AdminTableHeader>
            <!-- 结构行（表头/加载/空态）不可交互：关闭行 hover 高亮，数据行保留 -->
            <AdminTableRow :hoverable="false">
              <AdminTableHead
                v-for="column in columns"
                :key="column.key"
                scope="col"
                :style="column.style"
              >
                <input
                  v-if="column.selectionMode === 'multiple'"
                  type="checkbox"
                  :checked="allSelected"
                  :indeterminate="someSelected && !allSelected"
                  :disabled="rows.length === 0"
                  aria-label="选择全部行"
                  class="size-4 accent-primary"
                  @change="toggleSelectAll(($event.target as HTMLInputElement).checked)"
                />
                <component :is="column.headerSlot" v-if="column.headerSlot" />
                <template v-else-if="!column.expander">{{ column.header }}</template>
              </AdminTableHead>
              <AdminTableHead v-if="columns.length === 0" scope="col">
                <span class="sr-only">数据</span>
              </AdminTableHead>
            </AdminTableRow>
          </AdminTableHeader>
          <AdminTableBody>
            <AdminTableRow v-if="loading && rows.length === 0" :hoverable="false">
              <AdminTableCell :colspan="columnCount" class="px-3 py-10 text-center">
                <AdminSpinner class="mx-auto size-7" aria-label="正在加载" />
              </AdminTableCell>
            </AdminTableRow>
            <template v-else-if="rows.length === 0">
              <AdminTableEmpty :colspan="columnCount">
                <slot name="empty">
                  <div
                    class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
                    role="status"
                  >
                    <span class="font-semibold text-foreground">{{
                      emptyTitle || t('common.states.empty')
                    }}</span>
                    <span v-if="emptyDescription" class="mt-1 text-sm text-muted-foreground">{{
                      emptyDescription
                    }}</span>
                  </div>
                </slot>
              </AdminTableEmpty>
            </template>
            <template v-else>
              <template v-for="(row, rowIndex) in rows" :key="rowKey(row, rowIndex)">
                <AdminTableRow :aria-selected="isSelected(row, rowIndex) || undefined">
                  <AdminTableCell v-for="column in columns" :key="column.key" :style="column.style">
                    <input
                      v-if="column.selectionMode === 'multiple'"
                      type="checkbox"
                      :checked="isSelected(row, rowIndex)"
                      :aria-label="selectionRowLabel(row, rowIndex)"
                      class="size-4 accent-primary"
                      @change="
                        toggleSelectRow(row, rowIndex, ($event.target as HTMLInputElement).checked)
                      "
                    />
                    <AdminButton
                      v-else-if="column.expander"
                      variant="ghost"
                      size="small"
                      :aria-expanded="isExpanded(row, rowIndex)"
                      :aria-label="isExpanded(row, rowIndex) ? '收起详情' : '展开详情'"
                      @click="toggleExpand(row, rowIndex)"
                    >
                      <template #icon>
                        <NavArrowDown
                          class="size-4 transition-transform"
                          :class="{ 'rotate-180': isExpanded(row, rowIndex) }"
                        />
                      </template>
                    </AdminButton>
                    <component
                      :is="column.body"
                      v-else-if="column.body"
                      :data="row"
                      :field="column.field"
                      :index="rowIndex"
                    />
                    <template v-else>{{ cellText(row, column.field) }}</template>
                  </AdminTableCell>
                </AdminTableRow>
                <AdminTableRow v-if="isExpanded(row, rowIndex) && hasExpansionSlot">
                  <AdminTableCell
                    :colspan="columnCount"
                    class="border-b border-border px-0 py-0"
                  >
                    <slot name="expansion" :data="row" :index="rowIndex" />
                  </AdminTableCell>
                </AdminTableRow>
              </template>
            </template>
          </AdminTableBody>
        </template>
        <template v-else>
          <slot />
        </template>
      </table>
    </div>

    <slot name="after" />
  </div>
</template>

<script setup lang="ts" generic="TRow extends object">
import { computed, useSlots, type HTMLAttributes, type VNode } from 'vue'
import { useI18n } from 'vue-i18n'
import { NavArrowDown } from '@iconoir/vue'
import { cn } from '@/lib/utils'
import AdminButton from './AdminButton.vue'
import AdminColumn from './AdminColumn.vue'
import AdminSpinner from './AdminSpinner.vue'
import AdminTableHeader from './AdminTableHeader.vue'
import AdminTableBody from './AdminTableBody.vue'
import AdminTableRow from './AdminTableRow.vue'
import AdminTableHead from './AdminTableHead.vue'
import AdminTableCell from './AdminTableCell.vue'
import AdminTableEmpty from './AdminTableEmpty.vue'

export type AdminTableRowEvent<TRow> = {
  data: TRow
  index: number
}

export type AdminTableRowExpandEvent<TRow> = AdminTableRowEvent<TRow>
export type AdminTableRowCollapseEvent<TRow = unknown> = {
  data: TRow
  index: number
}

type AdminTableColumnDef = {
  key: string
  field?: string | undefined
  header?: string | undefined
  style?: string | Record<string, string | number> | undefined
  selectionMode?: 'multiple' | undefined
  expander?: boolean | undefined
  body?: ((props: Record<string, unknown>) => VNode[]) | undefined
  headerSlot?: (() => VNode[]) | undefined
}

const props = withDefaults(
  defineProps<{
    value?: readonly TRow[] | null
    loading?: boolean
    dataKey?: string | ((item: TRow) => string) | undefined
    scrollable?: boolean
    tableStyle?: string | Record<string, string | number> | undefined
    tableAriaLabel?: string
    frameClass?: string
    scrollAriaLabel?: string
    emptyTitle?: string
    emptyDescription?: string
    selection?: readonly TRow[] | null | undefined
    expandedRows?: Record<string, boolean> | null | undefined
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    value: () => [],
    loading: false,
    dataKey: undefined,
    scrollable: false,
    tableStyle: undefined,
    tableAriaLabel: '',
    frameClass: '',
    scrollAriaLabel: '',
    emptyTitle: '',
    emptyDescription: '',
    selection: undefined,
    expandedRows: undefined,
    class: undefined,
  },
)

const emit = defineEmits<{
  'update:selection': [selection: TRow[]]
  'update:expandedRows': [expandedRows: Record<string, boolean>]
  'row-expand': [event: AdminTableRowExpandEvent<TRow>]
  'row-collapse': [event: AdminTableRowCollapseEvent<TRow>]
}>()

defineOptions({ inheritAttrs: false })

const { t } = useI18n()
const slots = useSlots()

const tableClass = 'w-full min-w-full border-collapse text-left'
const scrollRegionRole = computed(() => (props.scrollAriaLabel ? 'region' : undefined))
const hasExpansionSlot = computed(() => Boolean(slots.expansion))

function normalizeStyleKey(index: number, field?: string, header?: string): string {
  return field || header || `column-${index}`
}

const columns = computed<AdminTableColumnDef[]>(() => {
  const vnodes = (slots.default?.() ?? []).filter((vnode) => vnode.type === AdminColumn)
  return vnodes.map((vnode, index) => {
    const columnProps = (vnode.props ?? {}) as {
      field?: string
      header?: string
      style?: string | Record<string, string | number>
      selectionMode?: 'multiple'
      expander?: boolean
    }
    const children = vnode.children as
      { body?: (slotProps: Record<string, unknown>) => VNode[]; header?: () => VNode[] } | undefined
    return {
      key: normalizeStyleKey(index, columnProps.field, columnProps.header),
      field: columnProps.field,
      header: columnProps.header,
      style: columnProps.style,
      selectionMode: columnProps.selectionMode,
      // Bare `expander` attributes compile to empty-string vnode props;
      // only an explicit false opts out.
      expander: columnProps.expander !== undefined && columnProps.expander !== false,
      body: typeof children?.body === 'function' ? children.body : undefined,
      headerSlot: typeof children?.header === 'function' ? children.header : undefined,
    }
  })
})

const isDataDriven = computed(
  () =>
    columns.value.length > 0 ||
    (props.value !== undefined && props.value !== null && props.value.length > 0) ||
    props.loading ||
    Boolean(slots.empty),
)

const columnCount = computed(() => Math.max(columns.value.length, 1))
const rows = computed<readonly TRow[]>(() => props.value ?? [])
const hasSelection = computed(() =>
  columns.value.some((column) => column.selectionMode === 'multiple'),
)

function resolveKey(row: TRow, index: number): string {
  if (typeof props.dataKey === 'function') {
    try {
      return String(props.dataKey(row))
    } catch {
      return String(index)
    }
  }
  if (typeof props.dataKey === 'string' && props.dataKey) {
    const record = row as Record<string, unknown>
    return String(record[props.dataKey] ?? index)
  }
  return String(index)
}

function rowKey(row: TRow, index: number): string {
  return resolveKey(row, index)
}

function selectedKeys(): Set<string> {
  const keys = new Set<string>()
  for (const [index, row] of (props.selection ?? []).entries()) {
    keys.add(resolveKey(row as TRow, index))
  }
  return keys
}

function isSelected(row: TRow, index: number): boolean {
  if (!hasSelection.value) return false
  return selectedKeys().has(resolveKey(row, index))
}

const allSelected = computed(() => {
  // 跨页选择时 selection 含其它页行：全选只看当前页行是否都在集合内。
  if (!hasSelection.value || rows.value.length === 0) return false
  const keys = selectedKeys()
  return rows.value.every((row, index) => keys.has(resolveKey(row, index)))
})
const someSelected = computed(
  () => hasSelection.value && selectedKeys().size > 0 && !allSelected.value,
)

function selectionRowLabel(row: TRow, index: number): string {
  const rowId = resolveKey(row, index)
  return `选择行 ${rowId}`
}

let warnedSelectionNeedsDataKey = false
function warnSelectionNeedsDataKey() {
  // 无 dataKey 时行标识回退到下标，跨页/排序后即错位：仅开发期提示，不阻断。
  if (warnedSelectionNeedsDataKey || props.dataKey !== undefined || !import.meta.env.DEV) return
  warnedSelectionNeedsDataKey = true
  console.warn('[AdminTable] selection column requires data-key for stable row identity.')
}

function toggleSelectRow(row: TRow, index: number, checked: boolean) {
  warnSelectionNeedsDataKey()
  const key = resolveKey(row, index)
  const current = props.selection ? [...props.selection] : []
  const next = checked
    ? [...current.filter((item, itemIndex) => resolveKey(item as TRow, itemIndex) !== key), row]
    : current.filter((item, itemIndex) => resolveKey(item as TRow, itemIndex) !== key)
  emit('update:selection', next)
}

function toggleSelectAll(checked: boolean) {
  warnSelectionNeedsDataKey()
  emit('update:selection', checked ? [...rows.value] : [])
}

function isExpanded(row: TRow, index: number): boolean {
  const key = resolveKey(row, index)
  return Boolean(props.expandedRows?.[key])
}

function toggleExpand(row: TRow, index: number) {
  const key = resolveKey(row, index)
  const nextExpanded = { ...(props.expandedRows ?? {}) }
  const willExpand = !nextExpanded[key]
  if (willExpand) {
    nextExpanded[key] = true
    emit('update:expandedRows', nextExpanded)
    emit('row-expand', { data: row, index })
  } else {
    delete nextExpanded[key]
    emit('update:expandedRows', nextExpanded)
    emit('row-collapse', { data: row, index })
  }
}

function cellText(row: TRow, field?: string): string {
  if (!field) return ''
  const record = row as Record<string, unknown>
  const value = record[field]
  return value === undefined || value === null ? '' : String(value)
}
</script>
