<template>
  <div class="min-w-0 overflow-hidden" :class="props.frameClass" v-bind="wrapperAttrs">
    <div
      :class="scrollContainerClass"
      :role="props.scrollAriaLabel ? 'region' : undefined"
      :aria-label="props.scrollAriaLabel || undefined"
      :tabindex="props.scrollAriaLabel ? 0 : undefined"
    >
      <table
        :aria-label="props.tableAriaLabel || undefined"
        :aria-busy="props.loading || undefined"
        :class="tableClass"
        :style="props.tableStyle"
      >
        <thead class="bg-background">
          <tr>
            <th
              v-for="col in columns"
              :key="col.key"
              scope="col"
              :style="col.style"
              :class="[headerCellClass, col.headerClass]"
            >
              <SlotRenderer v-if="col.headerSlot" :fn="col.headerSlot" :args="{}" />
              <template v-else>{{ col.header }}</template>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="props.loading">
            <td :colspan="Math.max(columns.length, 1)" :class="bodyCellClass">
              <slot name="loading">
                <div class="flex items-center justify-center px-6 py-10" role="status">
                  <Spinner class="size-6 text-muted-foreground" />
                </div>
              </slot>
            </td>
          </tr>
          <template v-else>
            <tr
              v-for="(row, rowIndex) in rows"
              :key="rowKey(row, rowIndex)"
              @click="handleRowClick($event, row, rowIndex)"
            >
              <td
                v-for="col in columns"
                :key="col.key"
                :style="col.style"
                :class="[bodyCellClass, col.bodyClass]"
              >
                <SlotRenderer
                  v-if="col.bodySlot"
                  :fn="col.bodySlot"
                  :args="{ data: row, field: col.field, index: rowIndex }"
                />
                <template v-else>{{ cellText(row, col) }}</template>
              </td>
            </tr>
          </template>
        </tbody>
      </table>

      <div v-if="!props.loading && rows.length === 0">
        <slot name="empty">
          <div
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="status"
          >
            <span class="font-semibold text-foreground">{{
              props.emptyTitle || t('common.states.empty')
            }}</span>
            <span v-if="props.emptyDescription" class="mt-1 text-sm text-muted-foreground">
              {{ props.emptyDescription }}
            </span>
          </div>
        </slot>
      </div>
    </div>

    <slot name="after" />
  </div>
</template>

<script setup lang="ts" generic="TRow extends object">
import {
  Comment,
  Fragment,
  computed,
  useAttrs,
  useSlots,
  type CSSProperties,
  type VNode,
} from 'vue'
import { useI18n } from 'vue-i18n'
import { Spinner } from '@/components/ui/spinner'

export type DataTableFrameRowClickPayload<TRow> = {
  originalEvent: MouseEvent
  data: TRow
  index: number
}

type ColumnDef = {
  key: string
  field?: string | undefined
  header?: string | undefined
  headerSlot?: ((slotProps: Record<string, unknown>) => unknown) | undefined
  bodySlot?: ((slotProps: Record<string, unknown>) => unknown) | undefined
  style?: string | CSSProperties | undefined
  headerClass?: string | undefined
  bodyClass?: string | undefined
}

type ColumnSlotFns = Pick<ColumnDef, 'headerSlot' | 'bodySlot'>

const props = withDefaults(
  defineProps<{
    value?: readonly TRow[] | null
    loading?: boolean
    dataKey?: string | ((item: TRow) => string) | undefined
    size?: 'small' | 'large' | undefined
    scrollable?: boolean
    tableStyle?: string | Record<string, string | number> | undefined
    tableAriaLabel?: string
    frameClass?: string
    scrollAriaLabel?: string
    emptyTitle?: string
    emptyDescription?: string
  }>(),
  {
    value: () => [],
    loading: false,
    dataKey: undefined,
    size: undefined,
    scrollable: false,
    tableStyle: undefined,
    tableAriaLabel: '',
    frameClass: '',
    scrollAriaLabel: '',
    emptyTitle: '',
    emptyDescription: '',
  },
)

const emit = defineEmits<{
  'row-click': [payload: DataTableFrameRowClickPayload<TRow>]
}>()

defineOptions({ inheritAttrs: false })

// scoped slot 函数本身就是合法的渲染输入：包一层函数式组件即可在模板里按行调用。
// prop 命名刻意避开连字符（hyphenation 会破坏函数式组件的 prop 匹配，见 UiColumn 迁移记录）。
function SlotRenderer(rendererProps: {
  fn: (innerProps: Record<string, unknown>) => unknown
  args: Record<string, unknown>
}) {
  return rendererProps.fn(rendererProps.args) as unknown as VNode
}

const { t } = useI18n()
const slots = useSlots()
const attrs = useAttrs()
// row-click 是已声明 emit，对应 onRowClick 监听不会落进 attrs；其余 class/testid 原样透传到外框。
const wrapperAttrs = computed(() => ({ ...attrs }))

const rows = computed<TRow[]>(() => [...(props.value ?? [])])

