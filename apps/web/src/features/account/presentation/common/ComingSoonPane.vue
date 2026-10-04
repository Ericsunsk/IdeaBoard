<template>
  <AccountPane :title="t(titleKey)">
    <section
      class="flex min-h-[320px] items-center py-8 sm:min-h-[420px]"
      :aria-label="t('account.comingSoonAria', { title: t(titleKey) })"
    >
      <div class="max-w-[520px] border-l-2 border-primary pl-5 sm:pl-6">
        <span
          class="inline-flex size-11 items-center justify-center rounded-ib-control border border-border bg-card text-primary shadow-ib-control"
          aria-hidden="true"
        >
          <component :is="icon" class="size-5" />
        </span>
        <p class="mt-5 mb-0 text-xs font-semibold text-muted-foreground">{{ t(categoryKey) }}</p>
        <span
          class="mt-1.5 inline-flex h-6 items-center rounded-ib-pill border border-border bg-card px-2.5 text-xs font-semibold text-muted-foreground"
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
