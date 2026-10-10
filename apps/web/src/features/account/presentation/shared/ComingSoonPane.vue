<template>
  <AccountPane :title="t(titleKey)">
    <section
      class="flex min-h-[320px] items-center py-8 sm:min-h-[420px]"
      :aria-label="t('account.comingSoonAria', { title: t(titleKey) })"
    >
      <div class="max-w-[520px] border-l-2 border-primary pl-5 sm:pl-6">
        <span
          class="inline-flex size-11 items-center justify-center rounded-control border border-border bg-card text-primary shadow-sm"
          aria-hidden="true"
        >
          <component :is="icon" class="size-5" />
        </span>
        <p class="mt-5 mb-0 text-xs font-semibold text-muted-foreground">{{ t(categoryKey) }}</p>
        <StatusBadge
          tone="neutral"
          :dot="false"
          :label="t('account.comingSoon')"
          class="mt-1.5 bg-card hover:bg-card"
        />
      </div>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import StatusBadge from '../../../../core/components/StatusBadge.vue'
import AccountPane from './AccountPane.vue'

defineProps<{
  categoryKey: string
  titleKey: string
  icon: Component
}>()

const { t } = useI18n()
</script>