// Column 定义组件都是 renderless 声明：只读 vnode 的 props + slots，不渲染自身。
const columns = computed<ColumnDef[]>(() => {
  const vnodes = flattenSlotVNodes(slots.default?.() ?? [])
  return vnodes
    .map((vnode, index) => toColumnDef(vnode, index))
    .filter((col): col is ColumnDef => col !== undefined)
})

function flattenSlotVNodes(vnodes: VNode[]): VNode[] {
  const flat: VNode[] = []
  for (const vnode of vnodes) {
    if (vnode.type === Comment) continue
    if (vnode.type === Fragment && Array.isArray(vnode.children)) {
      flat.push(...flattenSlotVNodes(vnode.children as VNode[]))
      continue
    }
    if (typeof vnode.type === 'string') continue
    flat.push(vnode)
  }
  return flat
}

function toColumnDef(vnode: VNode, index: number): ColumnDef | undefined {
  const vnodeProps = (vnode.props ?? {}) as Record<string, unknown>
  const { headerSlot, bodySlot } = extractColumnSlots(vnode)
  const field = readStringProp(vnodeProps, 'field')
  const header = readStringProp(vnodeProps, 'header')
  const style = vnodeProps.style as ColumnDef['style']
  const headerClass = readStringProp(vnodeProps, 'headerClass')
  const bodyClass = readStringProp(vnodeProps, 'bodyClass') ?? readStringProp(vnodeProps, 'class')
  // 纯选择列（仅 selection-mode，无 field/header/slot）直接丢弃：原生表不支持行选择，
  // 与 Prime DataTable 的 selection 行为差异是刻意的，见 UiColumn 注释。
  if (
    field === undefined &&
    header === undefined &&
    bodySlot === undefined &&
    headerSlot === undefined
  ) {
    return undefined
  }
  return {
    key: field ?? header ?? `column-${index}`,
    field,
    header,
    headerSlot,
    bodySlot,
    style,
    headerClass,
    bodyClass,
  }
}

function readStringProp(props: Record<string, unknown>, key: string): string | undefined {
  const value = props[key]
  return typeof value === 'string' ? value : undefined
}

function extractColumnSlots(vnode: VNode): ColumnSlotFns {
  const children = vnode.children as
    Record<string, ((slotProps: Record<string, unknown>) => unknown) | unknown> | null | undefined
  const namedSlots =
    children !== null && typeof children === 'object' && !Array.isArray(children) ? children : {}
  return {
    bodySlot: readSlotFn(namedSlots, 'body') ?? readSlotFn(namedSlots, 'default'),
    headerSlot: readSlotFn(namedSlots, 'header'),
  }
}

function readSlotFn(
  namedSlots: Record<string, unknown>,
  key: string,
): ((slotProps: Record<string, unknown>) => unknown) | undefined {
  const slot = namedSlots[key]
  return typeof slot === 'function'
    ? (slot as (slotProps: Record<string, unknown>) => unknown)
    : undefined
}

function rowKey(row: TRow, index: number): string {
  if (typeof props.dataKey === 'function') return props.dataKey(row)
  if (typeof props.dataKey === 'string') {
    const key = (row as Record<string, unknown>)[props.dataKey]
    if (key !== undefined && key !== null) return String(key)
  }
  return `row-${index}`
}

function cellText(row: TRow, col: ColumnDef): string {
  if (!col.field) return ''
  const value = (row as Record<string, unknown>)[col.field]
  if (value === undefined || value === null) return ''
  return String(value)
}

function handleRowClick(event: MouseEvent, row: TRow, index: number) {
  emit('row-click', { originalEvent: event, data: row, index })
}

const tableClass = computed(
  () =>
    'w-full border-separate border-spacing-0 text-left align-middle [&_tbody_tr:last-child_td]:border-b-0',
)
const scrollContainerClass = computed(() =>
  props.scrollable ? 'app-scrollbar overflow-x-auto' : 'app-scrollbar overflow-x-visible',
)
// size 只有 small/large 显式档位，其余（含 undefined）走 default：新增档位只加表项。
const headerCellClasses = {
  small:
    'border-b border-border px-3 py-2 text-left align-middle text-admin-body font-semibold text-foreground',
  large:
    'border-b border-border px-5 py-4 text-left align-middle text-admin-title-sm font-semibold text-foreground',
  default:
    'border-b border-border px-4 py-3 text-left align-middle text-admin-body font-semibold text-foreground',
} as const
const headerCellClass = computed(() => headerCellClasses[props.size ?? 'default'])
const bodyCellClasses = {
  small: 'border-b border-border px-3 py-2 align-middle text-admin-body text-foreground',
  large: 'border-b border-border px-5 py-4 align-middle text-admin-title-sm text-foreground',
  default: 'border-b border-border px-4 py-3 align-middle text-sm text-foreground',
} as const
const bodyCellClass = computed(() => bodyCellClasses[props.size ?? 'default'])
</script>
