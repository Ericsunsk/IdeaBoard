<template>
  <AccountPane :title="t(titleKey)">
    <section
      class="flex min-h-[420px] items-center py-8 max-[640px]:min-h-[320px]"
      :aria-label="t('account.comingSoonAria', { title: t(titleKey) })"
    >
      <div class="max-w-[520px] border-l-2 border-ib-creative-accent pl-6 max-[520px]:pl-5">
        <span
          class="inline-flex size-11 items-center justify-center rounded-ib-control border border-ib-border-soft bg-ib-surface text-ib-creative-accent-pressed shadow-ib-control"
          aria-hidden="true"
        >
          <component :is="icon" class="size-5" />
        </span>
        <p class="mt-5 mb-0 text-xs font-semibold text-ib-text-subtle">{{ t(categoryKey) }}</p>
        <span
          class="mt-1.5 inline-flex h-6 items-center rounded-ib-pill border border-ib-border-soft bg-ib-surface px-2.5 text-xs font-semibold text-ib-text-muted"
        >
          {{ t('account.comingSoon') }}
        </span>
      </div>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import AccountPane from './AccountPane.vue'

defineProps<{
  categoryKey: string
  titleKey: string
  icon: Component
}>()

const { t } = useI18n()
</script>
