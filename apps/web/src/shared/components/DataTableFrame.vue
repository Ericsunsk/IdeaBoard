<template>
  <div class="min-w-0 overflow-hidden" :class="frameClass">
    <DataTable
      v-bind="$attrs"
      :value="value"
      :loading="loading"
      :data-key="dataKey"
      :size="size"
      :scrollable="scrollable"
      :pt="dataTablePt"
      :table-style="tableStyle"
      :table-props="tableProps"
    >
      <template v-for="slotName in forwardedSlotNames" #[slotName]="slotProps">
        <slot :name="slotName" v-bind="slotProps || {}" />
      </template>

      <template #empty>
        <slot name="empty">
          <div
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="status"
          >
            <span class="font-semibold text-ib-text">{{
              emptyTitle || t('common.states.empty')
            }}</span>
            <span v-if="emptyDescription" class="mt-1 text-sm text-ib-text-muted">
              {{ emptyDescription }}
            </span>
          </div>
        </slot>
      </template>

      <template v-if="slots.loading" #loading>
        <slot name="loading" />
      </template>
    </DataTable>

    <slot name="after" />
  </div>
</template>

<script setup lang="ts" generic="TRow extends object">
import { computed, useSlots, type TableHTMLAttributes } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable, { type DataTablePassThroughOptions } from 'primevue/datatable'

const props = withDefaults(
  defineProps<{
    value?: readonly TRow[] | null
    loading?: boolean
    dataKey?: string | ((item: TRow) => string)
    size?: 'small' | 'large'
    scrollable?: boolean
    tableStyle?: string | Record<string, string | number>
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

defineOptions({ inheritAttrs: false })

const { t } = useI18n()

const slots = useSlots()
const forwardedSlotNames = computed(() =>
  Object.keys(slots).filter((name) => name !== 'empty' && name !== 'loading' && name !== 'after'),
)
const tableProps = computed<TableHTMLAttributes | undefined>(() =>
  props.tableAriaLabel ? { 'aria-label': props.tableAriaLabel } : undefined,
)
const dataTablePt = computed<DataTablePassThroughOptions>(() => ({
  tableContainer: {
    class: 'app-scrollbar',
    role: props.scrollAriaLabel ? 'region' : undefined,
    'aria-label': props.scrollAriaLabel || undefined,
    tabindex: props.scrollAriaLabel ? 0 : undefined,
  },
}))
</script>
