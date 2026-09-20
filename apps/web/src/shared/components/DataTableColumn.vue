<template>
  <!-- renderless 声明：DataTableFrame 解析 vnode 的 props/slots 生成原生 table，此 outlet 永不独立挂载。 -->
  <slot :data="undefined as unknown as TRow" :index="-1" />
</template>

<script setup lang="ts" generic="TRow = any">
import type { CSSProperties } from 'vue'

export type DataTableColumnBodySlotProps<TRow> = {
  data: TRow
  field?: string | undefined
  index: number
}

export type DataTableColumnSlots<TRow> = {
  body?(props: DataTableColumnBodySlotProps<TRow>): unknown
  header?(): unknown
  default?(props: DataTableColumnBodySlotProps<TRow>): unknown
}

withDefaults(
  defineProps<{
    field?: string | undefined
    header?: string | undefined
    style?: string | CSSProperties | undefined
    headerClass?: string | undefined
    bodyClass?: string | undefined
  }>(),
  {
    field: undefined,
    header: undefined,
    style: undefined,
    headerClass: undefined,
    bodyClass: undefined,
  },
)

defineSlots<DataTableColumnSlots<TRow>>()
</script>
