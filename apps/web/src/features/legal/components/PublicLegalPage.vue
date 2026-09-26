<template>
  <div class="min-h-dvh overflow-y-auto bg-ib-surface-muted text-ib-text">
    <header class="border-b border-ib-border-soft bg-ib-surface">
      <div class="mx-auto flex w-full max-w-[860px] items-center justify-between gap-4 px-6 py-5">
        <RouterLink
          :to="HOME_PATH"
          class="rounded-ib-control text-base font-bold text-ib-text no-underline focus-visible:outline-none focus-visible:shadow-ib-focus"
        >
          {{ projectName }}
        </RouterLink>
        <RouterLink
          :to="HOME_PATH"
          class="rounded-ib-control px-2 py-1 text-sm font-semibold text-ib-text-muted no-underline transition-colors duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:text-ib-text focus-visible:outline-none focus-visible:shadow-ib-focus"
        >
          Back to home
        </RouterLink>
      </div>
    </header>

    <main class="mx-auto w-full max-w-[860px] px-6 py-12 max-[640px]:px-4 max-[640px]:py-8">
      <article
        class="rounded-ib-overlay border border-ib-border-soft bg-ib-surface px-10 py-9 shadow-ib-control max-[640px]:rounded-ib-panel max-[640px]:px-5 max-[640px]:py-6"
      >
        <header class="border-b border-ib-border-soft pb-7">
          <p class="m-0 text-[12px] font-semibold tracking-wide text-ib-text-subtle">
            {{ projectName }}
          </p>
          <h1
            class="mt-2 mb-0 text-[34px] leading-tight font-bold text-ib-text max-[640px]:text-[28px]"
          >
            {{ title }}
          </h1>
          <p class="mt-3 mb-0 text-sm text-ib-text-muted">Effective date: {{ version }}</p>
        </header>

        <div class="legal-copy pt-7 text-[15px] leading-7 text-ib-text-muted">
          <slot />
        </div>
      </article>

      <nav
        aria-label="Legal links"
        class="mt-6 flex flex-wrap justify-center gap-x-4 gap-y-2 text-[13px] font-medium text-ib-text-muted"
      >
        <RouterLink
          v-for="link in legalLinks"
          :key="link.to"
          :to="link.to"
          class="rounded-ib-control no-underline transition-colors hover:text-ib-text focus-visible:outline-none focus-visible:shadow-ib-focus"
        >
          {{ link.label }}
        </RouterLink>
      </nav>
    </main>
  </div>
</template>

<script setup lang="ts">
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { i18n } from '../../../shared/i18n'
import { HOME_PATH } from '../../../shared/constants/routePaths'
import { PUBLIC_LEGAL_LINKS } from '../../../shared/constants/legalLinks'

defineProps<{ title: string; version: string }>()

const { projectName } = useSiteBranding({ fallback: i18n.global.t('account.branding.loadFailed') })

const legalLinks = PUBLIC_LEGAL_LINKS
</script>

<style scoped>
.legal-copy :slotted(h2) {
  margin: 1.75rem 0 0.5rem;
  color: var(--color-ib-text);
  font-size: 1.05rem;
  font-weight: 700;
  line-height: 1.5;
}

.legal-copy :slotted(p),
.legal-copy :slotted(ul) {
  margin: 0.5rem 0 0;
}

.legal-copy :slotted(ul) {
  padding-left: 1.25rem;
}

.legal-copy :slotted(a) {
  color: var(--color-ib-creative-accent-pressed);
  font-weight: 600;
}
</style>
