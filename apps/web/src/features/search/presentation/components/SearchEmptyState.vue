<template>
  <section
    v-if="hasError && resultsCount === 0"
    class="mt-8 flex min-h-[320px] flex-col items-center justify-center rounded-3xl border border-dashed border-board-card-border/80 bg-board-loading-bg/40 px-6 py-14 text-center backdrop-blur-sm"
  >
    <div class="flex max-w-sm flex-col items-center">
      <div
        class="mb-4 flex size-14 items-center justify-center rounded-2xl bg-board-card-bg text-board-danger shadow-xs ring-1 ring-board-card-border/70"
      >
        <WarningTriangle class="size-6 stroke-[1.75]" aria-hidden="true" />
      </div>
      <h3 class="m-0 text-base font-semibold text-board-page-text tracking-tight">
        {{ t('search.errors.unableToLoadInspiration') }}
      </h3>
      <p class="mt-2 mb-0 text-xs sm:text-[13px] text-board-page-text-muted leading-relaxed">
        {{ t('search.errors.unableToLoadResults') }}
      </p>
      <div class="mt-6">
        <Button
          class="!h-9 !rounded-full !border-0 !bg-board-primary !px-5 !text-xs !font-semibold !text-ib-text-inverse hover:!bg-board-primary-hover cursor-pointer shadow-xs"
          @click="$emit('reload')"
        >
          {{ t('search.actions.retry') }}
        </Button>
      </div>
    </div>
  </section>

  <section
    v-else-if="resultsCount === 0"
    class="mt-8 flex min-h-[320px] flex-col items-center justify-center rounded-3xl border border-dashed border-board-card-border/80 bg-board-loading-bg/40 px-6 py-14 text-center backdrop-blur-sm"
  >
    <div class="flex max-w-sm flex-col items-center">
      <div
        class="mb-4 flex size-14 items-center justify-center rounded-2xl bg-board-card-bg shadow-xs ring-1 ring-board-card-border/70 transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105"
      >
        <Search
          v-if="hasActiveSearch"
          class="size-6 stroke-[1.75] text-board-page-text-muted"
          aria-hidden="true"
        />
        <Compass v-else class="size-6 stroke-[1.75] text-board-primary" aria-hidden="true" />
      </div>

      <h3 class="m-0 text-base font-semibold text-board-page-text tracking-tight">
        {{
          hasActiveSearch ? t('search.empty.noResultsTitle') : t('search.empty.emptyFeedTitle')
        }}
      </h3>

      <p class="mt-2 mb-0 text-xs sm:text-[13px] text-board-page-text-muted leading-relaxed">
        {{
          hasActiveSearch
            ? t('search.empty.noResultsDescription')
            : t('search.empty.emptyFeedDescription')
        }}
      </p>

      <div class="mt-6 flex flex-wrap items-center justify-center gap-2.5">
        <button
          v-if="hasActiveSearch"
          type="button"
          class="h-9 px-4 rounded-full border border-board-card-border bg-board-card-bg text-xs font-medium text-board-page-text shadow-xs transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-board-primary hover:text-board-primary cursor-pointer flex items-center gap-1.5"
          @click="$emit('clearSearch')"
        >
          <Xmark class="size-3.5" aria-hidden="true" />
          <span>{{ t('search.empty.clearQuery') }}</span>
        </button>

        <template v-else>
          <button
            type="button"
            class="h-9 px-4 rounded-full border border-board-card-border bg-board-card-bg text-xs font-medium text-board-page-text shadow-xs transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-board-primary hover:text-board-primary cursor-pointer flex items-center gap-1.5 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!canAddReference"
            @click="$emit('openFilePicker')"
          >
            <Camera class="size-3.5 text-board-primary" aria-hidden="true" />
            <span>{{ t('search.fallbackImageTitle') }}</span>
          </button>
          <button
            type="button"
            class="h-9 px-4 rounded-full bg-board-primary text-xs font-semibold text-ib-text-inverse shadow-xs transition-[background-color,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-board-primary-hover hover:scale-[1.02] active:scale-[0.98] cursor-pointer flex items-center gap-1.5"
            @click="$emit('startCreating')"
          >
            <Plus class="size-3.5 stroke-[2.5]" aria-hidden="true" />
            <span>{{ t('search.empty.startCreating') }}</span>
          </button>
        </template>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Camera, Compass, Plus, Search, WarningTriangle, Xmark } from '@iconoir/vue'

defineProps<{
  hasError: boolean
  resultsCount: number
  hasActiveSearch: boolean
  canAddReference: boolean
}>()

defineEmits<{
  (e: 'reload'): void
  (e: 'clearSearch'): void
  (e: 'openFilePicker'): void
  (e: 'startCreating'): void
}>()

const { t } = useI18n()
</script>
