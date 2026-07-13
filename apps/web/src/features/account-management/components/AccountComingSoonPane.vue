<template>
  <section
    class="flex min-h-[420px] items-center py-8 max-[640px]:min-h-[320px]"
    :aria-label="`${title}即将上线`"
  >
    <div class="max-w-[520px] border-l-2 border-ib-creative-accent pl-6 max-[520px]:pl-5">
      <span
        class="inline-flex size-11 items-center justify-center rounded-[var(--radius-ib-control)] border border-ib-border-soft bg-ib-surface text-ib-creative-accent-pressed shadow-[var(--shadow-ib-control)]"
        aria-hidden="true"
      >
        <component :is="icon" class="size-5" />
      </span>
      <p class="mt-5 mb-0 text-xs font-semibold text-ib-text-subtle">{{ category }}</p>
      <div class="mt-1.5 flex flex-wrap items-center gap-3">
        <h2 class="m-0 text-[28px] leading-tight font-bold text-ib-text max-[520px]:text-[24px]">
          {{ title }}
        </h2>
        <span
          class="inline-flex h-6 items-center rounded-[var(--radius-ib-pill)] border border-ib-border-soft bg-ib-surface px-2.5 text-xs font-semibold text-ib-text-muted"
        >
          即将上线
        </span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  category: string
  title: string
  icon: Component
}>()
</script>
