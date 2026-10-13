<template>
  <div class="min-h-dvh overflow-y-auto bg-muted/40 text-foreground">
    <header class="border-b border-border bg-card">
      <div class="mx-auto flex w-full max-w-[860px] items-center justify-between gap-4 px-6 py-5">
        <RouterLink
          :to="HOME_PATH"
          class="inline-flex min-h-8 items-center rounded-control text-base font-bold text-foreground no-underline focus-visible:outline-none focus-visible:shadow-focus-ring"
        >
          {{ projectName }}
        </RouterLink>
        <RouterLink
          :to="HOME_PATH"
          class="inline-flex min-h-8 items-center rounded-control px-2 text-sm font-semibold text-muted-foreground no-underline transition-colors duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:text-foreground focus-visible:outline-none focus-visible:shadow-focus-ring"
        >
          Back to home
        </RouterLink>
      </div>
    </header>

    <main class="mx-auto w-full max-w-[860px] px-4 py-8 sm:px-6 sm:py-12">
      <article
        class="rounded-panel border border-border bg-card px-5 py-6 shadow-sm sm:rounded-overlay sm:px-10 sm:py-9"
      >
        <header class="border-b border-border pb-7">
          <p class="m-0 text-admin-meta font-semibold tracking-wide text-muted-foreground">
            {{ projectName }}
          </p>
          <h1
            class="mt-2 mb-0 text-admin-summary-value leading-tight font-bold text-foreground sm:text-[34px]"
          >
            {{ title }}
          </h1>
          <p class="mt-3 mb-0 text-sm text-muted-foreground">Effective date: {{ version }}</p>
        </header>

        <div class="legal-copy pt-7 text-admin-title-sm leading-7 text-muted-foreground">
          <slot />
        </div>
      </article>

      <nav
        aria-label="Legal links"
        class="mt-6 flex flex-wrap justify-center gap-x-4 gap-y-2 text-admin-body font-medium text-muted-foreground"
      >
        <RouterLink
          v-for="link in legalLinks"
          :key="link.to"
          :to="link.to"
          class="inline-flex min-h-8 items-center rounded-control px-2 no-underline transition-colors hover:text-foreground focus-visible:outline-none focus-visible:shadow-focus-ring"
        >
          {{ link.label }}
        </RouterLink>
      </nav>
    </main>
  </div>
</template>

<script setup lang="ts">
import { useSiteBranding } from '../../../core/composables/useSiteBranding'
import { i18n } from '../../../core/i18n'
import { HOME_PATH } from '../../workspace/routePaths'
import { PUBLIC_LEGAL_LINKS } from '../legalLinks'

defineProps<{ title: string; version: string }>()

const { projectName } = useSiteBranding({ fallback: i18n.global.t('account.branding.loadFailed') })

const legalLinks = PUBLIC_LEGAL_LINKS
</script>

<style scoped>
.legal-copy :slotted(h2) {
  margin: 1.75rem 0 0.5rem;
  color: var(--foreground);
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
  color: var(--primary);
  font-weight: 600;
  /* Slotted legal copy is prose; inline links must still clear a 32px tap target
     without loosening the surrounding line rhythm. */
  display: inline-block;
  min-height: 32px;
  padding-block: 0.25rem;
}
</style>
